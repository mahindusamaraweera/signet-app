package blockchain

import (
	"context"
	"fmt"
	"strconv"
	"time"
)

type WithdrawalAdmission struct {
	KycRequired        *bool      `json:"kyc_required"`
	KycVerified        *bool      `json:"kyc_verified"`
	ServerTime         time.Time  `json:"server_time"`
	CooldownMinutes    *int       `json:"cooldown_minutes"`
	CooldownUntil      *time.Time `json:"cooldown_until"`
	ActiveWithdrawalID *string    `json:"active_withdrawal_id"`
	CanRequest         *bool      `json:"can_request"`
}

// Authoritative token policy, read only. Missing or inconsistent fields never
// imply that another withdrawal can be requested.
func (g *Gateway) WithdrawalAdmission(ctx context.Context, userID uint64) (WithdrawalAdmission, error) {
	var state WithdrawalAdmission
	err := g.request(ctx, "POST", "/api/v1/integration/withdrawals/admission", fmt.Sprintf("signet-app:withdrawal-admission:%d", userID), map[string]interface{}{"source": "signet-app", "external_user_id": fmt.Sprint(userID)}, &state)
	if err != nil {
		return state, err
	}
	if state.ServerTime.IsZero() || state.CooldownMinutes == nil || *state.CooldownMinutes < 0 || state.CanRequest == nil {
		return state, ErrUnavailable
	}
	if state.ActiveWithdrawalID != nil {
		id, err := strconv.ParseUint(*state.ActiveWithdrawalID, 10, 64)
		if err != nil || id == 0 || strconv.FormatUint(id, 10) != *state.ActiveWithdrawalID {
			return state, ErrUnavailable
		}
	}
	if state.KycRequired == nil || state.KycVerified == nil {
		return state, ErrUnavailable
	}
	blocked := (state.KycRequired != nil && *state.KycRequired && !*state.KycVerified) || state.ActiveWithdrawalID != nil || (state.CooldownUntil != nil && state.CooldownUntil.After(state.ServerTime))
	if *state.CanRequest == blocked {
		return state, ErrUnavailable
	}
	return state, nil
}
