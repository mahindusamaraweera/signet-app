package withdrawalsecurity

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"
)

type WithdrawalAuthorization struct {
	ID           string
	Terms        WithdrawalTerms
	ProofHash    string
	Status       string
	ExpiresAt    time.Time
	AuthorizedAt *time.Time
	ConsumedAt   *time.Time
}

// Created pending, not authorized. SQL confirmation must consume fresh email
// and TOTP and revalidate current address/security state before setting status.
func NewWithdrawalAuthorization(key []byte, terms WithdrawalTerms, now time.Time) (WithdrawalAuthorization, string, error) {
	canonical, err := NewWithdrawalTerms(terms.UserID, terms.SecurityVersion, terms.AddressID, terms.Destination, terms.Gross)
	if err != nil || canonical != terms || len(key) < 32 {
		return WithdrawalAuthorization{}, "", errors.New("invalid withdrawal authorization terms")
	}
	id, proof := make([]byte, 32), make([]byte, 32)
	if _, err := rand.Read(id); err != nil {
		return WithdrawalAuthorization{}, "", err
	}
	if _, err := rand.Read(proof); err != nil {
		return WithdrawalAuthorization{}, "", err
	}
	// Whole seconds: expires_at is a TIMESTAMP(0) that MySQL rounds on insert,
	// and ExpiresAt.Unix() is part of the proof hash below.
	a := WithdrawalAuthorization{ID: hex.EncodeToString(id), Terms: terms, Status: "pending", ExpiresAt: now.UTC().Add(10 * time.Minute).Truncate(time.Second)}
	secret := hex.EncodeToString(proof)
	a.ProofHash = a.hash(key, secret)
	return a, secret, nil
}

func (a WithdrawalAuthorization) hash(key []byte, proof string) string {
	mac := hmac.New(sha256.New, key)
	fmt.Fprintf(mac, "signet-withdrawal-authorization-v1|%s|%s|%d|%s", a.ID, a.Terms.Digest(), a.ExpiresAt.Unix(), proof)
	return hex.EncodeToString(mac.Sum(nil))
}

// Validates a locked snapshot only. Callers MUST compare current security state,
// consume atomically and retain token's financial audit before any broadcast.
func (a WithdrawalAuthorization) CanConsume(key []byte, proof string, expected WithdrawalTerms, now time.Time) bool {
	canonical, err := NewWithdrawalTerms(expected.UserID, expected.SecurityVersion, expected.AddressID, expected.Destination, expected.Gross)
	return err == nil && canonical == expected && a.Terms == expected && len(key) >= 32 && challengeIDPattern.MatchString(a.ID) && challengeIDPattern.MatchString(proof) && a.Status == "authorized" && a.AuthorizedAt != nil && !a.AuthorizedAt.After(now) && a.AuthorizedAt.Before(a.ExpiresAt) && a.ConsumedAt == nil && now.Before(a.ExpiresAt) && hmac.Equal([]byte(a.ProofHash), []byte(a.hash(key, proof)))
}
