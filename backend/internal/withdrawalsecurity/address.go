package withdrawalsecurity

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"regexp"
	"strings"
	"time"
)

var bscAddressPattern = regexp.MustCompile(`^0x[0-9a-fA-F]{40}$`)
var ErrWithdrawalAddress = errors.New("invalid or already recorded withdrawal address")

// Enrollment is scoped to SIGNET on BSC. Never reuse email proof for an
// address mutation; its target binds the exact normalized network/asset/address.
func addressChallengeTarget(address string) (string, error) {
	if !bscAddressPattern.MatchString(address) || strings.EqualFold(address, "0x"+strings.Repeat("0", 40)) {
		return "", ErrWithdrawalAddress
	}
	digest := sha256.Sum256([]byte("BSC|SIGNET|" + strings.ToLower(address)))
	return hex.EncodeToString(digest[:]), nil
}

func NewAddressEnrollmentChallenge(key []byte, userID, version uint64, email, address string, now time.Time) (Challenge, string, error) {
	target, err := addressChallengeTarget(address)
	if err != nil {
		return Challenge{}, "", err
	}
	c, code, err := NewEmailChallenge(key, userID, version, email, now)
	if err != nil {
		return Challenge{}, "", err
	}
	c.Purpose, c.Target = AddressEnrollmentPurpose, target
	c.Hash = c.codeHash(key, code)
	return c, code, nil
}

func (c Challenge) AcceptsAddress(key []byte, userID, version uint64, email, address, code string, now time.Time) bool {
	target, err := addressChallengeTarget(address)
	return err == nil && c.acceptsAction(key, userID, version, email, code, now, AddressEnrollmentPurpose, target)
}

// Matches the native token address waiting period. The enrollment transaction
// must persist this alongside verified_at only after consuming email and TOTP.
func addressEligibleAt(verifiedAt time.Time) time.Time {
	return verifiedAt.UTC().Add(24 * time.Hour)
}

// Revocation is a distinct action bound to an owner-scoped saved address ID.
// Enrollment or email-verification codes cannot authorize it.
func NewAddressRevocationChallenge(key []byte, userID, version uint64, email, addressID string, now time.Time) (Challenge, string, error) {
	if !challengeIDPattern.MatchString(addressID) {
		return Challenge{}, "", ErrWithdrawalAddress
	}
	c, code, err := NewEmailChallenge(key, userID, version, email, now)
	if err != nil {
		return Challenge{}, "", err
	}
	c.Purpose, c.Target = AddressRevocationPurpose, addressID
	c.Hash = c.codeHash(key, code)
	return c, code, nil
}

func (c Challenge) AcceptsAddressRevocation(key []byte, userID, version uint64, email, addressID, code string, now time.Time) bool {
	return challengeIDPattern.MatchString(addressID) && c.acceptsAction(key, userID, version, email, code, now, AddressRevocationPurpose, addressID)
}
