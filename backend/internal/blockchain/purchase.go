package blockchain

import (
	"context"
	"fmt"
	"math/big"
	"regexp"
)

type PurchaseIntent struct {
	Status            string `db:"status" json:"-"`
	BonusPercent      int64  `db:"bonus_percent"`
	Fee               string `db:"service_fee_usdt"`
	Required          string `db:"required_usdt"`
	Kind              string `db:"purchase_kind" json:"purchase_kind"`
	PackageID         uint64 `db:"package_id" json:"package_id"`
	ID                uint64 `db:"id" json:"id"`
	UserID            uint64 `db:"user_id" json:"user_id"`
	UserPackageID     uint64 `db:"user_package_id" json:"user_package_id"`
	Price             string `db:"package_price" json:"package_price"`
	CommissionUserID  uint64 `db:"commission_user_id" json:"-"`
	CommissionPercent int64  `db:"commission_percent" json:"-"`
}

type PurchaseResult struct {
	Kind                string `json:"purchase_kind"`
	SettlementConfirmed bool   `json:"settlement_confirmed"`
	Required            string `json:"required_usdt"`
	ServiceFee          string `json:"service_fee_usdt"`
	Status              string `json:"status"`
	Available           string `json:"available_usdt"`
	Remaining           string `json:"remaining_usdt"`
	DepositSeen         *bool  `json:"deposit_seen"`
	OrderID             string `json:"order_id"`
	SwapConfirmed       bool   `json:"swap_confirmed"`
	VestingAllocated    bool   `json:"vesting_allocated"`
}

var decimalPattern = regexp.MustCompile(`^[0-9]{1,18}(\.[0-9]{1,18})?$`)

// FundingRemaining uses decimal arithmetic without floating-point rounding.
func FundingRemaining(price, available string) (string, error) {
	if !decimalPattern.MatchString(price) || !decimalPattern.MatchString(available) {
		return "", fmt.Errorf("invalid USDT amount")
	}
	p, _ := new(big.Rat).SetString(price)
	a, _ := new(big.Rat).SetString(available)
	if p.Sign() <= 0 {
		return "", fmt.Errorf("package price must be positive")
	}
	r := new(big.Rat).Sub(p, a)
	if r.Sign() < 0 {
		r.SetInt64(0)
	}
	return r.FloatString(18), nil
}

func (g *Gateway) SyncPurchase(ctx context.Context, intent PurchaseIntent) (*PurchaseResult, error) {
	kind := intent.Kind
	if kind == "" {
		kind = "first"
	}
	if kind != "first" && kind != "topup" {
		return nil, fmt.Errorf("%w: unknown purchase kind %q", ErrUnavailable, kind)
	}
	custody := "user_purchase_vesting"
	if kind == "topup" {
		custody = "treasury_topup_settlement"
	}
	var result PurchaseResult
	payload := map[string]interface{}{
		"source": "signet-app", "external_user_id": fmt.Sprint(intent.UserID), "external_purchase_id": fmt.Sprint(intent.ID),
		"package_price_usdt": intent.Price, "swap_asset": "SIGNET", "swap_percent": 100,
		"package_id":     fmt.Sprint(intent.PackageID),
		"purchase_kind":  kind,
		"custody_policy": custody, "auto_execute": true, "run_mlm": false,
	}
	if intent.Required != "" {
		if !decimalPattern.MatchString(intent.Price) || !decimalPattern.MatchString(intent.Fee) || !decimalPattern.MatchString(intent.Required) || intent.BonusPercent < 0 || (kind == "topup" && intent.BonusPercent != 0) {
			return nil, fmt.Errorf("%w: intent %d has malformed commercial terms (price %q, fee %q, required %q, bonus %d)", ErrUnavailable, intent.ID, intent.Price, intent.Fee, intent.Required, intent.BonusPercent)
		}
		price, _ := new(big.Rat).SetString(intent.Price)
		fee, _ := new(big.Rat).SetString(intent.Fee)
		total, _ := new(big.Rat).SetString(intent.Required)
		if price.Sign() <= 0 || new(big.Rat).Add(price, fee).Cmp(total) != 0 {
			return nil, fmt.Errorf("%w: intent %d terms don't add up: price %s + fee %s != required %s", ErrUnavailable, intent.ID, intent.Price, intent.Fee, intent.Required)
		}
		payload["service_fee_usdt"] = intent.Fee
		payload["required_usdt"] = intent.Required
		payload["bonus_percent"] = intent.BonusPercent
	}
	err := g.request(ctx, "POST", "/api/v1/integration/package-purchases/sync", fmt.Sprintf("signet-app:purchase:%d", intent.ID), payload, &result)
	if err != nil {
		return nil, err
	}
	if intent.Required != "" && (result.Required != intent.Required || result.ServiceFee != intent.Fee) {
		// SQL DECIMAL and the remote representation may differ in trailing zeros.
		expected, ok := new(big.Rat).SetString(intent.Required)
		actual, actualOK := new(big.Rat).SetString(result.Required)
		fee, feeOK := new(big.Rat).SetString(intent.Fee)
		remoteFee, remoteOK := new(big.Rat).SetString(result.ServiceFee)
		if !ok || !actualOK || !feeOK || !remoteOK || expected.Cmp(actual) != 0 || fee.Cmp(remoteFee) != 0 {
			return nil, fmt.Errorf("%w: intent %d: service terms differ from ours (required %q vs %q, fee %q vs %q)", ErrUnavailable, intent.ID, result.Required, intent.Required, result.ServiceFee, intent.Fee)
		}
	}
	required := intent.Price
	if result.Required != "" {
		if !decimalPattern.MatchString(result.Required) || !decimalPattern.MatchString(result.ServiceFee) {
			return nil, fmt.Errorf("%w: intent %d: service sent malformed amounts (required %q, fee %q)", ErrUnavailable, intent.ID, result.Required, result.ServiceFee)
		}
		p, ok := new(big.Rat).SetString(intent.Price)
		if !ok {
			return nil, fmt.Errorf("%w: intent %d: invalid package price %q", ErrUnavailable, intent.ID, intent.Price)
		}
		fee, _ := new(big.Rat).SetString(result.ServiceFee)
		total, _ := new(big.Rat).SetString(result.Required)
		if new(big.Rat).Add(p, fee).Cmp(total) != 0 {
			return nil, fmt.Errorf("%w: intent %d: service terms don't add up: price %s + fee %s != required %s", ErrUnavailable, intent.ID, intent.Price, result.ServiceFee, result.Required)
		}
		required = result.Required
	}
	remaining, err := FundingRemaining(required, result.Available)
	if err != nil || result.DepositSeen == nil {
		return nil, fmt.Errorf("%w: intent %d: unusable funding data from service (available %q, deposit_seen present: %v, err: %v)", ErrUnavailable, intent.ID, result.Available, result.DepositSeen != nil, err)
	}
	available, _ := new(big.Rat).SetString(result.Available)
	if available.Sign() > 0 {
		*result.DepositSeen = true
	}
	switch result.Status {
	case "awaiting_deposit":
		result.Remaining = remaining
	case "reserved", "pending_sweep", "pending_swap", "pending_vesting", "retrying":
		result.Remaining = "0.000000000000000000"
	case "awaiting_commission_settlement":
		if kind != "topup" || result.Kind != kind || !result.SwapConfirmed || result.VestingAllocated {
			return nil, fmt.Errorf("%w: intent %d: inconsistent awaiting_commission_settlement (kind %q/%q, swap_confirmed %v, vesting_allocated %v)", ErrUnavailable, intent.ID, kind, result.Kind, result.SwapConfirmed, result.VestingAllocated)
		}
		result.Remaining = "0.000000000000000000"
	case "completed":
		if !result.SwapConfirmed || result.OrderID == "" || (kind == "first" && !result.VestingAllocated) || (kind == "topup" && (result.Kind != kind || !result.SettlementConfirmed || result.VestingAllocated)) {
			return nil, fmt.Errorf("%w: intent %d: service says completed but is missing confirmations (swap_confirmed %v, order_id %q, vesting_allocated %v, settlement_confirmed %v)", ErrUnavailable, intent.ID, result.SwapConfirmed, result.OrderID, result.VestingAllocated, result.SettlementConfirmed)
		}
		result.Remaining = "0.000000000000000000"
	default:
		return nil, fmt.Errorf("%w: intent %d: unknown status %q from service", ErrUnavailable, intent.ID, result.Status)
	}
	return &result, nil
}
