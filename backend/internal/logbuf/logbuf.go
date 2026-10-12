// Package logbuf keeps the most recent log lines of this process in memory,
// so they can be shown through the admin logs endpoint when the systemd
// journal isn't readable (local development, missing permission). It holds
// only this process's own `log` output since it started.
package logbuf

import (
	"bytes"
	"sync"
)

// Ring is a fixed-size, concurrency-safe buffer of complete lines. It is an
// io.Writer, so it can sit next to stderr in log.SetOutput(io.MultiWriter(...)).
type Ring struct {
	mu      sync.Mutex
	lines   []string
	next    int
	full    bool
	partial []byte
}

func New(capacity int) *Ring {
	if capacity < 1 {
		capacity = 1
	}
	return &Ring{lines: make([]string, capacity)}
}

// Default is the process-wide buffer wired up in cmd/api.
var Default = New(5000)

func (r *Ring) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	data := append(r.partial, p...)
	for {
		i := bytes.IndexByte(data, '\n')
		if i < 0 {
			break
		}
		r.add(string(data[:i]))
		data = data[i+1:]
	}
	// Keep an unterminated tail for the next write, but never unboundedly.
	if len(data) > 64<<10 {
		r.add(string(data))
		data = nil
	}
	r.partial = append([]byte(nil), data...)
	return len(p), nil
}

func (r *Ring) add(line string) {
	r.lines[r.next] = line
	r.next = (r.next + 1) % len(r.lines)
	if r.next == 0 {
		r.full = true
	}
}

// Lines returns the buffered lines, oldest first.
func (r *Ring) Lines() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.full {
		return append([]string(nil), r.lines[:r.next]...)
	}
	out := make([]string, 0, len(r.lines))
	out = append(out, r.lines[r.next:]...)
	return append(out, r.lines[:r.next]...)
}
