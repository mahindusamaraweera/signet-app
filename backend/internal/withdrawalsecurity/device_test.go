package withdrawalsecurity

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestDeviceCredentialIsRandomAndBoundToAccount(t *testing.T) {
	s := &Service{key: []byte(strings.Repeat("k", 32))}
	first, err := randomDeviceValue()
	if err != nil {
		t.Fatal(err)
	}
	second, err := randomDeviceValue()
	if err != nil {
		t.Fatal(err)
	}
	if !challengeIDPattern.MatchString(first) || first == second {
		t.Fatal("invalid device credentials")
	}
	if s.deviceCredentialHash(42, first) == s.deviceCredentialHash(43, first) || s.deviceCredentialHash(42, first) == s.deviceCredentialHash(42, second) {
		t.Fatal("credential not bound to account and token")
	}
	if s.deviceCredentialHash(42, first) == first {
		t.Fatal("plaintext credential persisted")
	}
}

func TestDeviceRecordNeverGrantsOrResetsTrust(t *testing.T) {
	for _, scenario := range []string{"new", "renew", "revoked", "record limit"} {
		t.Run(scenario, func(t *testing.T) {
			now := time.Unix(1800000000, 0).UTC()
			credential := strings.Repeat("a", 64)
			fixture := &sqlFixture{steps: []sqlStep{{match: "SELECT id FROM users WHERE id=? FOR UPDATE", values: []driver.Value{int64(42)}}}}
			var storedHash string
			if scenario == "new" || scenario == "record limit" {
				credential = ""
				count := int64(0)
				if scenario == "record limit" {
					count = 50
				}
				fixture.steps = append(fixture.steps, sqlStep{match: "SELECT COUNT(*) FROM withdrawal_security_devices", values: []driver.Value{count}})
				if scenario == "new" {
					fixture.steps = append(fixture.steps, sqlStep{match: "INSERT INTO withdrawal_security_devices (id,user_id,credential_hash,last_seen_at,created_at)", check: func(args []driver.NamedValue) error {
						storedHash = args[2].Value.(string)
						return nil
					}})
				}
			} else {
				var revoked driver.Value
				if scenario == "revoked" {
					revoked = now
				}
				fixture.steps = append(fixture.steps,
					sqlStep{match: "WHERE user_id=? AND credential_hash=? FOR UPDATE", values: []driver.Value{strings.Repeat("b", 64), now, now.Add(24 * time.Hour), revoked}},
					sqlStep{match: "UPDATE withdrawal_security_devices SET last_seen_at=?"},
				)
			}
			db := sql.OpenDB(fixture)
			defer db.Close()
			service, err := NewService(db, []byte(strings.Repeat("k", 32)))
			if err != nil {
				t.Fatal(err)
			}
			device, returned, err := service.RecordDevice(context.Background(), 42, credential, now)
			if scenario == "record limit" {
				if err == nil || returned != "" || !fixture.rolledBack || fixture.committed {
					t.Fatal("record limit bypassed")
				}
			} else {
				if err != nil || !fixture.committed || fixture.rolledBack {
					t.Fatalf("record failed: %v", err)
				}
				if scenario == "new" {
					if device.VerifiedAt != nil || device.EligibleAt != nil || storedHash != service.deviceCredentialHash(42, returned) {
						t.Fatal("new device gained trust or credential mismatch")
					}
				} else if returned != credential || device.VerifiedAt == nil || device.EligibleAt == nil || !device.EligibleAt.Equal(now.Add(24*time.Hour)) || device.Revoked != (scenario == "revoked") {
					t.Fatal("renewal changed credential or security evidence")
				}
			}
			if len(fixture.steps) != 0 {
				t.Fatal(errors.New("device SQL steps not executed"))
			}
		})
	}
}
