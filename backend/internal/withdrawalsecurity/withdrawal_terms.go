package withdrawalsecurity

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math/big"
	"regexp"
	"strings"
)

var withdrawalAmountPattern = regexp.MustCompile(`^[0-9]{1,18}(\.[0-9]{1,18})?$`)

type WithdrawalTerms struct {
	UserID          uint64 `json:"user_id"`
	SecurityVersion uint64 `json:"security_version"`
	AddressID       string `json:"address_id"`
	Destination     string `json:"destination"`
	Network         string `json:"network"`
	Asset           string `json:"asset"`
	Gross           string `json:"gross"`
	Fee             string `json:"fee"`
	Net             string `json:"net"`
}

// Integer base-unit arithmetic matches token Money's 18-decimal truncation.
// The 3% is deducted once, never added on top of the gross withdrawal.
func NewWithdrawalTerms(userID, version uint64, addressID, destination, gross string) (WithdrawalTerms, error) {
	if userID == 0 || version == 0 || !challengeIDPattern.MatchString(addressID) || !withdrawalAmountPattern.MatchString(gross) {
		return WithdrawalTerms{}, errors.New("invalid withdrawal terms")
	}
	if _, err := addressChallengeTarget(destination); err != nil {
		return WithdrawalTerms{}, err
	}
	parts := strings.SplitN(gross, ".", 2)
	fraction := ""
	if len(parts) == 2 {
		fraction = parts[1]
	}
	units, ok := new(big.Int).SetString(parts[0]+fraction+strings.Repeat("0", 18-len(fraction)), 10)
	if !ok || units.Sign() <= 0 {
		return WithdrawalTerms{}, errors.New("withdrawal must be positive")
	}
	fee := new(big.Int).Quo(new(big.Int).Mul(new(big.Int).Set(units), big.NewInt(3)), big.NewInt(100))
	net := new(big.Int).Sub(new(big.Int).Set(units), fee)
	return WithdrawalTerms{UserID: userID, SecurityVersion: version, AddressID: addressID, Destination: strings.ToLower(destination), Network: "BSC", Asset: "SIGNET", Gross: formatWithdrawalUnits(units), Fee: formatWithdrawalUnits(fee), Net: formatWithdrawalUnits(net)}, nil
}

func formatWithdrawalUnits(units *big.Int) string {
	digits := units.String()
	if len(digits) <= 18 {
		digits = strings.Repeat("0", 19-len(digits)) + digits
	}
	return digits[:len(digits)-18] + "." + digits[len(digits)-18:]
}

// This is an immutable-terms digest, NOT authorization. The bridge must require
// stored consumed user proof, current security state and token financial audit.
func (t WithdrawalTerms) Digest() string {
	encoded, _ := json.Marshal(t)
	digest := sha256.Sum256(append([]byte("signet-withdrawal-terms-v1|"), encoded...))
	return hex.EncodeToString(digest[:])
}
