package withdrawalsecurity

import (
	"database/sql"
	"fmt"
	"time"
)

// EligibilityError contains only fixed, user-safe diagnostics. Never include
// stored credentials, SQL errors, enrollment ciphertext or verification codes.
type EligibilityError struct {
	Code, Message string
	AvailableAt   string
	cause         error
}

func (e *EligibilityError) Error() string { return e.Message }
func (e *EligibilityError) Unwrap() error { return e.cause }
func eligibility(code, message string, cause error) error {
	return &EligibilityError{Code: code, Message: message, cause: cause}
}

func waitingEligibility(code, message string, deadline time.Time, cause error) error {
	return &EligibilityError{Code: code, Message: message, AvailableAt: deadline.UTC().Format(time.RFC3339), cause: cause}
}

func withdrawalProfileEligibility(email string, verified sql.NullString, verifiedAt, confirmed, hold sql.NullTime, frozen bool, now time.Time) error {
	switch {
	case frozen:
		return eligibility("withdrawal_account_frozen", "Withdrawals are frozen for this account. Contact support for review.", ErrSecurityCredentials)
	case !verifiedAt.Valid || verified.String != email:
		return eligibility("withdrawal_email_verification_required", "Verify your current email in Withdrawal security before requesting a withdrawal.", ErrSecurityCredentials)
	case !confirmed.Valid:
		return eligibility("withdrawal_authenticator_setup_required", "Complete withdrawal authenticator setup in Withdrawal security.", ErrSecurityCredentials)
	case hold.Valid && now.Before(hold.Time):
		return waitingEligibility("withdrawal_security_wait", fmt.Sprintf("Your account security waiting period ends at %s. Check status after that time.", hold.Time.UTC().Format("2006-01-02 15:04:05 UTC")), hold.Time, ErrSecurityCredentials)
	}
	return nil
}
func withdrawalEnrollmentEligibility(confirmed, enrolledConfirmed sql.NullTime, email, enrolledEmail string, version uint64, previous, expectedPrevious string) error {
	if !enrolledConfirmed.Valid {
		return eligibility("withdrawal_authenticator_setup_required", "Complete withdrawal authenticator setup in Withdrawal security.", ErrSecurityCredentials)
	}
	if !confirmed.Time.Equal(enrolledConfirmed.Time) || email != enrolledEmail || version == 0 || previous != expectedPrevious {
		return eligibility("withdrawal_authenticator_changed", "Your withdrawal authenticator setup no longer matches your account security settings. Open Withdrawal security to review the setup; contact support if it still shows confirmed.", ErrSecurityCredentials)
	}
	return nil
}
func withdrawalAddressEligibility(verified, eligible time.Time, revoked sql.NullTime, now time.Time) error {
	if revoked.Valid || verified.IsZero() {
		return eligibility("withdrawal_address_unverified", "This withdrawal address is revoked or unverified. Select a verified address in Withdrawal security.", ErrWithdrawalAddress)
	}
	if now.Before(eligible) {
		return waitingEligibility("withdrawal_address_wait", fmt.Sprintf("This address becomes eligible at %s. Check status after that time.", eligible.UTC().Format("2006-01-02 15:04:05 UTC")), eligible, ErrWithdrawalAddress)
	}
	return nil
}
