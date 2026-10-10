// Package wallet ports the financial engine documented in
// docs/analysis/financial_engine.md verbatim, including the two divergent
// "credit a wallet" implementations the original app used
// (WalletService.updateWallet, cap-gated; and TokenController's private
// updateWallet, not cap-gated) — preserved as two separate functions here,
// each wired to the exact call sites the original used. Do not merge them
// into one "fixed" implementation; see ARCHITECTURE.md for why.
package wallet

import (
	"database/sql"
	"math"

	"github.com/jmoiron/sqlx"
)

// packageMultipliers is the hardcoded Global Share Wallet max_out multiplier
// map, duplicated verbatim in three places in the original source
// (AuthController::processStep2, TokenController::updateWallet,
// WalletService::creditWallet). All values are 1.5 today; kept as a map,
// not a constant, in case the tiers diverge later.
var packageMultipliers = map[int64]float64{
	5000: 1.5, 10000: 1.5, 25000: 1.5, 50000: 1.5, 100000: 1.5, 500000: 1.5, 1000000: 1.5,
}

// Balance returns a user's wallets.balance, or 0 if they have no wallet row
// yet (walletBalance() in the original helpers.php).
func Balance(db *sqlx.DB, userID uint64) (float64, error) {
	return balanceFrom(db, userID)
}

type capReader interface {
	Get(dest interface{}, query string, args ...interface{}) error
}

func balanceFrom(db capReader, userID uint64) (float64, error) {
	var balance sql.NullFloat64
	err := db.Get(&balance, "SELECT balance FROM wallets WHERE user_id = ?", userID)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return balance.Float64, nil
}

// activePackagesCapValue sums (packages.price * 4) across a user's ACTIVE
// user_packages rows — the "4x earnings cap" ceiling used throughout the
// wallet engine (WalletService.updateWallet's totalValue, checkWalet's
// totalValue, TokenController's totalValue in activePackage).
func activePackagesCapValue(db *sqlx.DB, userID uint64) (float64, error) {
	return activePackagesCapFrom(db, userID)
}

func activePackagesCapFrom(db capReader, userID uint64) (float64, error) {
	var total sql.NullFloat64
	err := db.Get(&total, `
		SELECT COALESCE(SUM(p.price * 4), 0)
		FROM user_packages up
		JOIN packages p ON p.id = CAST(up.package AS UNSIGNED)
		WHERE up.user_id = ? AND up.status = 'active'`, userID)
	if err != nil {
		return 0, err
	}
	return total.Float64, nil
}

// CheckWallet ports checkWalet($user_id, $package_id): returns 1 if crediting
// this user with the commission on packageID would NOT exceed their 4x cap,
// 0 otherwise. See financial_engine.md §2.
func CheckWallet(db *sqlx.DB, userID, packageID uint64) (int, error) {
	return checkWalletFrom(db, userID, packageID)
}

// CheckWalletTx preserves the legacy query policy inside an existing transaction.
// The caller must coordinate beneficiary locks/reservations; this read alone is
// not an authorization for an asynchronous treasury payment.
func CheckWalletTx(tx *sqlx.Tx, userID, packageID uint64) (int, error) {
	return checkWalletFrom(tx, userID, packageID)
}

func checkWalletFrom(db capReader, userID, packageID uint64) (int, error) {
	totalValue, err := activePackagesCapFrom(db, userID)
	if err != nil {
		return 0, err
	}
	parentWallet, err := balanceFrom(db, userID)
	if err != nil {
		return 0, err
	}

	var feePercentage sql.NullInt64
	err = db.Get(&feePercentage, `
		SELECT p.commission FROM user_packages up
		JOIN packages p ON p.id = CAST(up.package AS UNSIGNED)
		WHERE up.user_id = ? AND up.status = 'active'
		ORDER BY up.created_at DESC LIMIT 1`, userID)
	if err != nil && err != sql.ErrNoRows {
		return 0, err
	}

	var packagePrice sql.NullFloat64
	if err := db.Get(&packagePrice, "SELECT price FROM packages WHERE id = ?", packageID); err != nil && err != sql.ErrNoRows {
		return 0, err
	}

	commission := packagePrice.Float64 * (float64(feePercentage.Int64) / 100)
	if !CommissionFitsCap(parentWallet, commission, totalValue) {
		return 0, nil
	}
	return 1, nil
}

// CommissionFitsCap preserves the legacy predictive check, including equality.
// USD values remain the basis; SIGNET market conversion does not affect this test.
func CommissionFitsCap(balance, commission, cap float64) bool {
	return !(balance+commission > cap)
}

// bootstrapGlobalShareWallet creates a global_share_wallets row for userID
// if one doesn't exist and the user has at least one package priced at a
// known multiplier tier, matching the duplicated bootstrap logic in
// AuthController::processStep2 / TokenController::updateWallet /
// WalletService::creditWallet.
func bootstrapGlobalShareWallet(tx *sqlx.Tx, userID uint64) error {
	var exists int
	if err := tx.Get(&exists, "SELECT COUNT(*) FROM global_share_wallets WHERE user_id = ?", userID); err != nil {
		return err
	}
	if exists > 0 {
		return nil
	}

	var prices []int64
	if err := tx.Select(&prices, `
		SELECT p.price FROM user_packages up
		JOIN packages p ON p.id = CAST(up.package AS UNSIGNED)
		WHERE up.user_id = ?`, userID); err != nil {
		return err
	}

	var highest int64 = -1
	for _, price := range prices {
		if _, ok := packageMultipliers[price]; ok && price > highest {
			highest = price
		}
	}
	if highest < 0 {
		return nil
	}
	maxOut := float64(highest) * packageMultipliers[highest]
	_, err := tx.Exec(`INSERT INTO global_share_wallets (user_id, balance, max_out, created_at, updated_at)
		VALUES (?, 0, ?, NOW(), NOW())`, userID, maxOut)
	return err
}

// creditGlobalShareWallet applies the shared "credit up to max_out, log the
// full requested amount" pattern used by every wallet-crediting path in the
// original app. Logs the full walletAmount even when the actual credit was
// capped — that mismatch is preserved verbatim (see financial_engine.md §1).
func creditGlobalShareWallet(tx *sqlx.Tx, userID uint64, walletAmount float64) error {
	if err := bootstrapGlobalShareWallet(tx, userID); err != nil {
		return err
	}
	var gsw struct {
		ID      uint64  `db:"id"`
		Balance float64 `db:"balance"`
		MaxOut  float64 `db:"max_out"`
	}
	err := tx.Get(&gsw, "SELECT id, balance, max_out FROM global_share_wallets WHERE user_id = ?", userID)
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		return err
	}
	remaining := gsw.MaxOut - gsw.Balance
	if remaining <= 0 {
		return nil
	}
	credit := math.Min(walletAmount, remaining)
	if _, err := tx.Exec("UPDATE global_share_wallets SET balance = balance + ?, updated_at = NOW() WHERE id = ?", credit, gsw.ID); err != nil {
		return err
	}
	_, err = tx.Exec(`INSERT INTO global_share_wallets_log (user_id, amount, description, created_at, updated_at)
		VALUES (?, ?, 'Credited to Global Share Wallet', NOW(), NOW())`, itoa(userID), ftoa(walletAmount))
	return err
}

// UpdateWallet ports WalletService::updateWallet — the CAP-GATED credit
// path. Used by RocController.updateRocStatus and SalaryController.store.
// If the user's wallet balance already meets/exceeds their 4x-package cap,
// this is a silent no-op (nothing credited, no log written) — preserved
// verbatim from the original, see financial_engine.md §1.
func UpdateWallet(db *sqlx.DB, userID uint64, amount float64, description string) error {
	totalValue, err := activePackagesCapValue(db, userID)
	if err != nil {
		return err
	}
	currentBalance, err := Balance(db, userID)
	if err != nil {
		return err
	}
	var hasWallet bool
	if err := db.Get(&hasWallet, "SELECT EXISTS(SELECT 1 FROM wallets WHERE user_id = ?)", userID); err != nil {
		return err
	}
	if hasWallet && totalValue < currentBalance {
		return nil // cap reached, no credit — matches WalletService::updateWallet
	}
	return creditWallet(db, userID, amount, description)
}

// creditWallet ports WalletService::creditWallet (private in the original).
func creditWallet(db *sqlx.DB, userID uint64, amount float64, description string) error {
	tx, err := db.Beginx()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := EnsureNoPendingCommissions(tx, userID); err != nil {
		return err
	}

	var walletID sql.NullInt64
	err = tx.Get(&walletID, "SELECT id FROM wallets WHERE user_id = ?", userID)
	switch {
	case err == sql.ErrNoRows:
		if _, err := tx.Exec("INSERT INTO wallets (user_id, balance, created_at, updated_at) VALUES (?, ?, NOW(), NOW())", userID, amount); err != nil {
			return err
		}
	case err != nil:
		return err
	default:
		if _, err := tx.Exec("UPDATE wallets SET balance = balance + ?, updated_at = NOW() WHERE user_id = ?", amount, userID); err != nil {
			return err
		}
	}

	if _, err := tx.Exec(`INSERT INTO earn_logs (user_id, amount, description, created_at, updated_at)
		VALUES (?, ?, ?, NOW(), NOW())`, itoa(userID), ftoa(amount), description); err != nil {
		return err
	}

	if err := creditGlobalShareWallet(tx, userID, amount); err != nil {
		return err
	}

	// Earnings cap: first user_packages row for this user (no status
	// filter — matches the original's unfiltered ->first()), if its `earn`
	// is still under price*4, add to it; else overflow to the company
	// wallet (user_id=1). `earn` is a VARCHAR column — parse/format around
	// the arithmetic, same as PHP's type juggling would.
	var up struct {
		ID    uint64 `db:"id"`
		Earn  string `db:"earn"`
		Price int64  `db:"price"`
	}
	// A user with no user_packages row at all takes the overflow branch,
	// like the original's `if ($checkUserPackage && ...) else { company }`.
	err = tx.Get(&up, `
		SELECT up.id, COALESCE(up.earn, '') AS earn, COALESCE(p.price, 0) AS price
		FROM user_packages up
		LEFT JOIN packages p ON p.id = CAST(up.package AS UNSIGNED)
		WHERE up.user_id = ? LIMIT 1`, userID)
	hasPackage := err == nil
	if err != nil && err != sql.ErrNoRows {
		return err
	}
	if hasPackage && atof(up.Earn) <= float64(up.Price)*4 {
		earnValue := atof(up.Earn)
		if _, err := tx.Exec("UPDATE user_packages SET earn = ?, updated_at = NOW() WHERE id = ?", ftoa(earnValue+amount), up.ID); err != nil {
			return err
		}
	} else {
		if _, err := tx.Exec("UPDATE wallets SET balance = balance + ?, updated_at = NOW() WHERE user_id = 1", amount); err != nil {
			return err
		}
		if _, err := tx.Exec(`INSERT INTO earn_logs (user_id, amount, created_at, updated_at) VALUES ('1', ?, NOW(), NOW())`, ftoa(amount)); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// TokenControllerCredit ports TokenController's private updateWallet —
// the NON-CAP-GATED credit path used throughout package activation
// (processPackageActivation/processPackageActivationCompany). packagePrice
// is the activated package's price; feePercentage the commission rate
// applied. Unlike UpdateWallet above, this ALWAYS credits (no up-front cap
// check) — preserved verbatim, see financial_engine.md / api_spec.md §4.
func TokenControllerCredit(db *sqlx.DB, parentID uint64, packagePrice int64, feePercentage float64, description string) error {
	walletAmount := float64(packagePrice) * (feePercentage / 100)
	return creditWallet(db, parentID, walletAmount, description)
}

// TokenTransfer ports TokenController's private tokenTransfer: moves up to
// `amount` ACTIVE tokens from fromUserID to toUserID (amount computed by
// the caller, typically packagePrice*0.20). Returns false without
// transferring anything if fewer than `amount` tokens are available (no
// partial transfer).
func TokenTransfer(db *sqlx.DB, fromUserID, toUserID uint64, amount int64) (bool, error) {
	if amount <= 0 {
		return false, nil
	}
	tx, err := db.Beginx()
	if err != nil {
		return false, err
	}
	defer tx.Rollback()

	var ids []uint64
	if err := tx.Select(&ids, `SELECT id FROM tokens WHERE user_id = ? AND status = 'active' ORDER BY id ASC LIMIT ? FOR UPDATE`, fromUserID, amount); err != nil {
		return false, err
	}
	if int64(len(ids)) < amount {
		return false, nil
	}
	query, args, err := sqlx.In("UPDATE tokens SET user_id = ?, updated_at = NOW() WHERE id IN (?)", toUserID, ids)
	if err != nil {
		return false, err
	}
	query = tx.Rebind(query)
	if _, err := tx.Exec(query, args...); err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

// PackagePool ports the private packagePool() helper: records a
// package_pools row.
func PackagePool(db *sqlx.DB, userID, packageID uint64, amount float64) error {
	_, err := db.Exec(`INSERT INTO package_pools (user_id, package_id, pool_amount, created_at, updated_at)
		VALUES (?, ?, ?, NOW(), NOW())`, userID, packageID, amount)
	return err
}

// TokenShare ports tokenShare($package, $virtualParent) from helpers.php —
// the 20% activation bonus paid in TOKENS transferred from the house
// account (user_id=1; see financial_engine.md §3 for the "user_id=2 in the
// error message, user_id=1 in the code" discrepancy, preserved verbatim).
func TokenShare(db *sqlx.DB, packageID, virtualParent uint64) error {
	var packagePrice int64
	if err := db.Get(&packagePrice, "SELECT price FROM packages WHERE id = ?", packageID); err != nil {
		return err
	}
	bonusTokens := int64(math.Floor(float64(packagePrice) * 0.20))

	tx, err := db.Beginx()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := EnsureNoPendingCommissions(tx, virtualParent); err != nil {
		return err
	}

	var ids []uint64
	if err := tx.Select(&ids, `SELECT id FROM tokens WHERE user_id = 1 AND status = 'active' ORDER BY id ASC LIMIT ?`, bonusTokens); err != nil {
		return err
	}
	if int64(len(ids)) < bonusTokens {
		return errBonusTokens
	}
	if len(ids) > 0 {
		query, args, err := sqlx.In("UPDATE tokens SET user_id = ?, updated_at = NOW() WHERE id IN (?)", virtualParent, ids)
		if err != nil {
			return err
		}
		query = tx.Rebind(query)
		if _, err := tx.Exec(query, args...); err != nil {
			return err
		}
	}

	if _, err := tx.Exec(`INSERT INTO earn_logs (user_id, amount, description, created_at, updated_at)
		VALUES (?, ?, '20% activation bonus', NOW(), NOW())`, itoa(virtualParent), itoa64(bonusTokens)); err != nil {
		return err
	}

	// Only credits an EXISTING wallet — no Wallet.create fallback, matching
	// the original's asymmetry vs. creditWallet's create-if-missing.
	if _, err := tx.Exec("UPDATE wallets SET balance = balance + ?, updated_at = NOW() WHERE user_id = ?", bonusTokens, virtualParent); err != nil {
		return err
	}

	if err := creditGlobalShareWallet(tx, virtualParent, float64(bonusTokens)); err != nil {
		return err
	}
	return tx.Commit()
}

var errBonusTokens = notEnoughTokensErr{}

type notEnoughTokensErr struct{}

func (notEnoughTokensErr) Error() string { return "Not enough active tokens in user_id 2!" }
