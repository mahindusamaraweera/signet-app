// token_handler.go covers TokenController — the core wallet/commission
// activation flow (api_spec.md "## TokenController"). This is the highest
// financial-risk area of the port; every step below is numbered to match
// the corresponding step in api_spec.md so it can be re-audited line by
// line against the source of truth.
package handlers

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jmoiron/sqlx"

	"signet-backend/internal/app"
	"signet-backend/internal/auth"
	"signet-backend/internal/httpx"
	"signet-backend/internal/models"
	"signet-backend/internal/totp"
	"signet-backend/internal/wallet"
)

func RegisterTokenRoutes(r chi.Router, d *app.Deps) {
	r.Group(func(r chi.Router) {
		r.Use(d.Auth.RequireAuth)
		r.Use(auth.RequireRole("company"))
		r.Post("/api/v1/tokens/generate/{userId}", generateTokensHandler(d))
		r.Get("/api/v1/tokens/view/{userId}", viewTokensHandler(d))
	})

	r.Group(func(r chi.Router) {
		r.Use(d.Auth.RequireAuth) // deliberately required — see ARCHITECTURE.md
		r.Post("/api/v1/active-package", activePackageHandler(d))
		r.Get("/api/v1/token-shares", tokenShareBalanceHandler(d))
		r.Post("/api/v1/token/share", tokenShareSendHandler(d))
		r.Get("/api/v1/token/share/logs", tokenShareLogHandler(d))
	})
}

// --- generate / view tokens ---

func generateTokensHandler(d *app.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := parseUintParam(chi.URLParam(r, "userId"))
		if !ok {
			httpx.Error(w, http.StatusBadRequest, "Invalid user id")
			return
		}
		var target models.User
		if err := d.DB.Get(&target, "SELECT * FROM users WHERE id = ?", userID); err != nil {
			httpx.Error(w, http.StatusNotFound, "User not found")
			return
		}

		var body struct {
			TokenCount     int    `json:"token_count"`
			GoogleAuthCode string `json:"google_auth_code"`
		}
		if err := decodeJSON(r, &body); err != nil {
			httpx.Error(w, http.StatusBadRequest, "Invalid request body")
			return
		}
		errs := map[string][]string{}
		if body.TokenCount < 1 || body.TokenCount > 500 {
			errs["token_count"] = []string{"The token count field must be between 1 and 500."}
		}
		if body.GoogleAuthCode == "" {
			errs["google_auth_code"] = []string{"The google auth code field is required."}
		}
		if len(errs) > 0 {
			httpx.ValidationError(w, errs)
			return
		}

		// Reads the ACTING (logged-in) company user's secret, not the
		// target userID's — preserved verbatim, see api_spec.md /
		// financial_engine.md notes on this being a latent original bug.
		actingUser := auth.UserFromContext(r.Context())
		secret := actingUser.GoogleAuthenticatorSecret.String
		if secret == "" {
			httpx.OK(w, map[string]interface{}{"status": "error", "message": "Google Authenticator secret not set for the user"})
			return
		}
		if !totp.CheckCode(secret, body.GoogleAuthCode) {
			httpx.OK(w, map[string]interface{}{"status": "error", "message": "Invalid Google Authenticator code"})
			return
		}

		tx, err := d.DB.Beginx()
		if err != nil {
			httpx.Error(w, http.StatusInternalServerError, "Database error")
			return
		}
		defer tx.Rollback()
		for i := 0; i < body.TokenCount; i++ {
			tokenVal := randomTokenHex()
			if _, err := tx.Exec(`INSERT INTO tokens (user_id, token, status, created_at, updated_at) VALUES (?, ?, 'active', NOW(), NOW())`, userID, tokenVal); err != nil {
				httpx.Error(w, http.StatusInternalServerError, "Could not generate tokens")
				return
			}
		}
		if err := tx.Commit(); err != nil {
			httpx.Error(w, http.StatusInternalServerError, "Database error")
			return
		}

		httpx.OK(w, map[string]interface{}{"status": "success", "message": itoaInt(body.TokenCount) + " tokens generated successfully"})
	}
}

func randomTokenHex() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func itoaInt(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	if neg {
		return "-" + string(digits)
	}
	return string(digits)
}

func viewTokensHandler(d *app.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := parseUintParam(chi.URLParam(r, "userId"))
		if !ok {
			httpx.Error(w, http.StatusBadRequest, "Invalid user id")
			return
		}
		var target models.User
		if err := d.DB.Get(&target, "SELECT * FROM users WHERE id = ?", userID); err != nil {
			httpx.Error(w, http.StatusNotFound, "User not found")
			return
		}
		page, perPage, offset := httpx.PageParams(r, 10)
		var total int
		_ = d.DB.Get(&total, "SELECT COUNT(*) FROM tokens WHERE user_id = ?", userID)
		var tokens []models.Token
		_ = d.DB.Select(&tokens, "SELECT * FROM tokens WHERE user_id = ? ORDER BY id DESC LIMIT ? OFFSET ?", userID, perPage, offset)

		httpx.OK(w, map[string]interface{}{
			"status": "success",
			"user":   map[string]interface{}{"id": target.ID, "signet_id": models.SignetID(target.ID), "name": target.Name},
			"tokens": httpx.Paginate(tokens, total, page, perPage),
		})
	}
}

// --- active-package: the core commission/wallet activation flow ---

func activePackageHandler(d *app.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			PackageID flexString `json:"package_id"`
		}
		_ = decodeJSON(r, &body) // not validated in the original either — see api_spec.md

		actingUser := auth.UserFromContext(r.Context())

		// Every refusal is logged with its reason, so a failed activation can
		// be explained from `journalctl -u signet-api` after the fact.
		refuse := func(status int, message string, detail string) {
			log.Printf("active-package: refused user_package=%s by user=%d: %s %s", body.PackageID, actingUser.ID, message, detail)
			httpx.JSON(w, status, map[string]interface{}{"message": message})
		}
		fail := func(message string, err error) {
			log.Printf("active-package: FAILED user_package=%s by user=%d: %s: %v", body.PackageID, actingUser.ID, message, err)
			httpx.Error(w, http.StatusInternalServerError, message)
		}

		tx, err := d.DB.Beginx()
		if err != nil {
			fail("Database error", err)
			return
		}
		defer tx.Rollback()

		var pkg struct {
			ID           uint64 `db:"id"`
			UserID       uint64 `db:"user_id"`
			PackageStr   string `db:"package"`
			Status       string `db:"status"`
			Sale         string `db:"sale"`
			Earn         string `db:"earn"`
			PackagePrice int64  `db:"price"`
			Commission   int64  `db:"commission"`
		}
		err = tx.Get(&pkg, `
			SELECT up.id, up.user_id, up.package, up.status, up.sale, up.earn, p.price, p.commission
			FROM user_packages up JOIN packages p ON p.id = CAST(up.package AS UNSIGNED)
			WHERE up.id = ? FOR UPDATE`, body.PackageID)
		if err == sql.ErrNoRows {
			refuse(http.StatusNotFound, "Package not found.", "(no such user_packages row, or its package was deleted)")
			return
		}
		if err != nil {
			fail("Database error", err)
			return
		}

		if actingUser.ID == 1 {
			if err := processPackageActivationCompany(tx, &pkg); err != nil {
				fail("Could not activate package: "+err.Error(), err)
				return
			}
		} else {
			totalValue, err := activePackagesCapValueTx(tx, actingUser.ID)
			if err != nil {
				fail("Database error", err)
				return
			}
			var walletBalance models.NullFloat64
			werr := tx.Get(&walletBalance, "SELECT balance FROM wallets WHERE user_id = ?", actingUser.ID)
			if werr != nil && werr != sql.ErrNoRows {
				fail("Database error", werr)
				return
			}
			if totalValue < walletBalance.Float64 {
				refuse(http.StatusBadRequest, "Please top up your wallet.",
					fmt.Sprintf("(wallet balance %.2f is above the 4x cap %.2f of the activator's active packages)", walletBalance.Float64, totalValue))
				return
			}

			var feePercentage int64
			_ = tx.Get(&feePercentage, `
				SELECT p.commission FROM user_packages up JOIN packages p ON p.id = CAST(up.package AS UNSIGNED)
				WHERE up.user_id = ? AND up.status = 'active' ORDER BY p.price DESC LIMIT 1`, actingUser.ID)

			needTokens := pkg.PackagePrice - (pkg.PackagePrice * feePercentage / 100)

			var tokensCount int64
			_ = tx.Get(&tokensCount, "SELECT COUNT(*) FROM tokens WHERE user_id = ? AND status = 'active'", actingUser.ID)
			if tokensCount < needTokens {
				refuse(http.StatusBadRequest, "Not enough tokens.",
					fmt.Sprintf("(has %d active tokens, needs %d = price %d minus %d%% commission)", tokensCount, needTokens, pkg.PackagePrice, feePercentage))
				return
			}

			// checkWalet(auth id, $packageData->id): the PACKAGE id (packages
			// table), not the user_packages row id.
			packageID, _ := parseUintParam(pkg.PackageStr)
			checkActivation, err := wallet.CheckWallet(d.DB, actingUser.ID, packageID)
			// Note: CheckWallet reads via d.DB (its own connection), not tx —
			// matching the original which also issues checkWalet() as
			// independent queries rather than inside the same transaction.
			if err != nil {
				fail("Database error", err)
				return
			}
			if checkActivation != 1 {
				refuse(http.StatusBadRequest, "Not enough wallet.",
					fmt.Sprintf("(wallet + this commission would exceed the activator's 4x cap of %.2f)", totalValue))
				return
			}

			if err := processPackageActivation(tx, &pkg, actingUser.ID, needTokens, feePercentage); err != nil {
				if errors.Is(err, errNoUserParent) {
					refuse(http.StatusNotFound, "User-parent relationship not found.",
						fmt.Sprintf("(user %d has no user_parents row — registration step 2 never placed them in the tree)", pkg.UserID))
					return
				}
				fail("Could not activate package: "+err.Error(), err)
				return
			}
			if err := deactivateTokens(tx, actingUser.ID, needTokens); err != nil {
				fail("Could not deactivate tokens: "+err.Error(), err)
				return
			}
		}

		if err := tx.Commit(); err != nil {
			fail("Database error", err)
			return
		}
		log.Printf("active-package: activated user_package=%s (user %d) by user=%d", body.PackageID, pkg.UserID, actingUser.ID)
		httpx.OK(w, map[string]interface{}{"message": "Package and wallet updated successfully."})
	}
}

type activatingPackage = struct {
	ID           uint64 `db:"id"`
	UserID       uint64 `db:"user_id"`
	PackageStr   string `db:"package"`
	Status       string `db:"status"`
	Sale         string `db:"sale"`
	Earn         string `db:"earn"`
	PackagePrice int64  `db:"price"`
	Commission   int64  `db:"commission"`
}

func activePackagesCapValueTx(tx *sqlx.Tx, userID uint64) (float64, error) {
	var total models.NullFloat64
	err := tx.Get(&total, `
		SELECT COALESCE(SUM(p.price * 4), 0) FROM user_packages up
		JOIN packages p ON p.id = CAST(up.package AS UNSIGNED)
		WHERE up.user_id = ? AND up.status = 'active'`, userID)
	return total.Float64, err
}

var excludedPoolUserIDs = map[uint64]bool{2: true, 3: true, 4: true, 5: true}

var errNoUserParent = errors.New("user-parent relationship not found")

func processPackageActivation(tx *sqlx.Tx, pkg *activatingPackage, actingUserID uint64, needTokens, feePercentage int64) error {
	var findUser struct {
		ID        uint64 `db:"id"`
		VirtualID uint64 `db:"virtual_id"`
		Node      string `db:"node"`
	}
	// Deliberate deviation: the original returned a 404 from this private
	// method, but the caller ignored it and still burned the sponsor's
	// tokens and answered "success" with nothing activated. Failing the
	// whole transaction instead keeps the tokens.
	if err := tx.Get(&findUser, "SELECT id, virtual_id, node FROM user_parents WHERE user_id = ? LIMIT 1", pkg.UserID); err != nil {
		if err == sql.ErrNoRows {
			return errNoUserParent
		}
		return err
	}

	newNode := "active"
	if findUser.Node == "gratitude" {
		newNode = "gratitude"
	}
	if _, err := tx.Exec("UPDATE user_parents SET node = ?, updated_at = NOW() WHERE id = ?", newNode, findUser.ID); err != nil {
		return err
	}

	activatedAt := colomboWallClock()
	if _, err := tx.Exec("UPDATE user_packages SET status='active', activated_at=? , updated_at=NOW() WHERE id=?", activatedAt, pkg.ID); err != nil {
		return err
	}
	if _, err := tx.Exec("UPDATE users SET status='active', updated_at=NOW() WHERE id=?", pkg.UserID); err != nil {
		return err
	}

	if pkg.Sale == "other" {
		// UserPackage::where(user, active)->first() runs after this package
		// was set active, so it is the lowest-id active package — possibly
		// this one — exactly as in the original.
		var oldPackageID models.NullInt64
		_ = tx.Get(&oldPackageID, `SELECT id FROM user_packages WHERE user_id = ? AND status='active' ORDER BY id ASC LIMIT 1`, pkg.UserID)
		if oldPackageID.Valid {
			var discount struct {
				CurrentUserID uint64 `db:"current_user_id"`
			}
			err := tx.Get(&discount, "SELECT current_user_id FROM super_parent_logs WHERE user_package = ? LIMIT 1", oldPackageID.Int64)
			if err == nil {
				if err := creditWalletTx(tx, discount.CurrentUserID, float64(pkg.PackagePrice)*(20.0/100), ""); err != nil {
					return err
				}
				if err := tokenTransferTx(tx, 1, discount.CurrentUserID, int64(float64(pkg.PackagePrice)*0.20)); err != nil {
					return err
				}
			} else if err != sql.ErrNoRows {
				return err
			}
		}
	}

	if findUser.Node == "gratitude" {
		if err := tokenTransferTx(tx, 1, findUser.VirtualID, int64(float64(pkg.PackagePrice)*0.20)); err != nil {
			return err
		}
		if err := creditWalletTx(tx, findUser.VirtualID, float64(pkg.PackagePrice)*(20.0/100), ""); err != nil {
			return err
		}
	}

	if err := creditWalletTx(tx, actingUserID, float64(pkg.PackagePrice)*(float64(feePercentage)/100), ""); err != nil {
		return err
	}

	if !excludedPoolUserIDs[pkg.UserID] {
		packageID, _ := parseUintParam(pkg.PackageStr)
		if _, err := tx.Exec(`INSERT INTO package_pools (user_id, package_id, pool_amount, created_at, updated_at) VALUES (?, ?, ?, NOW(), NOW())`,
			pkg.UserID, packageID, float64(pkg.PackagePrice)*0.05); err != nil {
			return err
		}
	}

	var owner models.User
	if err := tx.Get(&owner, "SELECT * FROM users WHERE id = ?", pkg.UserID); err != nil {
		return err
	}
	// PHP's !empty() also treats "0" as empty — never credit user 0.
	if owner.LeaderCode.Valid && owner.LeaderCode.String != "" {
		if leaderID, ok := parseUintParam(owner.LeaderCode.String); ok && leaderID != 0 {
			if err := creditWalletTx(tx, leaderID, float64(pkg.PackagePrice)*(5.0/100), "Leadership Bonus"); err != nil {
				return err
			}
		}
	}
	if owner.ExecutiveCode.Valid && owner.ExecutiveCode.String != "" {
		if execID, ok := parseUintParam(owner.ExecutiveCode.String); ok && execID != 0 {
			if err := creditWalletTx(tx, execID, float64(pkg.PackagePrice)*(5.0/100), "Leadership Bonus"); err != nil {
				return err
			}
		}
	}
	return nil
}

func processPackageActivationCompany(tx *sqlx.Tx, pkg *activatingPackage) error {
	var findUser struct {
		ID uint64 `db:"id"`
	}
	if err := tx.Get(&findUser, "SELECT id FROM user_parents WHERE user_id = ? LIMIT 1", pkg.UserID); err != nil {
		// The original crashes here if missing (a dead null-guard never
		// runs, see api_spec.md) — surfacing as a clean error instead of a
		// panic is the one deliberate hardening in this function.
		return err
	}
	if _, err := tx.Exec("UPDATE user_parents SET node='active', updated_at=NOW() WHERE id=?", findUser.ID); err != nil {
		return err
	}
	activatedAt := colomboWallClock()
	if _, err := tx.Exec("UPDATE user_packages SET status='active', activated_at=?, updated_at=NOW() WHERE id=?", activatedAt, pkg.ID); err != nil {
		return err
	}
	if _, err := tx.Exec("UPDATE users SET status='active', updated_at=NOW() WHERE id=?", pkg.UserID); err != nil {
		return err
	}
	if !excludedPoolUserIDs[pkg.UserID] {
		packageID, _ := parseUintParam(pkg.PackageStr)
		if _, err := tx.Exec(`INSERT INTO package_pools (user_id, package_id, pool_amount, created_at, updated_at) VALUES (?, ?, ?, NOW(), NOW())`,
			pkg.UserID, packageID, float64(pkg.PackagePrice)*0.05); err != nil {
			return err
		}
	}
	return nil
}

// creditWalletTx / tokenTransferTx duplicate wallet.creditWallet /
// wallet.TokenTransfer's logic but operate WITHIN the caller's existing
// transaction (the wallet package's exported versions open their own tx,
// which would deadlock nested here) — this mirrors the original's own
// duplication (TokenController::updateWallet is a separate implementation
// from WalletService::creditWallet), so keeping a transaction-scoped copy
// here is faithful, not just a Go plumbing convenience.
func creditWalletTx(tx *sqlx.Tx, userID uint64, amount float64, description string) error {
	var walletID models.NullInt64
	err := tx.Get(&walletID, "SELECT id FROM wallets WHERE user_id = ?", userID)
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
	if _, err := tx.Exec(`INSERT INTO earn_logs (user_id, amount, description, created_at, updated_at) VALUES (?, ?, ?, NOW(), NOW())`,
		itoaU(userID), ftoaF(amount), description); err != nil {
		return err
	}

	// Global Share Wallet credit — same bootstrap+cap logic as wallet.go.
	var exists int
	if err := tx.Get(&exists, "SELECT COUNT(*) FROM global_share_wallets WHERE user_id = ?", userID); err != nil {
		return err
	}
	if exists == 0 {
		var prices []int64
		if err := tx.Select(&prices, `SELECT p.price FROM user_packages up JOIN packages p ON p.id = CAST(up.package AS UNSIGNED) WHERE up.user_id = ?`, userID); err != nil {
			return err
		}
		multipliers := map[int64]float64{5000: 1.5, 10000: 1.5, 25000: 1.5, 50000: 1.5, 100000: 1.5, 500000: 1.5, 1000000: 1.5}
		var highest int64 = -1
		for _, p := range prices {
			if _, ok := multipliers[p]; ok && p > highest {
				highest = p
			}
		}
		if highest >= 0 {
			if _, err := tx.Exec("INSERT INTO global_share_wallets (user_id, balance, max_out, created_at, updated_at) VALUES (?, 0, ?, NOW(), NOW())",
				userID, float64(highest)*multipliers[highest]); err != nil {
				return err
			}
		}
	}
	var gsw struct {
		ID      uint64  `db:"id"`
		Balance float64 `db:"balance"`
		MaxOut  float64 `db:"max_out"`
	}
	err = tx.Get(&gsw, "SELECT id, balance, max_out FROM global_share_wallets WHERE user_id = ?", userID)
	if err == nil {
		remaining := gsw.MaxOut - gsw.Balance
		if remaining > 0 {
			credit := amount
			if remaining < credit {
				credit = remaining
			}
			if _, err := tx.Exec("UPDATE global_share_wallets SET balance = balance + ?, updated_at = NOW() WHERE id = ?", credit, gsw.ID); err != nil {
				return err
			}
			if _, err := tx.Exec(`INSERT INTO global_share_wallets_log (user_id, amount, description, created_at, updated_at) VALUES (?, ?, 'Credited to Global Share Wallet', NOW(), NOW())`,
				itoaU(userID), ftoaF(amount)); err != nil {
				return err
			}
		}
	} else if err != sql.ErrNoRows {
		return err
	}

	// Earnings cap on the FIRST user_packages row for this user (no status
	// filter, matches the original).
	var up struct {
		ID    uint64 `db:"id"`
		Earn  string `db:"earn"`
		Price int64  `db:"price"`
	}
	// No user_packages row takes the overflow branch too, like the
	// original's `if ($checkUserPackage && ...) else { company }`.
	err = tx.Get(&up, `SELECT up.id, COALESCE(up.earn, '') AS earn, COALESCE(p.price, 0) AS price FROM user_packages up LEFT JOIN packages p ON p.id = CAST(up.package AS UNSIGNED) WHERE up.user_id = ? LIMIT 1`, userID)
	if err != nil && err != sql.ErrNoRows {
		return err
	}
	if err == nil && atofF(up.Earn) <= float64(up.Price)*4 {
		_, err = tx.Exec("UPDATE user_packages SET earn = ?, updated_at = NOW() WHERE id = ?", ftoaF(atofF(up.Earn)+amount), up.ID)
		return err
	}
	if _, err := tx.Exec("UPDATE wallets SET balance = balance + ?, updated_at = NOW() WHERE user_id = 1", amount); err != nil {
		return err
	}
	// TokenController::updateWallet logs the description on the company row
	// too (WalletService's version doesn't).
	_, err = tx.Exec(`INSERT INTO earn_logs (user_id, amount, description, created_at, updated_at) VALUES ('1', ?, ?, NOW(), NOW())`, ftoaF(amount), description)
	return err
}

func tokenTransferTx(tx *sqlx.Tx, fromUserID, toUserID uint64, amount int64) error {
	if amount <= 0 {
		return nil
	}
	var ids []uint64
	if err := tx.Select(&ids, `SELECT id FROM tokens WHERE user_id = ? AND status = 'active' ORDER BY id ASC LIMIT ? FOR UPDATE`, fromUserID, amount); err != nil {
		return err
	}
	if int64(len(ids)) < amount {
		return nil // no partial transfer, matches original's silent `return false`
	}
	query, args, err := sqlx.In("UPDATE tokens SET user_id = ?, updated_at = NOW() WHERE id IN (?)", toUserID, ids)
	if err != nil {
		return err
	}
	query = tx.Rebind(query)
	_, err = tx.Exec(query, args...)
	return err
}

func deactivateTokens(tx *sqlx.Tx, userID uint64, needTokens int64) error {
	if needTokens <= 0 {
		return nil
	}
	var ids []uint64
	if err := tx.Select(&ids, "SELECT id FROM tokens WHERE user_id = ? AND status = 'active' ORDER BY id ASC LIMIT ?", userID, needTokens); err != nil {
		return err
	}
	if len(ids) == 0 {
		return nil
	}
	query, args, err := sqlx.In("UPDATE tokens SET status='deactive', updated_at=NOW() WHERE id IN (?)", ids)
	if err != nil {
		return err
	}
	query = tx.Rebind(query)
	_, err = tx.Exec(query, args...)
	return err
}

// --- token sharing (peer-to-peer) ---

func tokenShareBalanceHandler(d *app.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := auth.UserFromContext(r.Context())
		var count int
		_ = d.DB.Get(&count, "SELECT COUNT(*) FROM tokens WHERE user_id = ? AND status = 'active'", user.ID)
		httpx.OK(w, map[string]interface{}{"status": "success", "tokens": count})
	}
}

func tokenShareSendHandler(d *app.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := auth.UserFromContext(r.Context())
		var body struct {
			TokenValue int    `json:"tokenValue"`
			UserID     uint64 `json:"user_id"`
		}
		if err := decodeJSON(r, &body); err != nil || body.TokenValue < 1 || body.UserID == 0 {
			httpx.ValidationError(w, map[string][]string{"tokenValue": {"The token value field is required and must be at least 1."}})
			return
		}
		var recipient models.User
		if err := d.DB.Get(&recipient, "SELECT * FROM users WHERE id = ?", body.UserID); err != nil {
			httpx.Error(w, http.StatusNotFound, "Recipient not found")
			return
		}

		tx, err := d.DB.Beginx()
		if err != nil {
			httpx.Error(w, http.StatusInternalServerError, "Database error")
			return
		}
		defer tx.Rollback()

		var ids []uint64
		if err := tx.Select(&ids, "SELECT id FROM tokens WHERE user_id = ? AND status = 'active' ORDER BY id ASC LIMIT ?", user.ID, body.TokenValue); err != nil {
			httpx.Error(w, http.StatusInternalServerError, "Database error")
			return
		}
		if len(ids) < body.TokenValue {
			httpx.OK(w, map[string]interface{}{"status": "error", "message": "You do not have enough active tokens to share."})
			return
		}
		query, args, err := sqlx.In("UPDATE tokens SET user_id = ?, updated_at = NOW() WHERE id IN (?)", recipient.ID, ids)
		if err != nil {
			httpx.Error(w, http.StatusInternalServerError, "Database error")
			return
		}
		query = tx.Rebind(query)
		if _, err := tx.Exec(query, args...); err != nil {
			httpx.Error(w, http.StatusInternalServerError, "Database error")
			return
		}
		if _, err := tx.Exec("INSERT INTO token_logs (user_id, shared_by, amount, created_at, updated_at) VALUES (?, ?, ?, NOW(), NOW())",
			recipient.ID, user.ID, body.TokenValue); err != nil {
			httpx.Error(w, http.StatusInternalServerError, "Database error")
			return
		}
		if err := tx.Commit(); err != nil {
			httpx.Error(w, http.StatusInternalServerError, "Database error")
			return
		}
		httpx.OK(w, map[string]interface{}{"status": "success", "message": "Tokens sent successfully!"})
	}
}

func tokenShareLogHandler(d *app.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := auth.UserFromContext(r.Context())
		page, perPage, offset := httpx.PageParams(r, 10)
		var total int
		_ = d.DB.Get(&total, "SELECT COUNT(*) FROM token_logs WHERE shared_by = ?", user.ID)

		rows, err := d.DB.Queryx(`
			SELECT tl.id, tl.amount, tl.created_at, u.name AS receiver_name, u.whatsapp_number AS receiver_whatsapp
			FROM token_logs tl LEFT JOIN users u ON u.id = tl.user_id
			WHERE tl.shared_by = ? ORDER BY tl.created_at DESC LIMIT ? OFFSET ?`, user.ID, perPage, offset)
		if err != nil {
			httpx.Error(w, http.StatusInternalServerError, "Database error")
			return
		}
		defer rows.Close()
		var logs []map[string]interface{}
		for rows.Next() {
			var id uint64
			var amount float64
			var createdAt models.NullTime
			var receiverName, receiverWhatsapp models.NullString
			if err := rows.Scan(&id, &amount, &createdAt, &receiverName, &receiverWhatsapp); err != nil {
				continue
			}
			name := "Unknown User"
			if receiverName.Valid {
				name = receiverName.String
			}
			wa := "N/A"
			if receiverWhatsapp.Valid {
				wa = receiverWhatsapp.String
			}
			logs = append(logs, map[string]interface{}{
				"id": id, "amount": amount, "created_at": createdAt.Time,
				"receiver": name, "receiver_whatsapp": wa,
			})
		}
		httpx.OK(w, map[string]interface{}{"status": "success", "logs": httpx.Paginate(logs, total, page, perPage)})
	}
}

func itoaU(v uint64) string {
	return strconv.FormatUint(v, 10)
}

func ftoaF(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}

func atofF(s string) float64 {
	if s == "" {
		return 0
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return f
}

// colomboWallClock reproduces how the original stored activated_at:
// Carbon::now('Asia/Colombo') written through a UTC connection, i.e. the
// Sri Lanka wall-clock time stored as if it were UTC. Weekly ROC selects
// packages by activated_at, so new rows must follow the same convention as
// the existing ones.
func colomboWallClock() time.Time {
	now := time.Now()
	if loc, err := time.LoadLocation("Asia/Colombo"); err == nil {
		now = now.In(loc)
	}
	return time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), now.Minute(), now.Second(), 0, time.UTC)
}
