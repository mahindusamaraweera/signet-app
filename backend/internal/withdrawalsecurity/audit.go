package withdrawalsecurity

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"time"
)

// Security events commit with the mutation, never best-effort afterward.
// Intentionally no arbitrary metadata: credentials, OTP and secrets cannot be
// supplied. Financial ledger audit remains separately required before payout.
func recordAudit(ctx context.Context, tx *sql.Tx, userID, version uint64, event, reference string, now time.Time) error {
	if tx == nil || userID == 0 || version == 0 || !challengeIDPattern.MatchString(reference) || !supportedWithdrawalAuditEvent(event) {
		return errors.New("invalid withdrawal audit event")
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO withdrawal_security_audit (user_id,event,reference_id,security_version,created_at) VALUES (?,?,?,?,?)`, userID, event, reference, version, now.UTC())
	return err
}

func supportedWithdrawalAuditEvent(event string) bool {
	switch event {
	case "address_verified", "address_revoked", "authenticator_confirmed", "email_verified", "withdrawal_prepared", "withdrawal_email_requested", "withdrawal_authorized", "withdrawal_consumed", "withdrawal_queued", "withdrawal_submitted", "withdrawal_cancel_requested", "withdrawal_cancelled_local", "withdrawal_cancelled", "withdrawal_cancel_review":
		return true
	default:
		return false
	}
}

// A non-secret stable reference for a versioned security change.
func securityChangeReference(userID, version uint64, event string) string {
	digest := sha256.Sum256([]byte(fmt.Sprintf("withdrawal-security-change-v1|%d|%d|%s", userID, version, event)))
	return hex.EncodeToString(digest[:])
}
