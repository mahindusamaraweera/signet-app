package withdrawalsecurity

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"
)

const EmailPurpose = "verify_email"
const DeviceEnrollmentPurpose = "enroll_device"
const AddressEnrollmentPurpose = "enroll_address"
const AddressRevocationPurpose = "revoke_address"
const WithdrawalPurpose = "authorize_withdrawal"

type Challenge struct {
	ID              string
	UserID          uint64
	Purpose         string
	Target          string
	Email           string
	Hash            string
	SecurityVersion uint64
	Status          string
	Attempts        uint8
	ExpiresAt       time.Time
	ConsumedAt      *time.Time
}

func NewEmailChallenge(key []byte, userID, version uint64, email string, now time.Time) (Challenge, string, error) {
	if len(key) < 32 || userID == 0 || version == 0 || email == "" || strings.ContainsAny(email, "\r\n|") {
		return Challenge{}, "", errors.New("invalid email challenge configuration")
	}
	id := make([]byte, 32)
	if _, err := rand.Read(id); err != nil {
		return Challenge{}, "", err
	}
	number, err := rand.Int(rand.Reader, big.NewInt(1000000))
	if err != nil {
		return Challenge{}, "", err
	}
	code := fmt.Sprintf("%06d", number.Int64())
	c := Challenge{ID: hex.EncodeToString(id), UserID: userID, Purpose: EmailPurpose, Email: email, SecurityVersion: version,
		Status: "pending_delivery", ExpiresAt: now.UTC().Add(10 * time.Minute)}
	c.Hash = c.codeHash(key, code)
	return c, code, nil
}

func (c Challenge) codeHash(key []byte, code string) string {
	mac := hmac.New(sha256.New, key)
	fmt.Fprintf(mac, "withdrawal-security-v1|%s|%d|%s|%s|%d|%s", c.ID, c.UserID, c.Purpose, c.Email, c.SecurityVersion, code)
	if c.Purpose != EmailPurpose {
		fmt.Fprintf(mac, "|target:%s", c.Target)
	}
	return hex.EncodeToString(mac.Sum(nil))
}

// Accepts only validates a locked challenge snapshot. The SQL caller must
// persist every attempted verification and consume a success atomically with
// its security-profile update, while holding the current user's lock.
func (c Challenge) Accepts(key []byte, userID, version uint64, currentEmail, code string, now time.Time) bool {
	return c.acceptsAction(key, userID, version, currentEmail, code, now, EmailPurpose, "")
}

func NewDeviceEnrollmentChallenge(key []byte, userID, version uint64, email, deviceID string, now time.Time) (Challenge, string, error) {
	if !challengeIDPattern.MatchString(deviceID) {
		return Challenge{}, "", errors.New("invalid device challenge target")
	}
	c, code, err := NewEmailChallenge(key, userID, version, email, now)
	if err != nil {
		return Challenge{}, "", err
	}
	c.Purpose, c.Target = DeviceEnrollmentPurpose, deviceID
	c.Hash = c.codeHash(key, code)
	return c, code, nil
}

func (c Challenge) acceptsAction(key []byte, userID, version uint64, currentEmail, code string, now time.Time, purpose, target string) bool {
	if (purpose != EmailPurpose && purpose != DeviceEnrollmentPurpose && purpose != AddressEnrollmentPurpose && purpose != AddressRevocationPurpose && purpose != WithdrawalPurpose) || len(key) < 32 || c.UserID != userID || c.SecurityVersion != version || c.Email != currentEmail || c.Purpose != purpose || c.Target != target ||
		c.Status != "delivered" || c.ConsumedAt != nil || c.Attempts >= 5 || !now.Before(c.ExpiresAt) || len(code) != 6 {
		return false
	}
	for _, digit := range code {
		if digit < '0' || digit > '9' {
			return false
		}
	}
	return hmac.Equal([]byte(c.Hash), []byte(c.codeHash(key, code)))
}
