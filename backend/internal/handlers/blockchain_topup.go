package handlers

import (
	"database/sql"
	"net/http"
	"signet-backend/internal/app"
	"signet-backend/internal/auth"
	"signet-backend/internal/blockchain"
	"signet-backend/internal/httpx"
	"signet-backend/internal/models"
	"strconv"
)

func createBlockchainTopup(d *app.Deps, w http.ResponseWriter, r *http.Request) {
	if !d.Cfg.BlockchainTopupEnabled || d.Blockchain == nil {
		httpx.Error(w, 503, "Automatic top-ups are not enabled yet")
		return
	}
	var body struct {
		Package string `json:"package"`
	}
	if decodeJSON(r, &body) != nil {
		httpx.Error(w, 400, "Invalid request body")
		return
	}
	packageID, ok := parseUintParam(body.Package)
	if !ok || packageID == 0 {
		httpx.Error(w, 422, "Select a valid package")
		return
	}
	user := auth.UserFromContext(r.Context())
	tx, err := d.DB.Beginx()
	if err != nil {
		httpx.Error(w, 500, "Database error")
		return
	}
	defer tx.Rollback()
	var id uint64
	if tx.Get(&id, "SELECT id FROM users WHERE id=? FOR UPDATE", user.ID) != nil {
		httpx.Error(w, 500, "Database error")
		return
	}
	cap, err := activePackagesCapValueTx(tx, user.ID)
	if err != nil {
		httpx.Error(w, 500, "Database error")
		return
	}
	var balance sql.NullFloat64
	err = tx.Get(&balance, "SELECT balance FROM wallets WHERE user_id=? FOR UPDATE", user.ID)
	if err != nil && err != sql.ErrNoRows {
		httpx.Error(w, 500, "Database error")
		return
	}
	if cap <= 0 || balance.Float64 < cap {
		httpx.Error(w, 409, "Top-up is available after reaching your income cap")
		return
	}
	var count int
	if tx.Get(&count, "SELECT COUNT(*) FROM package_purchase_intents WHERE user_id=? AND status NOT IN ('completed','expired')", user.ID) != nil {
		httpx.Error(w, 500, "Database error")
		return
	}
	if count != 0 {
		httpx.Error(w, 409, "A purchase is already in progress; open your deposit wallet")
		return
	}
	var pkg models.Package
	if tx.Get(&pkg, "SELECT * FROM packages WHERE id=? AND status='active'", packageID) != nil || pkg.Price <= 0 {
		httpx.Error(w, 422, "Package is unavailable")
		return
	}
	terms, err := blockchain.CommercialPackageTerms(pkg.Price)
	if err != nil {
		httpx.Error(w, 422, "Package commercial terms are not approved")
		return
	}
	// Same nearest-active-ancestor selection used by the legacy top-up flow.
	beneficiary := uint64(1)
	current := user.ID
	seen := map[uint64]bool{current: true}
	for hop := 0; hop < 50; hop++ {
		var parentID uint64
		err = tx.Get(&parentID, "SELECT parent_id FROM user_parents WHERE user_id=? LIMIT 1", current)
		if err == sql.ErrNoRows || parentID == 0 {
			break
		}
		if err != nil {
			httpx.Error(w, 500, "Database error")
			return
		}
		if seen[parentID] {
			httpx.Error(w, 409, "Referral placement requires review")
			return
		}
		seen[parentID] = true
		var status string
		if tx.Get(&status, "SELECT status FROM users WHERE id=?", parentID) != nil {
			httpx.Error(w, 500, "Database error")
			return
		}
		if status == "active" {
			beneficiary = parentID
			break
		}
		current = parentID
	}
	var percent int64
	if beneficiary != 1 {
		err = tx.Get(&percent, `SELECT p.commission FROM user_packages up JOIN packages p ON p.id=CAST(up.package AS UNSIGNED) WHERE up.user_id=? AND up.status='active' ORDER BY p.price DESC LIMIT 1`, beneficiary)
		if err != nil && err != sql.ErrNoRows {
			httpx.Error(w, 500, "Database error")
			return
		}
	}
	result, err := tx.Exec(`INSERT INTO user_packages (user_id,package,status,ref_id,sale,created_at,updated_at) VALUES (?,?,'pending',?,'other',NOW(),NOW())`, user.ID, strconv.FormatUint(packageID, 10), beneficiary)
	if err != nil {
		httpx.Error(w, 500, "Database error")
		return
	}
	userPackageID, err := result.LastInsertId()
	if err != nil {
		httpx.Error(w, 500, "Database error")
		return
	}
	result, err = tx.Exec(`INSERT INTO package_purchase_intents (user_id,user_package_id,package_id,package_price,remaining_usdt,commission_user_id,commission_percent,purchase_kind,service_fee_usdt,required_usdt,bonus_percent) VALUES (?,?,?,?,?,?,?,'topup',?,?,0)`, user.ID, userPackageID, packageID, pkg.Price, terms.Required, beneficiary, percent, terms.Fee, terms.Required)
	if err != nil {
		httpx.Error(w, 500, "Database error")
		return
	}
	intentID, err := result.LastInsertId()
	if err != nil || tx.Commit() != nil {
		httpx.Error(w, 500, "Database error")
		return
	}
	httpx.OK(w, map[string]interface{}{"status": "success", "blockchain_enabled": true, "purchase_kind": "topup", "purchase_intent_id": intentID, "message": "Deposit package amount plus service fee to your own wallet. No purchaser bonus or vesting is created."})
}
