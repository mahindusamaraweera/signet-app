package blockchain

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"time"
)

// Identity status comes only from the linked Tier account, never legacy KYC flags.
type TierKycStatus struct {
	ExternalUserID     string     `json:"external_user_id"`
	Linked             *bool      `json:"linked"`
	TierUserID         *string    `json:"tier_user_id"`
	Status             string     `json:"status"`
	Verified           *bool      `json:"verified"`
	WithdrawalRequired *bool      `json:"withdrawal_required"`
	Environment        string     `json:"environment"`
	Level              string     `json:"level"`
	CheckedAt          time.Time  `json:"checked_at"`
	ValidUntil         *time.Time `json:"valid_until"`
	VerificationPath   *string    `json:"verification_path"`
	VerificationURL    string     `json:"verification_url,omitempty"`
}

var kycPathPattern = regexp.MustCompile(`^/identity-verification\?account=[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

func (g *Gateway) TierKyc(ctx context.Context, userID uint64) (*TierKycStatus, error) {
	var state TierKycStatus
	if err := g.request(ctx, "POST", "/api/v1/integration/tier-link/kyc", fmt.Sprintf("signet-app:tier-kyc:%d", userID), map[string]string{
		"source": "signet-app", "external_user_id": fmt.Sprint(userID),
	}, &state); err != nil {
		return nil, err
	}
	if err := state.validate(userID, time.Now()); err != nil {
		return nil, err
	}
	if state.VerificationPath != nil {
		// Only an exact relative verification route on the configured Tier service.
		target, err := url.Parse(g.baseURL + *state.VerificationPath)
		if err != nil {
			return nil, ErrUnavailable
		}
		state.VerificationURL = target.String()
	}
	return &state, nil
}

func (s *TierKycStatus) validate(userID uint64, now time.Time) error {
	if s.ExternalUserID != fmt.Sprint(userID) || s.Linked == nil || s.Verified == nil || s.WithdrawalRequired == nil || s.Level == "" ||
		(s.Environment != "production" && s.Environment != "sandbox") || s.CheckedAt.IsZero() ||
		s.CheckedAt.Before(now.Add(-2*time.Minute)) || s.CheckedAt.After(now.Add(time.Minute)) {
		return ErrUnavailable
	}
	switch s.Status {
	case "not_linked", "not_started", "pending", "in_review", "approved", "checking", "resubmission_required", "rejected", "unavailable":
	default:
		return ErrUnavailable
	}
	if *s.Linked {
		if s.TierUserID == nil || !tierIDPattern.MatchString(*s.TierUserID) || s.VerificationPath == nil || !kycPathPattern.MatchString(*s.VerificationPath) || s.Status == "not_linked" {
			return ErrUnavailable
		}
	} else if s.TierUserID != nil || s.VerificationPath != nil || *s.Verified || s.Status != "not_linked" {
		return ErrUnavailable
	}
	if *s.Verified && (!*s.Linked || s.Status != "approved" || s.Environment != "production" || s.ValidUntil == nil || !s.ValidUntil.After(now) || s.ValidUntil.After(s.CheckedAt.Add(time.Hour+time.Minute))) {
		return ErrUnavailable
	}
	return nil
}
