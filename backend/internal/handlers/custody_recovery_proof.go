package handlers

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"regexp"
	"signet-backend/internal/app"
	"signet-backend/internal/blockchain"
	"time"
)

type CustodyRecoveryEvidence struct {
	DiscoverDeposit  bool   `json:"discover_deposit"`
	UserID           uint64 `json:"user_id"`
	IntentID         uint64 `json:"intent_id"`
	OldCustodyUserID uint64 `json:"old_custody_user_id"`
	OldOrderID       uint64 `json:"old_order_id"`
	DepositHash      string `json:"deposit_hash"`
	DepositAmount    string `json:"deposit_amount"`
	OldAddress       string `json:"old_address"`
}

// Read-only proof of the pending production purchase. The operator must stop
// the API and disable staging first; this command never queues or pays anything.
func CustodyRecoveryProof(d *app.Deps, e CustodyRecoveryEvidence) ([]byte, error) {
	if d.Cfg.AppEnv != "production" {
		return nil, fmt.Errorf("recovery requires APP_ENV=production; loaded APP_ENV=%q", d.Cfg.AppEnv)
	}
	if len(d.Cfg.BlockchainServiceToken) < 32 {
		return nil, fmt.Errorf("recovery requires BLOCKCHAIN_SERVICE_TOKEN of at least 32 bytes; loaded %d bytes (secret hidden)", len(d.Cfg.BlockchainServiceToken))
	}
	if e.UserID == 0 || e.IntentID == 0 {
		return nil, fmt.Errorf("production user and intent are required")
	}
	if e.DiscoverDeposit && (e.OldCustodyUserID != 0 || e.OldOrderID != 0 || e.DepositHash != "" || e.DepositAmount != "" || e.OldAddress != "") {
		return nil, fmt.Errorf("discovery mode cannot override custody evidence")
	}
	if !e.DiscoverDeposit && (e.OldCustodyUserID == 0 || e.OldOrderID == 0 || !regexp.MustCompile(`^0x[0-9a-f]{64}$`).MatchString(e.DepositHash) || !regexp.MustCompile(`^0x[0-9a-f]{40}$`).MatchString(e.OldAddress)) {
		return nil, fmt.Errorf("invalid recovery evidence")
	}
	amount, ok := new(big.Rat).SetString(e.DepositAmount)
	if !e.DiscoverDeposit && (!ok || amount.Sign() <= 0) {
		return nil, fmt.Errorf("invalid deposit amount")
	}
	tx, err := d.DB.Beginx()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var user struct {
		ID     uint64 `db:"id"`
		Status string `db:"status"`
		UUID   string `db:"uuid"`
	}
	if err := tx.Get(&user, "SELECT id,status,uuid FROM users WHERE id=? FOR UPDATE", e.UserID); err != nil {
		return nil, err
	}
	if !regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`).MatchString(user.UUID) {
		return nil, fmt.Errorf("missing/invalid production customer UUID")
	}
	if user.Status == "active" {
		return nil, fmt.Errorf("user already active; reconcile existing purchase instead")
	}
	var row struct {
		blockchain.PurchaseIntent
		PackageOwner     uint64 `db:"package_owner"`
		PackageStatus    string `db:"package_status"`
		CurrentPackageID uint64 `db:"current_package_id"`
	}
	if err := tx.Get(&row, `SELECT p.id,p.user_id,p.user_package_id,p.purchase_kind,p.status,p.package_price,COALESCE(p.package_id,CAST(up.package AS UNSIGNED)) AS package_id,CAST(up.package AS UNSIGNED) AS current_package_id,COALESCE(CAST(p.service_fee_usdt AS CHAR),'') AS service_fee_usdt,COALESCE(CAST(p.required_usdt AS CHAR),'') AS required_usdt,COALESCE(p.bonus_percent,0) AS bonus_percent,up.user_id AS package_owner,up.status AS package_status FROM package_purchase_intents p JOIN user_packages up ON up.id=p.user_package_id WHERE p.id=? FOR UPDATE`, e.IntentID); err != nil {
		return nil, err
	}
	price, valid := new(big.Rat).SetString(row.Price)
	if row.UserID != e.UserID || row.PackageOwner != e.UserID || row.Kind != "first" || row.Status != "awaiting_deposit" || row.PackageStatus != "pending" || row.PackageID != row.CurrentPackageID || !valid || !price.IsInt() || !price.Num().IsInt64() || price.Cmp(big.NewRat(1, 1)) <= 0 {
		return nil, fmt.Errorf("purchase is not an unchanged pending production first purchase")
	}
	terms, err := blockchain.CommercialPackageTerms(price.Num().Int64())
	if err != nil {
		return nil, err
	}
	equal := func(a, b string) bool {
		x, o := new(big.Rat).SetString(a)
		y, v := new(big.Rat).SetString(b)
		return o && v && x.Cmp(y) == 0
	}
	required, _ := new(big.Rat).SetString(terms.Required)
	if !equal(row.Fee, terms.Fee) || !equal(row.Required, terms.Required) || row.BonusPercent != terms.BonusPercent || (!e.DiscoverDeposit && amount.Cmp(required) < 0) {
		return nil, fmt.Errorf("package/deposit commercial terms do not match")
	}
	var count int
	if err := tx.Get(&count, "SELECT COUNT(*) FROM blockchain_commission_outbox WHERE purchase_intent_id=?", e.IntentID); err != nil {
		return nil, err
	}
	if count != 0 {
		return nil, fmt.Errorf("purchase already has commission evidence; automatic identity recovery refused")
	}
	if err := tx.Get(&count, "SELECT COUNT(*) FROM package_purchase_intents WHERE user_id=? AND id<>? AND status NOT IN ('expired')", e.UserID, e.IntentID); err != nil {
		return nil, err
	}
	if count != 0 {
		return nil, fmt.Errorf("other purchase history requires manual reconciliation")
	}
	payload, err := json.Marshal(map[string]interface{}{"user_uuid": user.UUID, "version": "custody-collision-v1", "environment": "production", "database": d.Cfg.DBDatabase, "generated_at": time.Now().UTC().Format(time.RFC3339), "evidence": e, "package_id": row.PackageID, "price_usdt": row.Price, "service_fee_usdt": row.Fee, "required_usdt": row.Required, "bonus_percent": row.BonusPercent, "purchase_kind": "first", "status": row.Status, "commission_outbox_count": 0})
	if err != nil {
		return nil, err
	}
	mac := hmac.New(sha256.New, []byte(d.Cfg.BlockchainServiceToken))
	mac.Write(payload)
	return json.MarshalIndent(map[string]string{"payload": base64.StdEncoding.EncodeToString(payload), "signature": hex.EncodeToString(mac.Sum(nil))}, "", "  ")
}
