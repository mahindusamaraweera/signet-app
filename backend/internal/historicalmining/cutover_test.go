package historicalmining

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"database/sql/driver"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/jmoiron/sqlx"
	"io"
	"strconv"
	"strings"
	"testing"
	"time"
)

// Scripted transactions prove ordering and rollback, not MySQL isolation.
type step struct {
	match    string
	cols     []string
	vals     []driver.Value
	affected int64
	check    func([]driver.NamedValue) error
}
type fixture struct {
	steps             []step
	committed, rolled bool
}
type fixtureDriver struct{}

func (fixtureDriver) Open(string) (driver.Conn, error)                          { return nil, errors.New("connector only") }
func (f *fixture) Connect(context.Context) (driver.Conn, error)                 { return f, nil }
func (f *fixture) Driver() driver.Driver                                        { return fixtureDriver{} }
func (f *fixture) Prepare(string) (driver.Stmt, error)                          { return nil, errors.New("unexpected prepare") }
func (f *fixture) Close() error                                                 { return nil }
func (f *fixture) Begin() (driver.Tx, error)                                    { return f, nil }
func (f *fixture) BeginTx(context.Context, driver.TxOptions) (driver.Tx, error) { return f, nil }
func (f *fixture) Commit() error                                                { f.committed = true; return nil }
func (f *fixture) Rollback() error                                              { f.rolled = true; return nil }
func (f *fixture) next(q string, args []driver.NamedValue) (step, error) {
	if len(f.steps) == 0 {
		return step{}, errors.New("unexpected SQL")
	}
	s := f.steps[0]
	f.steps = f.steps[1:]
	if !strings.Contains(q, s.match) {
		return s, errors.New("wrong SQL order: " + q)
	}
	if s.check != nil {
		if err := s.check(args); err != nil {
			return s, err
		}
	}
	return s, nil
}
func (f *fixture) QueryContext(_ context.Context, q string, a []driver.NamedValue) (driver.Rows, error) {
	s, e := f.next(q, a)
	return &rows{s.cols, s.vals, false}, e
}
func (f *fixture) ExecContext(_ context.Context, q string, a []driver.NamedValue) (driver.Result, error) {
	s, e := f.next(q, a)
	return driver.RowsAffected(s.affected), e
}

type rows struct {
	cols []string
	vals []driver.Value
	read bool
}

func (r *rows) Columns() []string { return r.cols }
func (r *rows) Close() error      { return nil }
func (r *rows) Next(d []driver.Value) error {
	if r.read || r.vals == nil {
		return io.EOF
	}
	r.read = true
	copy(d, r.vals)
	return nil
}
func original(p Profile) Mining {
	return Mining{p.MiningID, p.UserID, p.OldTotal, "13637.49991272", "1000", "active", "pending", "", "2", "2026-10-19 00:01:00", "2026-10-05 09:32:45", "2026-10-19 00:00:00"}
}
func reads(p Profile, m Mining, audit *Snapshot) []step {
	activation, err := time.Parse("2006-01-02 15:04:05", p.Activated)
	if err != nil {
		panic(err)
	}
	utcActivation := activation.Add(-330 * time.Minute).Format("2006-01-02 15:04:05")
	s := []step{
		{match: "FROM users", cols: []string{"email", "uuid", "status"}, vals: []driver.Value{p.Email, p.UUID, "active"}},
		{match: "FROM user_packages", cols: []string{"user_id", "package", "status", "sale", "activated"}, vals: []driver.Value{int64(p.UserID), p.PackageID, "active", "first", utcActivation}},
		{match: "FROM user_minings", cols: []string{"id", "user_id", "total_token", "mining_token", "daily_mining", "status", "webhook_status", "webhook_sent_at", "webhook_attempts", "next_retry_at", "created_at", "updated_at"}, vals: []driver.Value{int64(m.ID), int64(m.UserID), m.Total, m.Accrued, m.Daily, m.Status, m.WebhookStatus, m.WebhookSent, m.WebhookAttempts, m.NextRetry, m.Created, m.Updated}},
		{match: "FROM historical_mining_cutovers", cols: []string{"snapshot_json", "snapshot_sha256", "batch_id"}}}
	if audit != nil {
		raw, _ := json.Marshal(audit)
		s[3].vals = []driver.Value{string(raw), digest(raw), Batch}
	}
	return s
}
func dbFor(f *fixture) *sqlx.DB { return sqlx.NewDb(sql.OpenDB(f), "mysql") }
func TestDryRunDoesNotWriteOrClaimPayment(t *testing.T) {
	f := &fixture{}
	for _, p := range Profiles() {
		f.steps = append(f.steps, reads(p, original(p), nil)...)
	}
	db := dbFor(f)
	defer db.Close()
	out, err := Run(context.Background(), db, false, false, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 3 || out[1].AfterTotal != "3500000" || out[1].Before.Total != "16000000" || !f.rolled || f.committed || len(f.steps) != 0 {
		t.Fatal("dry run lost evidence or wrote state")
	}
}
func TestAtomicArchiveBeforeResetAndRollback(t *testing.T) {
	for _, fail := range []bool{false, true} {
		f := &fixture{}
		for _, p := range Profiles() {
			f.steps = append(f.steps, reads(p, original(p), nil)...)
		}
		for i, p := range Profiles() {
			p := p
			f.steps = append(f.steps, step{match: "INSERT INTO historical_mining_cutovers", affected: 1, check: func(a []driver.NamedValue) error {
				var s Snapshot
				if json.Unmarshal([]byte(a[4].Value.(string)), &s) != nil || s.Before.Total != p.OldTotal || s.Before.Accrued != "13637.49991272" || s.AfterTotal != p.Amount || digest([]byte(a[4].Value.(string))) != a[5].Value {
					return errors.New("archive lost original mining")
				}
				return nil
			}}, step{match: "SET total_token=?,mining_token=0,daily_mining=0,status='inactive',webhook_status='historical_frozen'", affected: 1})
			if fail && i == 2 {
				f.steps[len(f.steps)-1].affected = 0
			}
		}
		db := dbFor(f)
		_, err := Run(context.Background(), db, true, false, time.Now())
		db.Close()
		if (err != nil) != fail || f.committed == fail || f.rolled != fail || len(f.steps) != 0 {
			t.Fatalf("wrong transaction boundary %v", err)
		}
	}
}
func TestFrozenReexportHasNoWritesAndSignsBoundScope(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	f := &fixture{}
	for _, p := range Profiles() {
		s := snapshot(p, original(p), now)
		m := s.Before
		m.Total = p.Amount
		m.Accrued = "0.00000000"
		m.Daily = "0"
		m.Status = "inactive"
		m.WebhookStatus = "historical_frozen"
		m.NextRetry = ""
		f.steps = append(f.steps, reads(p, m, &s)...)
	}
	db := dbFor(f)
	defer db.Close()
	out, err := Run(context.Background(), db, false, true, now)
	if err != nil {
		t.Fatal(err)
	}
	proof, err := Proof(out, strings.Repeat("k", 32), now)
	if err != nil {
		t.Fatal(err)
	}
	var e map[string]string
	json.Unmarshal(proof, &e)
	mac := hmac.New(sha256.New, []byte(strings.Repeat("k", 32)))
	mac.Write([]byte(Domain + e["payload"]))
	if e["signature"] != hex.EncodeToString(mac.Sum(nil)) {
		t.Fatal("proof domain mismatch")
	}
	raw, err := base64.StdEncoding.DecodeString(e["payload"])
	if err != nil || !strings.Contains(string(raw), Batch) {
		t.Fatal("proof lost scope")
	}
	if f.committed || !f.rolled || len(f.steps) != 0 {
		t.Fatal("export wrote state")
	}
	out[0].AfterDaily = "1000"
	if _, err := Proof(out, strings.Repeat("k", 32), now); err == nil {
		t.Fatal("active mining signed")
	}
}
func TestExportBeforeFreezeIsRejected(t *testing.T) {
	p := Profiles()[0]
	f := &fixture{steps: reads(p, original(p), nil)}
	db := dbFor(f)
	defer db.Close()
	if _, err := Run(context.Background(), db, false, true, time.Now()); err == nil || f.committed {
		t.Fatal("unfrozen source exported")
	}
}

func TestOptionOneBatchIsSeparateAndArchivesOnlyNineReviewedMiningRows(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	f := &fixture{}
	for _, p := range OptionOneProfiles() {
		f.steps = append(f.steps, reads(p, original(p), nil)...)
	}
	for _, p := range OptionOneProfiles() {
		p := p
		f.steps = append(f.steps, step{match: "INSERT INTO historical_mining_cutovers", affected: 1, check: func(a []driver.NamedValue) error {
			var s Snapshot
			if a[2].Value != OptionOneBatch || json.Unmarshal([]byte(a[4].Value.(string)), &s) != nil || s.Profile != p || s.Before.Total != p.OldTotal || s.AfterTotal != p.Amount || (p.UserID == 377 && p.PackageRecord != 397) {
				return errors.New("wrong exception scope")
			}
			return nil
		}}, step{match: "UPDATE user_minings", affected: 1, check: func(a []driver.NamedValue) error {
			if a[1].Value != int64(p.MiningID) || a[2].Value != int64(p.UserID) {
				return errors.New("wrong mining owner")
			}
			return nil
		}})
	}
	db := dbFor(f)
	defer db.Close()
	out, err := RunBatch(context.Background(), db, true, false, now, OptionOneBatch)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 9 || !f.committed || len(f.steps) != 0 {
		t.Fatal("nine-user cutover incomplete")
	}
	proof, err := ProofBatch(out, strings.Repeat("k", 32), now, OptionOneBatch)
	if err != nil || !strings.Contains(string(proof), "payload") {
		t.Fatal("missing proof", err)
	}
	if _, err := Proof(out, strings.Repeat("k", 32), now); err == nil {
		t.Fatal("wrong batch accepted")
	}
	if _, err := RunBatch(context.Background(), db, true, false, now, "all-users"); err == nil {
		t.Fatal("unreviewed batch accepted")
	}
}

func TestPackageActivationUsesUTCWithoutChangingReviewedEvidence(t *testing.T) {
	p := OptionOneProfiles()[0]
	for _, tc := range []struct {
		timestamp string
		valid     bool
	}{
		{"2026-07-03 20:25:25", true},
		{"2026-07-04 01:55:25", false},
		{"2026-07-03 20:25:26", false},
	} {
		f := &fixture{steps: reads(p, original(p), nil)[:2]}
		f.steps[1].vals[4] = tc.timestamp
		db := dbFor(f)
		tx, err := db.BeginTxx(context.Background(), nil)
		if err != nil {
			t.Fatal(err)
		}
		err = loadOwner(context.Background(), tx, p)
		if (err == nil) != tc.valid {
			t.Errorf("timestamp %s: %v", tc.timestamp, err)
		}
		tx.Rollback()
		db.Close()
	}
	if p.Activated != "2026-07-04 01:55:25" {
		t.Fatal("reviewed proof timestamp changed")
	}
}

func octoberReads(p Profile, m Mining, audit *Snapshot) []step {
	s := reads(p, m, audit)
	if p.MiningID == 0 {
		s[2].vals = nil
	}
	if audit != nil {
		s[3].vals[2] = OctoberElevenBatch
	}
	return s
}

func TestOctoberElevenApplyAndExportPreserveAbsenceAndFundingScope(t *testing.T) {
	at := time.Date(2026, 10, 29, 0, 0, 0, 0, time.UTC)
	profiles := OctoberElevenProfiles()
	if len(profiles) != 11 {
		t.Fatal("scope changed")
	}
	f := &fixture{}
	snapshots := []Snapshot{}
	total := uint64(0)
	for _, p := range profiles {
		m := original(p)
		if p.MiningID == 0 {
			m = absentMining(p)
		}
		f.steps = append(f.steps, octoberReads(p, m, nil)...)
		snapshots = append(snapshots, snapshot(p, m, at))
		amount, err := strconv.ParseUint(p.Amount, 10, 64)
		if err != nil {
			t.Fatal(err)
		}
		total += amount
	}
	if total != 5800000 || profiles[6].UserID != 449 || profiles[9].Email != "hbmadubashi81@gmail.com" {
		t.Fatal("reviewed correction changed")
	}
	for _, p := range profiles {
		p := p
		f.steps = append(f.steps, step{match: "INSERT INTO historical_mining_cutovers", affected: 1, check: func(a []driver.NamedValue) error {
			if p.MiningID == 0 && a[1].Value != nil {
				return errors.New("invented mining ID")
			}
			return nil
		}})
		if p.MiningID != 0 {
			f.steps = append(f.steps, step{match: "UPDATE user_minings", affected: 1})
		}
	}
	db := dbFor(f)
	out, err := RunBatch(context.Background(), db, true, false, at, OctoberElevenBatch)
	db.Close()
	if err != nil || !f.committed || len(f.steps) != 0 {
		t.Fatalf("apply: %v", err)
	}
	if _, err = ProofBatch(out, strings.Repeat("k", 32), at, OctoberElevenBatch); err != nil {
		t.Fatal(err)
	}
	// An already frozen no-mining account stays row-free on export and reruns.
	f = &fixture{}
	for i, p := range profiles {
		m := original(p)
		if p.MiningID == 0 {
			m = absentMining(p)
		} else {
			m.Total = p.Amount
			m.Accrued = "0.00000000"
			m.Daily = "0"
			m.Status = "inactive"
			m.WebhookStatus = "historical_frozen"
			m.NextRetry = ""
		}
		f.steps = append(f.steps, octoberReads(p, m, &snapshots[i])...)
	}
	db = dbFor(f)
	defer db.Close()
	if _, err = RunBatch(context.Background(), db, false, true, at, OctoberElevenBatch); err != nil || f.committed || len(f.steps) != 0 {
		t.Fatalf("export: %v", err)
	}
}

func TestOctoberElevenUnexpectedMiningRowAbortsBeforeAnyWrites(t *testing.T) {
	f := &fixture{}
	for _, p := range OctoberElevenProfiles() {
		steps := octoberReads(p, original(p), nil)
		if p.MiningID == 0 {
			steps[2].vals = []driver.Value{int64(999), int64(p.UserID), "1", "0", "1", "active", "pending", "", "0", "", "", ""}
			f.steps = append(f.steps, steps[:3]...)
			break
		}
		f.steps = append(f.steps, steps...)
	}
	db := dbFor(f)
	defer db.Close()
	_, err := RunBatch(context.Background(), db, true, false, time.Now(), OctoberElevenBatch)
	if err == nil || f.committed || !f.rolled || len(f.steps) != 0 {
		t.Fatalf("unexpected mining accepted: %v", err)
	}
}

func TestOctober444UsesVerifiedDatabaseEmail(t *testing.T) {
	p := OctoberElevenProfiles()[4]
	if p.UserID != 444 || p.Email != "chamindananayakkaraforkalignme@gmail.com" {
		t.Fatal("verified email changed")
	}
	f := &fixture{steps: reads(p, absentMining(p), nil)[:2]}
	f.steps[0].vals[0] = "chamindananayakkaraforkaliganme@gmail.com"
	db := dbFor(f)
	defer db.Close()
	tx, err := db.BeginTxx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if err := loadOwner(context.Background(), tx, p); err == nil {
		t.Fatal("CSV typo accepted instead of verified identity")
	}
}

func TestOctober456RejectsPreviouslyConfirmedEmailTypo(t *testing.T) {
	p := OctoberElevenProfiles()[9]
	if p.UserID != 456 || p.Email != "hbmadubashi81@gmail.com" {
		t.Fatal("verified recipient changed")
	}
	f := &fixture{steps: reads(p, original(p), nil)[:2]}
	f.steps[0].vals[0] = "bhmadubashi81@gmail.com"
	db := dbFor(f)
	defer db.Close()
	tx, err := db.BeginTxx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if err := loadOwner(context.Background(), tx, p); err == nil {
		t.Fatal("old recipient typo accepted")
	}
}
