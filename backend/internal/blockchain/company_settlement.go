package blockchain

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"math/big"
	"regexp"
	"sort"
)

type CompanySettlementResult struct {
	Enabled  *bool  `json:"enabled"`
	Sealed   *bool  `json:"sealed"`
	Settled  *bool  `json:"settled"`
	Hash     string `json:"manifest_hash"`
	Buyer    string `json:"external_user_id"`
	Purchase string `json:"external_purchase_id"`
}

func (g *Gateway) SealFirstCompanyManifest(ctx context.Context, intent PurchaseIntent, commissions []CommissionInstruction) (*CompanySettlementResult, error) {
	price, ok := new(big.Rat).SetString(intent.Price)
	if intent.ID == 0 || intent.UserID == 0 || !ok || price.Sign() <= 0 || intent.Kind != "first" {
		return nil, ErrUnavailable
	}
	rows := make([]map[string]string, 0, len(commissions))
	total := new(big.Rat)
	ids := map[uint64]bool{}
	for _, c := range commissions {
		amount, valid := new(big.Rat).SetString(c.USD)
		if c.ID == 0 || c.EarnLogID == 0 || c.RecipientID == 0 || c.PurchaseID != intent.ID || c.BuyerID != intent.UserID || ids[c.ID] || !decimalPattern.MatchString(c.USD) || !valid || amount.Sign() <= 0 {
			return nil, ErrUnavailable
		}
		ids[c.ID] = true
		total.Add(total, amount)
		rows = append(rows, map[string]string{"external_outbox_id": fmt.Sprint(c.ID), "external_earn_log_id": fmt.Sprint(c.EarnLogID), "external_recipient_id": fmt.Sprint(c.RecipientID), "commission_kind": c.Kind, "commission_usd": c.USD})
	}
	if total.Cmp(price) > 0 || len(rows) > 20 {
		return nil, ErrUnavailable
	}
	var identityBindings interface{}
	if g.identityResolver != nil {
		wire, _, err := g.prepareUUIDWire(ctx, map[string]interface{}{"external_user_id": fmt.Sprint(intent.UserID), "commissions": rows})
		if err != nil {
			return nil, err
		}
		identityBindings = wire["identity_bindings"]
		encoded, _ := json.Marshal(wire["commissions"])
		if err := json.Unmarshal(encoded, &rows); err != nil {
			return nil, err
		}
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i]["external_outbox_id"] < rows[j]["external_outbox_id"] })
	canonical, err := json.Marshal(rows)
	if err != nil {
		return nil, err
	}
	hash := fmt.Sprintf("%x", sha256.Sum256(canonical))
	var result CompanySettlementResult
	err = g.request(ctx, "POST", "/api/v1/integration/package-purchases/company-manifest", fmt.Sprintf("signet-app:first-company-manifest:%d", intent.ID), map[string]interface{}{"source": "signet-app", "external_user_id": fmt.Sprint(intent.UserID), "external_purchase_id": fmt.Sprint(intent.ID), "commissions": rows, "identity_bindings": identityBindings}, &result)
	if err != nil {
		return nil, err
	}
	if result.Enabled == nil || result.Sealed == nil || result.Settled == nil || result.Buyer != fmt.Sprint(intent.UserID) || result.Purchase != fmt.Sprint(intent.ID) || (*result.Enabled && (!*result.Sealed || result.Hash != hash)) || (!*result.Enabled && (*result.Sealed || *result.Settled)) {
		return nil, ErrUnavailable
	}
	return &result, nil
}

// FirstCompanyReceipt reads existing settlement evidence only; it cannot dispatch a payment.
type FirstCompanyReceipt struct {
	Buyer         string `json:"external_user_id"`
	Purchase      string `json:"external_purchase_id"`
	NativeOrderID uint64 `json:"native_order_id"`
	Price         string `json:"package_price_usdt"`
	Sealed        *bool  `json:"sealed"`
	Settled       *bool  `json:"settled"`
	ManifestHash  string `json:"manifest_hash"`
	TxHash        string `json:"settlement_tx_hash"`
	USDT          string `json:"settlement_usdt"`
	ConfirmedAt   string `json:"settlement_confirmed_at"`
}

func (g *Gateway) Recovered460CompanyReceipt(ctx context.Context) (*FirstCompanyReceipt, error) {
	var result FirstCompanyReceipt
	if err := g.request(ctx, "POST", "/api/v1/integration/package-purchases/company-status", "signet-app:first-company-status:1", map[string]string{
		"source": "signet-app", "external_user_id": "460", "external_purchase_id": "1",
	}, &result); err != nil {
		return nil, err
	}
	if result.Buyer != "460" || result.Purchase != "1" || result.NativeOrderID != 5 || result.Sealed == nil || result.Settled == nil {
		return nil, ErrUnavailable
	}
	price, ok := new(big.Rat).SetString(result.Price)
	if !ok || price.Cmp(big.NewRat(1000, 1)) != 0 {
		return nil, ErrUnavailable
	}
	if *result.Settled {
		amount, valid := new(big.Rat).SetString(result.USDT)
		if !*result.Sealed || !regexp.MustCompile(`^[0-9a-f]{64}$`).MatchString(result.ManifestHash) || !regexp.MustCompile(`^0x[0-9a-fA-F]{64}$`).MatchString(result.TxHash) || result.ConfirmedAt == "" || !decimalPattern.MatchString(result.USDT) || !valid || amount.Sign() <= 0 {
			return nil, ErrUnavailable
		}
	}
	return &result, nil
}
