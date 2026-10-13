// logs_handler.go — read-only view of this API's own logs for the company
// (owner) role, instead of `sudo journalctl -u <unit>` on the server.
//
//	GET /api/v1/admin/logs?since=24h&grep=purchase%20intent&limit=500
//
// Source: the systemd journal for THIS service's unit (signet-api,
// signet-api-staging, … detected from /proc/self/cgroup), so it survives
// restarts. If the journal can't be read (not Linux, or the service user isn't
// in the systemd-journal group) it falls back to the in-memory buffer of this
// process's recent log lines and says so in "warning".
//
// Nothing from the request reaches a shell: journalctl is exec'd with fixed
// arguments, `since` is turned into a timestamp we format ourselves, and
// `grep` is a plain case-insensitive substring matched in Go.
package handlers

import (
	"bufio"
	"bytes"
	"context"
	"log"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"signet-backend/internal/app"
	"signet-backend/internal/auth"
	"signet-backend/internal/httpx"
	"signet-backend/internal/logbuf"
)

const (
	logsDefaultSince = 24 * time.Hour
	logsMaxSince     = 7 * 24 * time.Hour
	logsDefaultLimit = 500
	logsMaxLimit     = 5000
	logsMaxGrep      = 200
)

var unitNameRe = regexp.MustCompile(`^[A-Za-z0-9@._-]+\.service$`)

func RegisterLogRoutes(r chi.Router, d *app.Deps) {
	r.Group(func(r chi.Router) {
		r.Use(d.Auth.RequireAuth)
		r.Use(auth.RequireRole("company"))
		r.Use(httpx.SecurityRateLimit(20, time.Minute))
		r.Get("/api/v1/admin/logs", adminLogsHandler(d))
	})
}

type logsResult struct {
	Status    string   `json:"status"`
	Source    string   `json:"source"` // "journal" or "memory"
	Unit      string   `json:"unit,omitempty"`
	Since     string   `json:"since"`
	Grep      string   `json:"grep,omitempty"`
	Count     int      `json:"count"`
	Truncated bool     `json:"truncated"` // more lines matched than `limit`; the newest are returned
	Warning   string   `json:"warning,omitempty"`
	Lines     []string `json:"lines"`
}

func adminLogsHandler(d *app.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()

		since := logsDefaultSince
		if v := q.Get("since"); v != "" {
			parsed, err := time.ParseDuration(v)
			if err != nil || parsed <= 0 || parsed > logsMaxSince {
				httpx.ValidationError(w, map[string][]string{"since": {"Use a duration like 30m, 6h or 24h (max 168h)."}})
				return
			}
			since = parsed
		}
		limit := logsDefaultLimit
		if v := q.Get("limit"); v != "" {
			n, err := strconv.Atoi(v)
			if err != nil || n < 1 || n > logsMaxLimit {
				httpx.ValidationError(w, map[string][]string{"limit": {"Use a number from 1 to 5000."}})
				return
			}
			limit = n
		}
		grep := strings.TrimSpace(q.Get("grep"))
		if len(grep) > logsMaxGrep || strings.ContainsAny(grep, "\r\n\x00") {
			httpx.ValidationError(w, map[string][]string{"grep": {"Use plain text up to 200 characters."}})
			return
		}

		from := time.Now().Add(-since)
		res := logsResult{Status: "success", Since: from.Format(time.RFC3339), Grep: grep}

		var warning string
		if unit := currentSystemdUnit(); unit != "" && q.Get("source") != "memory" {
			res.Unit = unit
			lines, truncated, err := readJournal(r.Context(), unit, from, grep, limit)
			if err == nil {
				res.Source, res.Lines, res.Truncated = "journal", lines, truncated
			} else {
				warning = err.Error()
			}
		} else if q.Get("source") != "memory" {
			warning = "not running under systemd; showing this process's in-memory log buffer"
		}
		if res.Source == "" {
			res.Source = "memory"
			res.Lines, res.Truncated = filterLines(logbuf.Default.Lines(), from, grep, limit)
			if warning != "" {
				res.Warning = warning + " — showing only lines logged by this process since it started"
			}
		}
		if res.Lines == nil {
			res.Lines = []string{}
		}
		res.Count = len(res.Lines)

		var viewer uint64
		if user := auth.UserFromContext(r.Context()); user != nil {
			viewer = user.ID
		}
		log.Printf("admin-logs: user %d viewed %d %s log lines (since %s, grep %q)", viewer, res.Count, res.Source, since, grep)
		w.Header().Set("Cache-Control", "no-store")
		httpx.OK(w, res)
	}
}

// currentSystemdUnit returns this process's service unit (e.g.
// "signet-api-staging.service") from its cgroup, or "" when not under systemd.
func currentSystemdUnit() string {
	if v := os.Getenv("LOG_JOURNAL_UNIT"); v != "" {
		if !strings.HasSuffix(v, ".service") {
			v += ".service"
		}
		if unitNameRe.MatchString(v) {
			return v
		}
		return ""
	}
	data, err := os.ReadFile("/proc/self/cgroup")
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		for _, part := range strings.Split(line, "/") {
			if unitNameRe.MatchString(part) {
				return part
			}
		}
	}
	return ""
}

type journalUnreadable struct{ msg string }

func (e journalUnreadable) Error() string { return e.msg }

// readJournal runs journalctl for one unit and keeps the newest `limit` lines
// that contain grep (case-insensitive).
func readJournal(ctx context.Context, unit string, from time.Time, grep string, limit int) ([]string, bool, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "journalctl",
		"--unit", unit,
		"--since", from.Format("2006-01-02 15:04:05"),
		"--no-pager", "--quiet", "--output", "short-iso")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, false, journalUnreadable{"journal unavailable: " + err.Error()}
	}
	if err := cmd.Start(); err != nil {
		return nil, false, journalUnreadable{"journalctl not available: " + err.Error()}
	}
	needle := strings.ToLower(grep)
	ring := make([]string, 0, limit)
	matched := 0
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 64<<10), 1<<20)
	for scanner.Scan() {
		line := scanner.Text()
		if needle != "" && !strings.Contains(strings.ToLower(line), needle) {
			continue
		}
		matched++
		if len(ring) < limit {
			ring = append(ring, line)
		} else {
			copy(ring, ring[1:])
			ring[limit-1] = line
		}
	}
	waitErr := cmd.Wait()
	if ctx.Err() != nil {
		return nil, false, journalUnreadable{"reading the journal timed out; narrow `since` or add `grep`"}
	}
	msg := stderr.String()
	if matched == 0 && (strings.Contains(msg, "insufficient permissions") || strings.Contains(msg, "not seeing messages")) {
		return nil, false, journalUnreadable{"the service user can't read the journal (fix: sudo usermod -aG systemd-journal <service user> && sudo systemctl restart " + unit + ")"}
	}
	if waitErr != nil && matched == 0 {
		return nil, false, journalUnreadable{"journalctl failed: " + strings.TrimSpace(firstLine(msg))}
	}
	return ring, matched > limit, nil
}

// filterLines applies since/grep/limit to buffered `log` lines, whose prefix is
// the standard "2006/01/02 15:04:05" timestamp; lines without one are kept.
func filterLines(lines []string, from time.Time, grep string, limit int) ([]string, bool) {
	needle := strings.ToLower(grep)
	out := make([]string, 0, limit)
	matched := 0
	for _, line := range lines {
		if len(line) >= 19 {
			if t, err := time.ParseInLocation("2006/01/02 15:04:05", line[:19], time.Local); err == nil && t.Before(from) {
				continue
			}
		}
		if needle != "" && !strings.Contains(strings.ToLower(line), needle) {
			continue
		}
		matched++
		if len(out) < limit {
			out = append(out, line)
		} else {
			copy(out, out[1:])
			out[limit-1] = line
		}
	}
	return out, matched > limit
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
