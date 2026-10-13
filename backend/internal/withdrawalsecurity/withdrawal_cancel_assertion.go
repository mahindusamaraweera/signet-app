package withdrawalsecurity

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"time"
)

// Caller must validate a durable owner-authorized cancellation intent first.
// Cancellation signature cannot be substituted for a reservation or fresh
// broadcast proof. It authorizes only cancellation/tombstone, never payout.
func signWithdrawalCancellation(a WithdrawalAuthorization, key []byte, now time.Time) (WithdrawalAssertion, string, error) {
	assertion, _, err := signWithdrawalAssertion(a, key, now)
	if err != nil {
		return WithdrawalAssertion{}, "", err
	}
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte("signet-external-withdrawal-cancel-v1\n" + assertion.payload()))
	return assertion, hex.EncodeToString(mac.Sum(nil)), nil
}
