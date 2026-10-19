package withdrawalsecurity

import (
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestWithdrawalEligibilityDiagnosticsPreserveAllSecurityGates(t *testing.T) {
	now := time.Unix(1800000000, 0).UTC()
	at := sql.NullTime{Time: now, Valid: true}
	verified := sql.NullString{String: "user@example.com", Valid: true}
	cases := []struct {
		code string
		err  error
	}{
		{"withdrawal_account_frozen", withdrawalProfileEligibility(verified.String, verified, at, at, sql.NullTime{}, true, now)},
		{"withdrawal_email_verification_required", withdrawalProfileEligibility("new@example.com", verified, at, at, sql.NullTime{}, false, now)},
		{"withdrawal_authenticator_setup_required", withdrawalProfileEligibility(verified.String, verified, at, sql.NullTime{}, sql.NullTime{}, false, now)},
		{"withdrawal_security_wait", withdrawalProfileEligibility(verified.String, verified, at, at, sql.NullTime{Time: now.Add(time.Hour), Valid: true}, false, now)},
		{"withdrawal_authenticator_changed", withdrawalEnrollmentEligibility(at, at, verified.String, verified.String, 1, "old", "changed")},
		{"withdrawal_address_unverified", withdrawalAddressEligibility(now, now, at, now)},
		{"withdrawal_address_wait", withdrawalAddressEligibility(now, now.Add(time.Hour), sql.NullTime{}, now)},
	}
	for _, c := range cases {
		var diagnostic *EligibilityError
		if !errors.As(c.err, &diagnostic) || diagnostic.Code != c.code || diagnostic.Message == "" {
			t.Fatalf("missing safe diagnostic for %s", c.code)
		}
		if !errors.Is(c.err, ErrSecurityCredentials) && !errors.Is(c.err, ErrWithdrawalAddress) {
			t.Fatal("lost original security gate")
		}
		if strings.Contains(c.code, "wait") && !strings.Contains(diagnostic.Message, now.Add(time.Hour).Format("2006-01-02 15:04:05 UTC")) {
			t.Fatal("missing precise wait deadline")
		}
	}
	if err := withdrawalProfileEligibility(verified.String, verified, at, at, at, false, now); err != nil {
		t.Fatal("elapsed hold still blocked", err)
	}
	if err := withdrawalAddressEligibility(now, now, sql.NullTime{}, now); err != nil {
		t.Fatal("eligible address blocked", err)
	}
}
