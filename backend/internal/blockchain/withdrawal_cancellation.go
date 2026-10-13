package blockchain

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"signet-backend/internal/withdrawalsecurity"
	"strconv"
)

type WithdrawalCancellation struct {
	AuthorizationID string  `json:"authorization_id"`
	Status          string  `json:"status"`
	WithdrawalID    *string `json:"withdrawal_id"`
}

var ErrCancellationReviewRequired = errors.New("withdrawal cancellation requires reconciliation")

func (g *Gateway) CancelWithdrawal(ctx context.Context, a withdrawalsecurity.WithdrawalAssertion, signature string) (*WithdrawalCancellation, error) {
	owner, err := strconv.ParseUint(a.ExternalUserID, 10, 64)
	if err != nil || owner == 0 || strconv.FormatUint(owner, 10) != a.ExternalUserID {
		return nil, ErrRejected
	}
	version, err := strconv.ParseUint(a.SecurityVersion, 10, 64)
	if err != nil {
		return nil, ErrRejected
	}
	terms, err := withdrawalsecurity.NewWithdrawalTerms(owner, version, a.AddressID, a.Destination, a.Gross)
	if err != nil || terms.Gross != a.Gross || terms.Fee != a.Fee || terms.Net != a.Net || terms.Asset != a.Asset || terms.Network != a.Network || terms.Destination != a.Destination {
		return nil, ErrRejected
	}
	for _, value := range []string{a.AuthorizationID, signature} {
		decoded, err := hex.DecodeString(value)
		if err != nil || len(decoded) != 32 || hex.EncodeToString(decoded) != value {
			return nil, ErrRejected
		}
	}
	var wire struct {
		AuthorizationID string          `json:"authorization_id"`
		Status          string          `json:"status"`
		WithdrawalID    json.RawMessage `json:"withdrawal_id"`
	}
	err = g.request(ctx, "POST", "/api/v1/integration/withdrawals/cancel", "signet-app:withdrawal-cancel:"+a.AuthorizationID, map[string]interface{}{"source": "signet-app", "external_user_id": a.ExternalUserID, "assertion": a, "signature": signature}, &wire)
	if err != nil {
		if errors.Is(err, ErrConflict) {
			return nil, ErrCancellationReviewRequired
		}
		return nil, err
	}
	result := WithdrawalCancellation{AuthorizationID: wire.AuthorizationID, Status: wire.Status}
	if len(wire.WithdrawalID) == 0 || json.Unmarshal(wire.WithdrawalID, &result.WithdrawalID) != nil {
		return nil, ErrUnavailable
	}
	if result.AuthorizationID != a.AuthorizationID || result.Status != "cancelled" {
		return nil, ErrUnavailable
	}
	if result.WithdrawalID != nil {
		id, err := strconv.ParseUint(*result.WithdrawalID, 10, 64)
		if err != nil || id == 0 || strconv.FormatUint(id, 10) != *result.WithdrawalID {
			return nil, ErrUnavailable
		}
	}
	return &result, nil
}
