// Package migrate is a small versioned migration runner for MySQL. Applied
// migrations are recorded in the schema_migrations table; each run applies
// only the migrations with no record, in version order, holding a MySQL
// named lock so two instances can never migrate the same database at once.
package migrate

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"
	"text/tabwriter"
	"time"
)

// Table is the migration history table.
const Table = "schema_migrations"

// BaselineVersion is the migration that describes the schema the live
// database already had before versioned migrations existed. A database
// that has tables but no history is baselined up to this version instead
// of having it executed.
const BaselineVersion = 1

const createTableSQL = `CREATE TABLE IF NOT EXISTS ` + Table + ` (
  version BIGINT UNSIGNED NOT NULL,
  name VARCHAR(255) NOT NULL,
  checksum CHAR(64) NOT NULL,
  baseline TINYINT(1) NOT NULL DEFAULT 0,
  execution_ms INT UNSIGNED NOT NULL DEFAULT 0,
  applied_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (version)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`

// Record is one row of the history table.
type Record struct {
	Version   uint64
	Name      string
	Checksum  string
	Baseline  bool
	AppliedAt time.Time
}

// Migrator runs every statement on one dedicated connection: MySQL named
// locks and session settings (SET FOREIGN_KEY_CHECKS, ...) are per
// connection, so a pooled *sql.DB would silently spread them across
// sessions.
type Migrator struct {
	conn       *sql.Conn
	migrations []Migration
	out        io.Writer
	lockName   string
}

func New(ctx context.Context, db *sql.DB, migrations []Migration, out io.Writer) (*Migrator, error) {
	conn, err := db.Conn(ctx)
	if err != nil {
		return nil, fmt.Errorf("open connection: %w", err)
	}
	return &Migrator{conn: conn, migrations: migrations, out: out}, nil
}

func (m *Migrator) Close() error { return m.conn.Close() }

// Lock takes a server-wide named lock scoped to the current database, so a
// second migrate process (another instance, a double-clicked deploy) waits
// and then fails instead of running the same migrations concurrently.
func (m *Migrator) Lock(ctx context.Context, timeout time.Duration) error {
	var dbName sql.NullString
	if err := m.conn.QueryRowContext(ctx, "SELECT DATABASE()").Scan(&dbName); err != nil {
		return fmt.Errorf("read current database: %w", err)
	}
	if !dbName.Valid || dbName.String == "" {
		return errors.New("no database selected (set DB_DATABASE)")
	}
	name := "schema_migrations." + dbName.String
	if len(name) > 64 {
		name = name[:64]
	}

	var got sql.NullInt64
	if err := m.conn.QueryRowContext(ctx, "SELECT GET_LOCK(?, ?)", name, int(timeout.Seconds())).Scan(&got); err != nil {
		return fmt.Errorf("acquire migration lock: %w", err)
	}
	if !got.Valid || got.Int64 != 1 {
		return fmt.Errorf("another migrate process holds the lock %q (waited %s); try again once it has finished", name, timeout)
	}
	m.lockName = name
	return nil
}

func (m *Migrator) Unlock(ctx context.Context) {
	if m.lockName != "" {
		m.conn.ExecContext(ctx, "SELECT RELEASE_LOCK(?)", m.lockName)
		m.lockName = ""
	}
}

func (m *Migrator) EnsureTable(ctx context.Context) error {
	if _, err := m.conn.ExecContext(ctx, createTableSQL); err != nil {
		return fmt.Errorf("create %s: %w", Table, err)
	}
	return nil
}

func (m *Migrator) tableExists(ctx context.Context) (bool, error) {
	var n int
	err := m.conn.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = DATABASE() AND table_name = ?", Table).Scan(&n)
	return n > 0, err
}

// otherTables counts the tables in the database besides the history table.
func (m *Migrator) otherTables(ctx context.Context) (int, error) {
	var n int
	err := m.conn.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = DATABASE() AND table_name <> ?", Table).Scan(&n)
	return n, err
}

func (m *Migrator) applied(ctx context.Context) (map[uint64]Record, error) {
	rows, err := m.conn.QueryContext(ctx, "SELECT version, name, checksum, baseline, applied_at FROM "+Table)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", Table, err)
	}
	defer rows.Close()
	out := map[uint64]Record{}
	for rows.Next() {
		var r Record
		var baseline int
		if err := rows.Scan(&r.Version, &r.Name, &r.Checksum, &baseline, &r.AppliedAt); err != nil {
			return nil, fmt.Errorf("read %s: %w", Table, err)
		}
		r.Baseline = baseline != 0
		out[r.Version] = r
	}
	return out, rows.Err()
}

// Up applies every pending migration in version order and stops at the
// first failure. On a database that already has tables but no history, it
// first baselines the initial migration rather than running it.
func (m *Migrator) Up(ctx context.Context) error {
	if err := m.EnsureTable(ctx); err != nil {
		return err
	}
	applied, err := m.applied(ctx)
	if err != nil {
		return err
	}

	if len(applied) == 0 {
		existing, err := m.otherTables(ctx)
		if err != nil {
			return fmt.Errorf("inspect database: %w", err)
		}
		if existing > 0 && m.find(BaselineVersion) != nil {
			fmt.Fprintf(m.out, "Existing database detected (%d tables, no migration history).\n", existing)
			fmt.Fprintf(m.out, "Recording migration %03d as a baseline — it is NOT executed and no table is touched.\n\n", BaselineVersion)
			if err := m.baseline(ctx, BaselineVersion, applied); err != nil {
				return err
			}
			if applied, err = m.applied(ctx); err != nil {
				return err
			}
		}
	}

	m.warnDrift(applied)

	fmt.Fprintf(m.out, "Found %d migrations\n\n", len(m.migrations))
	width := m.nameWidth()
	ran := 0
	for _, mig := range m.migrations {
		if rec, ok := applied[mig.Version]; ok {
			note := ""
			if rec.Baseline {
				note = " (baseline)"
			}
			fmt.Fprintf(m.out, "%-*s  ✓ already applied%s\n", width, mig.UpFile, note)
			continue
		}
		if adopted, err := m.adoptExistingUsersUUID(ctx, mig); err != nil {
			return err
		} else if adopted {
			ran++
			continue
		}
		fmt.Fprintf(m.out, "%-*s  → applying...\n", width, mig.UpFile)
		took, err := m.apply(ctx, mig)
		if err != nil {
			fmt.Fprintf(m.out, "%-*s  ✗ FAILED\n", width, mig.UpFile)
			return err
		}
		fmt.Fprintf(m.out, "%-*s  ✓ applied (%s)\n", width, mig.UpFile, took.Round(time.Millisecond))
		ran++
	}

	if ran == 0 {
		fmt.Fprintln(m.out, "\nNothing to migrate — database is up to date.")
	} else {
		fmt.Fprintf(m.out, "\nMigration completed successfully (%d applied).\n", ran)
	}
	return nil
}

// apply runs one migration's statements in a transaction and records it in
// the same transaction, so a failure never leaves a record behind. MySQL
// commits DDL implicitly, though, so DDL statements that ran before the
// failing one stay applied — the error says so explicitly.
func (m *Migrator) apply(ctx context.Context, mig Migration) (time.Duration, error) {
	stmts, err := SplitStatements(mig.Up)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", mig.UpFile, err)
	}
	start := time.Now()
	tx, err := m.conn.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("%s: begin transaction: %w", mig.UpFile, err)
	}
	for i, s := range stmts {
		if _, err := tx.ExecContext(ctx, s); err != nil {
			tx.Rollback()
			return 0, statementError(mig.UpFile, i, len(stmts), s, err)
		}
	}
	took := time.Since(start)
	if _, err := tx.ExecContext(ctx,
		"INSERT INTO "+Table+" (version, name, checksum, baseline, execution_ms) VALUES (?, ?, ?, 0, ?)",
		mig.Version, mig.UpFile, mig.Checksum, took.Milliseconds()); err != nil {
		tx.Rollback()
		return 0, fmt.Errorf("%s: record migration: %w", mig.UpFile, err)
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("%s: commit: %w", mig.UpFile, err)
	}
	return took, nil
}

func statementError(file string, i, total int, stmt string, err error) error {
	msg := fmt.Sprintf("%s: statement %d of %d failed: %v\n--- statement ---\n%s\n---", file, i+1, total, err, stmt)
	if i > 0 {
		msg += fmt.Sprintf("\nNOTE: the %d statement(s) before it already ran. MySQL commits DDL (CREATE/ALTER/DROP/RENAME/TRUNCATE)"+
			" immediately and cannot roll it back, so check the database before re-running.", i)
	}
	msg += "\nThe migration was NOT recorded as applied; later migrations were not run."
	return errors.New(msg)
}

// Baseline records every migration up to and including upTo as applied
// without executing it — for a database whose schema already contains
// those changes.
func (m *Migrator) Baseline(ctx context.Context, upTo uint64) error {
	if m.find(upTo) == nil {
		return fmt.Errorf("no migration with version %d", upTo)
	}
	if err := m.EnsureTable(ctx); err != nil {
		return err
	}
	applied, err := m.applied(ctx)
	if err != nil {
		return err
	}
	return m.baseline(ctx, upTo, applied)
}

func (m *Migrator) baseline(ctx context.Context, upTo uint64, applied map[uint64]Record) error {
	tx, err := m.conn.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("baseline: begin transaction: %w", err)
	}
	var marked []Migration
	for _, mig := range m.migrations {
		if mig.Version > upTo {
			break
		}
		if _, ok := applied[mig.Version]; ok {
			continue
		}
		if _, err := tx.ExecContext(ctx,
			"INSERT INTO "+Table+" (version, name, checksum, baseline) VALUES (?, ?, ?, 1)",
			mig.Version, mig.UpFile, mig.Checksum); err != nil {
			tx.Rollback()
			return fmt.Errorf("baseline %s: %w", mig.UpFile, err)
		}
		marked = append(marked, mig)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("baseline: commit: %w", err)
	}
	if len(marked) == 0 {
		fmt.Fprintf(m.out, "Nothing to baseline — every migration up to %d is already recorded.\n", upTo)
		return nil
	}
	for _, mig := range marked {
		fmt.Fprintf(m.out, "%s  ✓ recorded as baseline (not executed)\n", mig.UpFile)
	}
	return m.warnMissingTables(ctx, marked)
}

var createTableRe = regexp.MustCompile("(?i)^CREATE\\s+TABLE\\s+(?:IF\\s+NOT\\s+EXISTS\\s+)?`?(\\w+)`?")

// createdTables lists the tables a migration's CREATE TABLE statements
// define. It matches parsed statements, so comments never count.
func createdTables(sql string) []string {
	stmts, _ := SplitStatements(sql)
	var out []string
	for _, s := range stmts {
		if match := createTableRe.FindStringSubmatch(s); match != nil {
			out = append(out, match[1])
		}
	}
	return out
}

// warnMissingTables reports tables that baselined migrations define but the
// database lacks. Baselining never creates them; the operator decides.
func (m *Migrator) warnMissingTables(ctx context.Context, migs []Migration) error {
	rows, err := m.conn.QueryContext(ctx, "SELECT table_name FROM information_schema.tables WHERE table_schema = DATABASE()")
	if err != nil {
		return fmt.Errorf("list tables: %w", err)
	}
	defer rows.Close()
	have := map[string]bool{}
	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err != nil {
			return err
		}
		have[strings.ToLower(t)] = true
	}
	if err := rows.Err(); err != nil {
		return err
	}

	var missing []string
	for _, mig := range migs {
		for _, table := range createdTables(mig.Up) {
			if !have[strings.ToLower(table)] {
				missing = append(missing, table)
			}
		}
	}
	if len(missing) > 0 {
		fmt.Fprintf(m.out, "\nWARNING: the baselined schema defines %d table(s) this database does not have: %s\n", len(missing), strings.Join(missing, ", "))
		fmt.Fprintln(m.out, "They were NOT created. If the application needs them, add a new migration with their CREATE TABLE IF NOT EXISTS statements.")
	}
	fmt.Fprintln(m.out)
	return nil
}

// warnDrift flags applied migrations whose file was edited afterwards or
// no longer exists. Neither blocks a run: the database is what it is, and
// the fix is a new migration, never re-running an old one.
func (m *Migrator) warnDrift(applied map[uint64]Record) {
	for _, rec := range sortedRecords(applied) {
		mig := m.find(rec.Version)
		switch {
		case mig == nil:
			fmt.Fprintf(m.out, "WARNING: %s is recorded as applied but its file no longer exists.\n", rec.Name)
		case mig.Checksum != rec.Checksum:
			fmt.Fprintf(m.out, "WARNING: %s was edited after it was applied; the change will NOT be run. Put schema changes in a new migration.\n", mig.UpFile)
		}
	}
}

// Status prints every migration and whether it has been applied. It is
// read-only: it neither creates the history table nor takes the lock.
func (m *Migrator) Status(ctx context.Context) error {
	exists, err := m.tableExists(ctx)
	if err != nil {
		return fmt.Errorf("inspect database: %w", err)
	}
	applied := map[uint64]Record{}
	if exists {
		if applied, err = m.applied(ctx); err != nil {
			return err
		}
	}

	w := tabwriter.NewWriter(m.out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "MIGRATION\tSTATUS\tAPPLIED AT")
	pending := 0
	for _, mig := range m.migrations {
		rec, ok := applied[mig.Version]
		if !ok {
			pending++
			fmt.Fprintf(w, "%s\tpending\t\n", mig.UpFile)
			continue
		}
		status := "applied"
		if rec.Baseline {
			status += " (baseline)"
		}
		if rec.Checksum != mig.Checksum {
			status += " — file edited since"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\n", mig.UpFile, status, rec.AppliedAt.Format("2006-01-02 15:04:05"))
	}
	for _, rec := range sortedRecords(applied) {
		if m.find(rec.Version) == nil {
			fmt.Fprintf(w, "%s\tapplied — file missing\t%s\n", rec.Name, rec.AppliedAt.Format("2006-01-02 15:04:05"))
		}
	}
	w.Flush()

	fmt.Fprintf(m.out, "\n%d applied, %d pending\n", len(m.migrations)-pending, pending)
	if len(applied) == 0 && pending > 0 {
		if n, err := m.otherTables(ctx); err == nil && n > 0 && m.find(BaselineVersion) != nil {
			fmt.Fprintf(m.out, "No migration history yet and the database has %d tables: `migrate` will record %03d as a baseline instead of running it.\n", n, BaselineVersion)
		}
	}
	return nil
}

// Rollback reverts the highest applied migration using its down file. It
// refuses baselined migrations (migrate never ran them, so it has no
// business undoing them) and migrations without a down file. Without
// confirm it only prints what it would run.
func (m *Migrator) Rollback(ctx context.Context, confirm bool) error {
	if err := m.EnsureTable(ctx); err != nil {
		return err
	}
	applied, err := m.applied(ctx)
	if err != nil {
		return err
	}
	recs := sortedRecords(applied)
	if len(recs) == 0 {
		fmt.Fprintln(m.out, "Nothing to roll back — no migrations are applied.")
		return nil
	}
	rec := recs[len(recs)-1]
	mig := m.find(rec.Version)
	switch {
	case mig == nil:
		return fmt.Errorf("cannot roll back %s: its file no longer exists", rec.Name)
	case rec.Baseline:
		return fmt.Errorf("refusing to roll back %s: it was recorded as a baseline (the schema predates migrate), not applied by it", mig.UpFile)
	case mig.DownFile == "":
		return fmt.Errorf("cannot roll back %s: it has no down file (%03d_%s.down.sql). Write a new forward migration instead", mig.UpFile, mig.Version, mig.Name)
	}

	stmts, err := SplitStatements(mig.Down)
	if err != nil {
		return fmt.Errorf("%s: %w", mig.DownFile, err)
	}
	if !confirm {
		fmt.Fprintf(m.out, "Would roll back %s by running %s:\n\n", mig.UpFile, mig.DownFile)
		for _, s := range stmts {
			fmt.Fprintf(m.out, "  %s;\n\n", strings.ReplaceAll(s, "\n", "\n  "))
		}
		fmt.Fprintln(m.out, "Nothing was changed. Re-run with -yes to execute it.")
		return nil
	}

	fmt.Fprintf(m.out, "%s  → rolling back...\n", mig.DownFile)
	tx, err := m.conn.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("%s: begin transaction: %w", mig.DownFile, err)
	}
	for i, s := range stmts {
		if _, err := tx.ExecContext(ctx, s); err != nil {
			tx.Rollback()
			return statementError(mig.DownFile, i, len(stmts), s, err)
		}
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM "+Table+" WHERE version = ?", mig.Version); err != nil {
		tx.Rollback()
		return fmt.Errorf("%s: remove record: %w", mig.DownFile, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("%s: commit: %w", mig.DownFile, err)
	}
	fmt.Fprintf(m.out, "%s  ✓ rolled back\n", mig.UpFile)
	return nil
}

func (m *Migrator) find(version uint64) *Migration {
	for i := range m.migrations {
		if m.migrations[i].Version == version {
			return &m.migrations[i]
		}
	}
	return nil
}

func (m *Migrator) nameWidth() int {
	w := 0
	for _, mig := range m.migrations {
		if len(mig.UpFile) > w {
			w = len(mig.UpFile)
		}
	}
	return w
}

func sortedRecords(applied map[uint64]Record) []Record {
	out := make([]Record, 0, len(applied))
	for _, r := range applied {
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Version < out[j].Version })
	return out
}
