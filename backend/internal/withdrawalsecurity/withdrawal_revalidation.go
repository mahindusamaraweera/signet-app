package withdrawalsecurity

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"time"
)

type WithdrawalRevalidation struct {
	Assertion          WithdrawalAssertion `json:"assertion"`
	RemoteWithdrawalID string              `json:"remote_withdrawal_id"`
	Nonce              string              `json:"nonce"`
	Signature          string              `json:"signature"`
}

// Private signing primitive. Caller must revalidate submitted queue/current
// security and exact remote identity under locks for EACH custody challenge.
// Never sign this from a cached reservation assertion or expose to browsers.
func signWithdrawalRevalidation(a WithdrawalAuthorization, remoteID uint64, nonce string, key []byte, now time.Time) (WithdrawalRevalidation, error) {
	if remoteID == 0 || !challengeIDPattern.MatchString(nonce) {
		return WithdrawalRevalidation{}, ErrWithdrawalAuthorization
	}
	assertion, _, err := signWithdrawalAssertion(a, key, now)
	if err != nil {
		return WithdrawalRevalidation{}, err
	}
	assertion.ExpiresAt = strconv.FormatInt(now.Add(15*time.Second).Unix(), 10)
	id := strconv.FormatUint(remoteID, 10)
	payload := "signet-external-withdrawal-revalidation-v1\n" + id + "\n" + nonce + "\n" + assertion.payload()
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(payload))
	return WithdrawalRevalidation{Assertion: assertion, RemoteWithdrawalID: id, Nonce: nonce, Signature: hex.EncodeToString(mac.Sum(nil))}, nil
}
