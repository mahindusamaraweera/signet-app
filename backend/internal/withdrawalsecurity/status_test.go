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

func TestSecurityStatusRejectsStaleOrAlteredEnrollmentEvidence(t *testing.T) {
	for _, scenario := range []string{"valid", "changed email", "legacy secret changed", "wrong fingerprint", "corrupt ciphertext", "missing profile", "conflicting confirmation times"} {
		t.Run(scenario, func(t *testing.T) {
			key := []byte(strings.Repeat("k", 32))
			now := time.Unix(1800000000, 0).UTC()
			secret := "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ"
			encrypted, err := encryptEnrollment(key, 42, 1, "user@example.com", secret)
			if err != nil {
				t.Fatal(err)
			}
			values := []driver.Value{"user@example.com", nil, "user@example.com", now, secretFingerprint(key, secret), now, false, now.Add(48 * time.Hour),
				encrypted, secretFingerprint(key, ""), "user@example.com", int64(1), now}
			switch scenario {
			case "changed email":
				values[0] = "new@example.com"
			case "legacy secret changed":
				values[1] = "NEWLEGACYSECRET"
			case "wrong fingerprint":
				values[4] = strings.Repeat("0", 64)
			case "corrupt ciphertext":
				values[8] = "malformed"
			case "missing profile":
				for i := 2; i < len(values); i++ {
					values[i] = nil
				}
			case "conflicting confirmation times":
				values[12] = now.Add(time.Second)
			}
			fixture := &sqlFixture{steps: []sqlStep{{match: "LEFT JOIN withdrawal_security_profiles", values: values, check: func(args []driver.NamedValue) error {
				if len(args) != 1 || args[0].Value != int64(42) {
					return errors.New("wrong status identity")
				}
				return nil
			}}}}
			db := sql.OpenDB(fixture)
			defer db.Close()
			service, err := NewService(db, key)
			if err != nil {
				t.Fatal(err)
			}
			status, err := service.Status(context.Background(), 42)
			if scenario == "corrupt ciphertext" {
				if err == nil {
					t.Fatal("corrupt enrollment accepted")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if status.AuthenticatorConfirmed != (scenario == "valid") {
				t.Fatal("invalid authenticator confirmation reported")
			}
			if status.SubmissionEnabled {
				t.Fatal("status enabled unfinished withdrawals")
			}
			if status.EmailVerified != (scenario != "changed email" && scenario != "missing profile") {
				t.Fatal("invalid email verification reported")
			}
			encoded, _ := json.Marshal(status)
			if strings.Contains(string(encoded), secret) || strings.Contains(string(encoded), encrypted) || strings.Contains(string(encoded), secretFingerprint(key, secret)) {
				t.Fatal("private enrollment evidence leaked")
			}
			if len(fixture.steps) != 0 || fixture.committed || fixture.rolledBack {
				t.Fatal("status was not read-only")
			}
		})
	}
}
