package migrate

import (
	"reflect"
	"strings"
	"testing"
	"testing/fstest"

	"signet-backend/migrations"
)

func TestSplitStatements(t *testing.T) {
	src := "-- header comment\nSET NAMES utf8mb4;\n" +
		"# hash comment\n" +
		"-- comment right before a statement\nCREATE TABLE a (x VARCHAR(10) DEFAULT 'a;b');\n" +
		"/* block ; comment */ INSERT INTO a VALUES ('it''s; fine'), (\"q\\\";\"), (`c;`);\n" +
		"/*!40101 SET x = 1 */;\n" +
		"SELECT 1--1;\n" +
		"-- trailing comment only\n"
	got, err := SplitStatements(src)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"SET NAMES utf8mb4",
		"CREATE TABLE a (x VARCHAR(10) DEFAULT 'a;b')",
		"INSERT INTO a VALUES ('it''s; fine'), (\"q\\\";\"), (`c;`)",
		"/*!40101 SET x = 1 */",
		"SELECT 1--1",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q\nwant %q", got, want)
	}
}

func TestSplitStatementsErrors(t *testing.T) {
	for _, src := range []string{
		"SELECT 'unterminated;",
		"SELECT 1 /* unterminated",
		"DELIMITER $$\nCREATE PROCEDURE p() BEGIN SELECT 1; END$$",
	} {
		if _, err := SplitStatements(src); err == nil {
			t.Errorf("expected error for %q", src)
		}
	}
}

func TestLoad(t *testing.T) {
	fsys := fstest.MapFS{
		"002_add_phone.up.sql":   {Data: []byte("ALTER TABLE users ADD COLUMN phone VARCHAR(50);\r\n")},
		"002_add_phone.down.sql": {Data: []byte("ALTER TABLE users DROP COLUMN phone;")},
		"001_initial.sql":        {Data: []byte("CREATE TABLE t (id INT);")},
		"migrations.go":          {Data: []byte("package migrations")},
		"README.md":              {Data: []byte("docs")},
	}
	migs, err := Load(fsys)
	if err != nil {
		t.Fatal(err)
	}
	if len(migs) != 2 || migs[0].Version != 1 || migs[1].Version != 2 {
		t.Fatalf("unexpected migrations %+v", migs)
	}
	if migs[1].UpFile != "002_add_phone.up.sql" || migs[1].DownFile != "002_add_phone.down.sql" || migs[0].DownFile != "" {
		t.Fatalf("files not paired: %+v", migs)
	}
	if strings.Contains(migs[1].Up, "\r") {
		t.Fatal("CRLF not normalised")
	}
}

func TestLoadRejects(t *testing.T) {
	cases := map[string]fstest.MapFS{
		"duplicate version": {
			"001_a.sql": {Data: []byte("SELECT 1;")},
			"001_b.sql": {Data: []byte("SELECT 1;")},
		},
		"plain and up": {
			"001_a.sql":    {Data: []byte("SELECT 1;")},
			"001_a.up.sql": {Data: []byte("SELECT 1;")},
		},
		"down without up":       {"001_a.down.sql": {Data: []byte("SELECT 1;")}},
		"down with plain up":    {"001_a.sql": {Data: []byte("SELECT 1;")}, "001_a.down.sql": {Data: []byte("SELECT 1;")}},
		"bad name":              {"1-add-thing.sql": {Data: []byte("SELECT 1;")}},
		"uppercase name":        {"001_AddThing.sql": {Data: []byte("SELECT 1;")}},
		"version zero":          {"000_a.sql": {Data: []byte("SELECT 1;")}},
		"comments only (empty)": {"001_a.sql": {Data: []byte("-- nothing yet\n")}},
	}
	for name, fsys := range cases {
		if _, err := Load(fsys); err == nil {
			t.Errorf("%s: expected error", name)
		}
	}
}

// The real embedded migrations must always load, and 001 must split into
// every CREATE TABLE (the old splitter silently dropped password_reset).
func TestEmbeddedMigrations(t *testing.T) {
	migs, err := Load(migrations.FS)
	if err != nil {
		t.Fatal(err)
	}
	if len(migs) == 0 || migs[0].Version != BaselineVersion {
		t.Fatalf("first migration must be version %d", BaselineVersion)
	}
	stmts, err := SplitStatements(migs[0].Up)
	if err != nil {
		t.Fatal(err)
	}
	if len(stmts) != 37 {
		t.Fatalf("split 001 into %d statements, want 37 (3 SET + 34 CREATE TABLE)", len(stmts))
	}
	tables := createdTables(migs[0].Up)
	found := false
	for _, table := range tables {
		found = found || table == "password_reset"
	}
	if len(tables) != 34 || !found {
		t.Fatalf("001 defines %d tables %v, want 34 including password_reset", len(tables), tables)
	}
}

func TestNewRegistrationExpiryMigrationNeverEnrollsOrMutatesExistingUsers(t *testing.T) {
	content, err := migrations.FS.ReadFile("030_new_registration_expiry.sql")
	if err != nil {
		t.Fatal(err)
	}
	statements, err := SplitStatements(string(content))
	if err != nil {
		t.Fatal(err)
	}
	if len(statements) != 2 {
		t.Fatalf("unexpected statements: %d", len(statements))
	}
	for _, statement := range statements {
		if !strings.HasPrefix(statement, "CREATE TABLE IF NOT EXISTS registration_") {
			t.Fatalf("expiry migration must only create empty audit/enrollment tables: %s", statement)
		}
		if strings.Contains(statement, "REFERENCES users") {
			t.Fatal("expiry evidence must survive account deletion")
		}
	}
}
