package blockchain

import (
	"context"
	"encoding/hex"
	"strconv"

	"signet-backend/internal/withdrawalsecurity"
)

type WithdrawalReservation struct {
	ID             string `json:"id"`
	ExternalUserID string `json:"external_user_id"`
	Status         string `json:"status"`
	Asset          string `json:"asset"`
	Amount         string `json:"amount"`
	Fee            string `json:"fee"`
	NetAmount      string `json:"net_amount"`
	Destination    string `json:"to_address"`
}

// A response is not trusted until its owner, amounts and destination exactly
// match the signed approval. Stable authorization identity survives retries.
func (g *Gateway) ReserveWithdrawal(ctx context.Context, a withdrawalsecurity.WithdrawalAssertion, signature string) (*WithdrawalReservation, error) {
	userID, err := strconv.ParseUint(a.ExternalUserID, 10, 64)
	if err != nil || userID == 0 || strconv.FormatUint(userID, 10) != a.ExternalUserID {
		return nil, ErrRejected
	}
	version, err := strconv.ParseUint(a.SecurityVersion, 10, 64)
	if err != nil {
		return nil, ErrRejected
	}
	terms, err := withdrawalsecurity.NewWithdrawalTerms(userID, version, a.AddressID, a.Destination, a.Gross)
	if err != nil || terms.Gross != a.Gross || terms.Fee != a.Fee || terms.Net != a.Net || terms.Network != a.Network || terms.Asset != a.Asset || terms.Destination != a.Destination {
		return nil, ErrRejected
	}
	for _, value := range []string{a.AuthorizationID, signature} {
		decoded, err := hex.DecodeString(value)
		if err != nil || len(decoded) != 32 || hex.EncodeToString(decoded) != value {
			return nil, ErrRejected
		}
	}
	var result WithdrawalReservation
	err = g.request(ctx, "POST", "/api/v1/integration/withdrawals/reserve", "signet-app:withdrawal:"+a.AuthorizationID,
		map[string]interface{}{"source": "signet-app", "external_user_id": a.ExternalUserID, "assertion": a, "signature": signature}, &result)
	if err != nil {
		return nil, err
	}
	id, err := strconv.ParseUint(result.ID, 10, 64)
	if err != nil || id == 0 || strconv.FormatUint(id, 10) != result.ID || result.ExternalUserID != a.ExternalUserID || result.Status != "external_security_pending" || result.Asset != a.Asset || result.Amount != a.Gross || result.Fee != a.Fee || result.NetAmount != a.Net || result.Destination != a.Destination {
		return nil, ErrUnavailable
	}
	return &result, nil
}
