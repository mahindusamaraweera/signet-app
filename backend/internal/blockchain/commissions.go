package blockchain

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"regexp"
)

type CommissionInstruction struct {
	ID          uint64 `db:"id"`
	EarnLogID   uint64 `db:"earn_log_id"`
	PurchaseID  uint64 `db:"purchase_intent_id"`
	BuyerID     uint64 `db:"buyer_id"`
	RecipientID uint64 `db:"beneficiary_user_id"`
	Kind        string `db:"commission_kind"`
	USD         string `db:"commission_usd"`
}

type CommissionPreparation struct {
	ID               string          `json:"id"`
	OutboxID         string          `json:"external_outbox_id"`
	EarnLogID        string          `json:"external_earn_log_id"`
	RecipientID      string          `json:"external_recipient_id"`
	Kind             string          `json:"commission_kind"`
	USD              string          `json:"commission_usd"`
	RecipientAddress string          `json:"recipient_address"`
	Status           string          `json:"status"`
	Quote            json.RawMessage `json:"quote"`
}

var commissionAddress = regexp.MustCompile(`^0x[0-9a-fA-F]{40}$`)

type CommissionDelivery struct {
	ID               string          `json:"id"`
	BuyerID          string          `json:"external_user_id"`
	PurchaseID       string          `json:"external_purchase_id"`
	OutboxID         string          `json:"external_outbox_id"`
	Status           string          `json:"status"`
	Quote            json.RawMessage `json:"quote"`
	RecipientAddress string          `json:"recipient_address"`
	TxHash           string          `json:"tx_hash"`
	ConfirmedAt      *string         `json:"confirmed_at"`
}

func (g *Gateway) AuthorizeCommission(ctx context.Context, body CommissionInstruction) (*CommissionDelivery, error) {
	var result CommissionDelivery
	err := g.request(ctx, "POST", "/api/v1/integration/commissions/authorize", fmt.Sprintf("signet-app:commission-authorize:%d", body.ID), map[string]interface{}{
		"source": "signet-app", "external_user_id": fmt.Sprint(body.BuyerID),
		"external_purchase_id": fmt.Sprint(body.PurchaseID), "external_outbox_id": fmt.Sprint(body.ID),
	}, &result)
	if err != nil {
		return nil, err
	}
	if result.ID == "" || result.BuyerID != fmt.Sprint(body.BuyerID) || result.PurchaseID != fmt.Sprint(body.PurchaseID) || result.OutboxID != fmt.Sprint(body.ID) || !commissionAddress.MatchString(result.RecipientAddress) {
		return nil, ErrUnavailable
	}
	if result.Status != "authorized" && result.Status != "confirming" && result.Status != "confirmed" {
		return nil, ErrUnavailable
	}
	var quote struct {
		Price  string `json:"signet_price_usdt"`
		Amount string `json:"signet_amount"`
	}
	if json.Unmarshal(result.Quote, &quote) != nil || !decimalPattern.MatchString(quote.Price) || !decimalPattern.MatchString(quote.Amount) {
		return nil, ErrUnavailable
	}
	price, _ := new(big.Rat).SetString(quote.Price)
	amount, _ := new(big.Rat).SetString(quote.Amount)
	if price.Sign() <= 0 || amount.Sign() <= 0 {
		return nil, ErrUnavailable
	}
	if (result.Status == "confirming" || result.Status == "confirmed") && !regexp.MustCompile(`^0x[0-9a-fA-F]{64}$`).MatchString(result.TxHash) {
		return nil, ErrUnavailable
	}
	if result.Status == "confirmed" && (result.ConfirmedAt == nil || *result.ConfirmedAt == "") {
		return nil, ErrUnavailable
	}
	return &result, nil
}

func (g *Gateway) PrepareCommission(ctx context.Context, body CommissionInstruction) (*CommissionPreparation, error) {
	var result CommissionPreparation
	err := g.request(ctx, "POST", "/api/v1/integration/commissions/prepare", fmt.Sprintf("signet-app:commission-prepare:%d", body.ID), map[string]interface{}{
		"source": "signet-app", "external_user_id": fmt.Sprint(body.BuyerID), "external_outbox_id": fmt.Sprint(body.ID),
		"external_earn_log_id": fmt.Sprint(body.EarnLogID), "external_purchase_id": fmt.Sprint(body.PurchaseID),
		"external_recipient_id": fmt.Sprint(body.RecipientID), "commission_kind": body.Kind, "commission_usd": body.USD,
	}, &result)
	if err != nil {
		return nil, err
	}
	if result.ID == "" || result.OutboxID != fmt.Sprint(body.ID) || result.EarnLogID != fmt.Sprint(body.EarnLogID) || result.RecipientID != fmt.Sprint(body.RecipientID) || result.Kind != body.Kind || result.Status != "awaiting_authorization" || !commissionAddress.MatchString(result.RecipientAddress) {
		return nil, ErrUnavailable
	}
	usd, ok := new(big.Rat).SetString(result.USD)
	expected, expectedOK := new(big.Rat).SetString(body.USD)
	if !decimalPattern.MatchString(result.USD) || !ok || !expectedOK || usd.Cmp(expected) != 0 || usd.Sign() <= 0 {
		return nil, ErrUnavailable
	}
	var quote struct {
		Price  string `json:"signet_price_usdt"`
		Amount string `json:"signet_amount"`
	}
	if json.Unmarshal(result.Quote, &quote) != nil || !decimalPattern.MatchString(quote.Price) || !decimalPattern.MatchString(quote.Amount) {
		return nil, ErrUnavailable
	}
	price, _ := new(big.Rat).SetString(quote.Price)
	amount, _ := new(big.Rat).SetString(quote.Amount)
	if price.Sign() <= 0 || amount.Sign() <= 0 {
		return nil, ErrUnavailable
	}
	return &result, nil
}
