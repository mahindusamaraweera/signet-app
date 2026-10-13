package withdrawalsecurity

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"time"
)

func withdrawalChallengeTarget(a WithdrawalAuthorization) (string, error) {
	canonical, err := NewWithdrawalTerms(a.Terms.UserID, a.Terms.SecurityVersion, a.Terms.AddressID, a.Terms.Destination, a.Terms.Gross)
	if err != nil || canonical != a.Terms || !challengeIDPattern.MatchString(a.ID) {
		return "", errors.New("invalid withdrawal challenge terms")
	}
	digest := sha256.Sum256([]byte("withdrawal-email-target-v1|" + a.ID + "|" + a.Terms.Digest()))
	return hex.EncodeToString(digest[:]), nil
}

func NewWithdrawalChallenge(key []byte, email string, a WithdrawalAuthorization, now time.Time) (Challenge, string, error) {
	target, err := withdrawalChallengeTarget(a)
	if err != nil || a.Status != "pending" || a.ConsumedAt != nil || a.AuthorizedAt != nil || !now.Before(a.ExpiresAt) {
		return Challenge{}, "", ErrInvalidChallenge
	}
	c, code, err := NewEmailChallenge(key, a.Terms.UserID, a.Terms.SecurityVersion, email, now)
	if err != nil {
		return Challenge{}, "", err
	}
	// Sending or resending a code must never extend the prepared request.
	if a.ExpiresAt.Before(c.ExpiresAt) {
		c.ExpiresAt = a.ExpiresAt
	}
	c.Purpose, c.Target = WithdrawalPurpose, target
	c.Hash = c.codeHash(key, code)
	return c, code, nil
}

func (c Challenge) AcceptsWithdrawal(key []byte, email, code string, a WithdrawalAuthorization, now time.Time) bool {
	target, err := withdrawalChallengeTarget(a)
	return err == nil && a.Status == "pending" && a.AuthorizedAt == nil && a.ConsumedAt == nil && now.Before(a.ExpiresAt) && c.acceptsAction(key, a.Terms.UserID, a.Terms.SecurityVersion, email, code, now, WithdrawalPurpose, target)
}
