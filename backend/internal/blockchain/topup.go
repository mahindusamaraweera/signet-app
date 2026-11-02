package blockchain

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sort"
)

type TopupCommission struct {
	ID        uint64
	Recipient uint64
	Kind      string
	USD       string
}

func (g *Gateway) SealTopupManifest(ctx context.Context, intent PurchaseIntent, plans []TopupCommission) error {
	rows := make([]map[string]string, 0, len(plans))
	for _, plan := range plans {
		if plan.ID == 0 || plan.Recipient == 0 || !decimalPattern.MatchString(plan.USD) {
			return ErrUnavailable
		}
		identity := fmt.Sprintf("topup-plan:%d", plan.ID)
		rows = append(rows, map[string]string{"external_outbox_id": identity, "external_earn_log_id": identity, "external_recipient_id": fmt.Sprint(plan.Recipient), "commission_kind": plan.Kind, "commission_usd": plan.USD})
	}
	var result struct {
		Sealed   bool   `json:"sealed"`
		Hash     string `json:"manifest_hash"`
		Buyer    string `json:"external_user_id"`
		Purchase string `json:"external_purchase_id"`
	}
	var identityBindings interface{}
	if g.identityResolver != nil {
		wire, _, err := g.prepareUUIDWire(ctx, map[string]interface{}{"external_user_id": fmt.Sprint(intent.UserID), "commissions": rows})
		if err != nil {
			return err
		}
		identityBindings = wire["identity_bindings"]
		encoded, _ := json.Marshal(wire["commissions"])
		if err := json.Unmarshal(encoded, &rows); err != nil {
			return err
		}
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i]["external_outbox_id"] < rows[j]["external_outbox_id"] })
	canonical, err := json.Marshal(rows)
	if err != nil {
		return err
	}
	expectedHash := fmt.Sprintf("%x", sha256.Sum256(canonical))
	err = g.request(ctx, "POST", "/api/v1/integration/topups/commission-manifest", fmt.Sprintf("signet-app:topup-manifest:%d", intent.ID), map[string]interface{}{"source": "signet-app", "external_user_id": fmt.Sprint(intent.UserID), "external_purchase_id": fmt.Sprint(intent.ID), "commissions": rows, "identity_bindings": identityBindings}, &result)
	if err != nil {
		return err
	}
	if !result.Sealed || result.Hash != expectedHash || result.Buyer != fmt.Sprint(intent.UserID) || result.Purchase != fmt.Sprint(intent.ID) {
		return ErrUnavailable
	}
	return nil
}
