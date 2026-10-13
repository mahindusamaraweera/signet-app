package withdrawalsecurity

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestAddressListScopesOwnerAndProjectsPublicEvidence(t *testing.T) {
	now := time.Unix(1800000000, 0).UTC()
	fixture := &sqlFixture{steps: []sqlStep{{match: "WHERE user_id=?", values: []driver.Value{strings.Repeat("a", 64), "BSC", "SIGNET", "0x" + strings.Repeat("b", 40), now, now.Add(24 * time.Hour), now}, check: func(args []driver.NamedValue) error {
		if len(args) != 1 || args[0].Value != int64(42) {
			return errors.New("wrong owner")
		}
		return nil
	}}}}
	db := sql.OpenDB(fixture)
	defer db.Close()
	s, err := NewService(db, []byte(strings.Repeat("k", 32)))
	if err != nil {
		t.Fatal(err)
	}
	addresses, err := s.ListAddresses(context.Background(), 42)
	if err != nil || len(addresses) != 1 || addresses[0].RevokedAt == nil || !addresses[0].EligibleAt.Equal(now.Add(24*time.Hour)) {
		t.Fatalf("invalid list: %v", err)
	}
	encoded, _ := json.Marshal(addresses)
	if strings.Contains(string(encoded), "security_version") || strings.Contains(string(encoded), "secret") || fixture.committed || fixture.rolledBack || len(fixture.steps) != 0 {
		t.Fatal("private evidence or mutation")
	}
}
