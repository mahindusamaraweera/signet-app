// Package handlers implements the HTTP surface of the app, one file per
// original controller domain (see docs/analysis/api_spec.md, which this
// package follows section-by-section). auth_handler.go covers
// AuthController + Auth/PasswordResetController.
package handlers

import (
	"database/sql"
	"errors"
	"github.com/jmoiron/sqlx"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"signet-backend/internal/app"
	"signet-backend/internal/auth"
	"signet-backend/internal/blockchain"
	"signet-backend/internal/httpx"
	"signet-backend/internal/models"
	"signet-backend/internal/registrationverification"
	"signet-backend/internal/securitymail"
	"signet-backend/internal/tree"
)

var emailRe = regexp.MustCompile(`^[^\s@]+@[^\s@]+\.[^\s@]+$`)

func RegisterAuthRoutes(r chi.Router, d *app.Deps) {
	r.Post("/api/v1/login", loginHandler(d))
	r.Post("/api/v1/logout", d.Auth.RequireAuth(http.HandlerFunc(logoutHandler(d))).ServeHTTP)
	r.Get("/api/v1/me", d.Auth.OptionalAuth(http.HandlerFunc(meHandler(d))).ServeHTTP)

	r.Get("/api/v1/register/referral", registerReferralHandler(d))
	r.With(httpx.SecurityRateLimit(10, time.Minute)).Post("/api/v1/register/step1", registerStep1Handler(d))
	r.With(httpx.SecurityRateLimit(10, time.Minute)).Post("/api/v1/register/verify-email", verifyRegistrationHandler(d))
	r.Get("/api/v1/register/step2/{id}", d.Auth.OptionalAuth(registerStep2FormHandler(d)).ServeHTTP)
	r.Post("/api/v1/register/step2", d.Auth.OptionalAuth(registerStep2SubmitHandler(d)).ServeHTTP)
	r.Get("/api/v1/register/status", registerStatusHandler(d))

	r.With(httpx.SecurityRateLimit(5, time.Minute)).Post("/api/v1/password/email", passwordEmailHandler(d))
	r.Get("/api/v1/password/reset/{token}", passwordResetTokenHandler(d))
	r.With(httpx.SecurityRateLimit(10, time.Minute)).Post("/api/v1/password/reset", passwordResetHandler(d))
}

// --- login/logout/me ---

func loginHandler(d *app.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Email    string `json:"email"`
			Password string `json:"password"`
		}
		if err := decodeJSON(r, &body); err != nil {
			httpx.Error(w, http.StatusBadRequest, "Invalid request body")
			return
		}
		errs := map[string][]string{}
		if strings.TrimSpace(body.Email) == "" || !emailRe.MatchString(body.Email) {
			errs["email"] = []string{"The email field must be a valid email address."}
		}
		if body.Password == "" {
			errs["password"] = []string{"The password field is required."}
		}
		if len(errs) > 0 {
			httpx.ValidationError(w, errs)
			return
		}

		var user models.User
		err := d.DB.Get(&user, "SELECT * FROM users WHERE email = ?", body.Email)
		if err != nil || !auth.CheckPassword(user.Password, body.Password) {
			httpx.JSON(w, http.StatusOK, map[string]interface{}{"status": "error", "message": "Invalid credentials"})
			return
		}

		if err := d.Auth.IssueSession(w, &user); err != nil {
			httpx.Error(w, http.StatusInternalServerError, "Could not start session")
			return
		}

		// Pending blockchain registrations can resume funding after signing in.
		// The deadline controls unpaid cleanup, not access to deposited funds.
		if d.Cfg.BlockchainEnabled && user.Role == "user" && user.Status == "pending" {
			path, err := registrationLoginPath(d, user.ID)
			if err != nil {
				httpx.Error(w, 503, "Registration status temporarily unavailable")
				return
			}
			httpx.OK(w, map[string]interface{}{"status": "success", "redirect": path, "role": user.Role})
			return
		}

		var packageCount int
		_ = d.DB.Get(&packageCount, "SELECT COUNT(*) FROM user_packages WHERE user_id = ?", user.ID)

		if packageCount > 1 {
			httpx.OK(w, map[string]interface{}{"status": "success", "role": user.Role, "dashboard": user.Role + ".dashboard"})
			return
		}
		if packageCount == 1 {
			var pendingID models.NullInt64
			_ = d.DB.Get(&pendingID, "SELECT id FROM user_packages WHERE user_id = ? AND status = 'pending' LIMIT 1", user.ID)
			if pendingID.Valid {
				httpx.OK(w, map[string]interface{}{"status": "error", "message": "Your package is pending"})
				return
			}
			httpx.OK(w, map[string]interface{}{"status": "success", "role": user.Role, "dashboard": user.Role + ".dashboard"})
			return
		}
		// packageCount == 0 — user IS authenticated (session set above) but
		// has no package at all; matches the original exactly (not a logout).
		httpx.OK(w, map[string]interface{}{"status": "error", "message": "You do not have any package"})
	}
}

func registrationLoginPath(d *app.Deps, userID uint64) (string, error) {
	var count int
	if err := d.DB.Get(&count, "SELECT COUNT(*) FROM package_purchase_intents WHERE user_id=?", userID); err != nil {
		return "", err
	}
	if count > 0 {
		return "/register/funding/" + strconv.FormatUint(userID, 10), nil
	}
	return "/register/step2/" + strconv.FormatUint(userID, 10), nil
}

func logoutHandler(d *app.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		d.Auth.ClearSession(w)
		httpx.OK(w, map[string]interface{}{"status": "success"})
	}
}

func meHandler(d *app.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := auth.UserFromContext(r.Context())
		if user == nil {
			httpx.JSON(w, http.StatusOK, map[string]interface{}{"authenticated": false})
			return
		}
		loginPath := ""
		if d.Cfg.BlockchainEnabled && user.Role == "user" && user.Status == "pending" {
			var err error
			loginPath, err = registrationLoginPath(d, user.ID)
			if err != nil {
				httpx.Error(w, 503, "Registration status temporarily unavailable")
				return
			}
		}
		httpx.OK(w, map[string]interface{}{
			"registration_path": loginPath,
			"authenticated":     true,
			"user": map[string]interface{}{
				"id":          user.ID,
				"signet_id":   models.SignetID(user.ID),
				"name":        user.Name,
				"email":       user.Email,
				"role":        user.Role,
				"status":      user.Status,
				"on_vacation": user.OnVacation,
				"roc_status":  user.RocStatus.String,
				// Read by the dashboards' "This Time My Share Portion" card
				// (auth()->user()->global_director_share_status /
				// global_director_share in the Blade views).
				"global_director_share":        user.GlobalDirectorShare,
				"global_director_share_status": user.GlobalDirectorShareStatus,
			},
		})
	}
}

// --- registration ---

func registerReferralHandler(d *app.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		code := r.URL.Query().Get("ref")
		var referral models.ReferralCode
		if err := d.DB.Get(&referral, "SELECT * FROM referral_codes WHERE code = ?", code); err != nil {
			httpx.OK(w, map[string]interface{}{"status": "error", "message": "Invalid referral code"})
			return
		}

		var lastRefUser models.User
		latestQuery := "SELECT * FROM users WHERE referred_by = ?"
		if d.Cfg.RegistrationExpiryEnabled {
			latestQuery += " AND NOT EXISTS (SELECT 1 FROM registration_expiries e WHERE e.user_id=users.id)"
		}
		err := d.DB.Get(&lastRefUser, latestQuery+" ORDER BY id DESC LIMIT 1", referral.UserID)
		if err == nil {
			var activeCount int
			_ = d.DB.Get(&activeCount, "SELECT COUNT(*) FROM user_packages WHERE user_id = ? AND status = 'active'", lastRefUser.ID)
			if activeCount == 0 {
				httpx.OK(w, map[string]interface{}{"status": "error", "message": "The last user in this referral does not have an active package. Please contact your referral person."})
				return
			}
		}

		var countries []models.Country
		_ = d.DB.Select(&countries, "SELECT * FROM countries WHERE deleted_at IS NULL ORDER BY id")
		var leaders []models.User
		_ = d.DB.Select(&leaders, "SELECT * FROM users WHERE leader_status = 'active'")

		httpx.OK(w, map[string]interface{}{
			"status":             "success",
			"referral_code":      code,
			"countries":          countries,
			"leaders":            leaderOptions(leaders),
			"blockchain_enabled": d.Cfg.BlockchainEnabled,
		})
	}
}

func leaderOptions(leaders []models.User) []map[string]interface{} {
	out := make([]map[string]interface{}, 0, len(leaders))
	for _, l := range leaders {
		out = append(out, map[string]interface{}{"id": l.ID, "signet_id": models.SignetID(l.ID), "name": l.Name})
	}
	return out
}

func registerStep1Handler(d *app.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if d.Registration == nil {
			httpx.Error(w, 503, "Registration email verification is temporarily unavailable")
			return
		}
		var body struct {
			Name            string `json:"name"`
			Email           string `json:"email"`
			Password        string `json:"password"`
			PasswordConfirm string `json:"password_confirmation"`
			WhatsappNumber  string `json:"whatsapp_number"`
			CountryCode     string `json:"country_code"`
			BinancePayID    string `json:"binance_pay_id"`
			ReferralCode    string `json:"referral_code"`
			Country         string `json:"country"`
			LeaderCode      string `json:"leader_code"`
			ExecutiveCode   string `json:"executive_code"`
		}
		if err := decodeJSON(r, &body); err != nil {
			httpx.Error(w, http.StatusBadRequest, "Invalid request body")
			return
		}

		errs := map[string][]string{}
		if !emailRe.MatchString(body.Email) {
			errs["email"] = []string{"The email field must be a valid email address."}
		}
		if len(body.Password) < 6 {
			errs["password"] = []string{"The password field must be at least 6 characters."}
		} else if body.Password != body.PasswordConfirm {
			errs["password"] = []string{"The password field confirmation does not match."}
		}
		if body.WhatsappNumber == "" {
			errs["whatsapp_number"] = []string{"The whatsapp number field is required."}
		}
		if body.CountryCode == "" {
			errs["country_code"] = []string{"The country code field is required."}
		}
		if body.BinancePayID == "" && !d.Cfg.BlockchainEnabled {
			errs["binance_pay_id"] = []string{"The binance pay id field is required."}
		}
		if body.Country == "" {
			errs["country"] = []string{"The country field is required."}
		}
		var referral models.ReferralCode
		if err := d.DB.Get(&referral, "SELECT * FROM referral_codes WHERE code = ?", body.ReferralCode); err != nil {
			errs["referral_code"] = []string{"The selected referral code is invalid."}
		}
		// 'leader_code'/'executive_code' => 'nullable|exists:users,id': both
		// receive the 5% leadership bonus on activation, so they must be
		// real user ids.
		for field, code := range map[string]string{"leader_code": body.LeaderCode, "executive_code": body.ExecutiveCode} {
			if code == "" {
				continue
			}
			var n int
			if err := d.DB.Get(&n, "SELECT COUNT(*) FROM users WHERE id = ?", code); err != nil || n == 0 {
				errs[field] = []string{"The selected " + strings.ReplaceAll(field, "_", " ") + " is invalid."}
			}
		}
		if body.LeaderCode != "" && body.LeaderCode == body.ExecutiveCode {
			errs["executive_code"] = []string{"The executive code and leader code must be different."}
		}
		if len(errs) > 0 {
			httpx.ValidationError(w, errs)
			return
		}

		whatsapp := body.CountryCode + body.WhatsappNumber

		var dupe int
		_ = d.DB.Get(&dupe, "SELECT COUNT(*) FROM users WHERE whatsapp_number = ?", whatsapp)
		if dupe > 0 {
			httpx.OK(w, map[string]interface{}{"status": "error", "message": "The WhatsApp number is already registered."})
			return
		}
		dupe = 0
		if body.BinancePayID != "" {
			_ = d.DB.Get(&dupe, "SELECT COUNT(*) FROM users WHERE binance_pay_id = ?", body.BinancePayID)
		}
		if dupe > 0 {
			httpx.OK(w, map[string]interface{}{"status": "error", "message": "The Binance Pay ID is already registered."})
			return
		}
		_ = d.DB.Get(&dupe, "SELECT COUNT(*) FROM users WHERE email = ?", body.Email)
		if dupe > 0 {
			httpx.OK(w, map[string]interface{}{"status": "error", "message": "Email already exists"})
			return
		}

		countryID, err := strconv.ParseUint(body.Country, 10, 64)
		if err != nil || countryID == 0 {
			httpx.Error(w, 422, "Invalid country")
			return
		}
		id, err := d.Registration.Issue(r.Context(), body.Email, body.Password, registrationverification.Data{
			Name: body.Name, Whatsapp: whatsapp, CountryID: countryID, ReferralCode: body.ReferralCode,
			LeaderCode: body.LeaderCode, ExecutiveCode: body.ExecutiveCode, BinancePayID: body.BinancePayID,
		}, time.Now().UTC())
		if errors.Is(err, registrationverification.ErrResendLimit) {
			httpx.Error(w, 429, "Please wait before requesting another verification email")
			return
		}
		if err != nil {
			// The reason stays server-side (journalctl -u signet-api); the
			// client only learns that delivery failed.
			log.Printf("register/step1: verification email to %s failed: %v", body.Email, err)
			httpx.Error(w, 503, "Unable to send registration verification email")
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		httpx.JSON(w, 202, map[string]interface{}{"status": "verification_required", "registration_id": id,
			"message": "Verification email sent. Your account will be created after verification."})
	}
}

func verifyRegistrationHandler(d *app.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if d.Registration == nil {
			httpx.Error(w, 503, "Registration email verification is temporarily unavailable")
			return
		}
		var body struct {
			ID    string `json:"id"`
			Token string `json:"token"`
		}
		if err := decodeJSON(r, &body); err != nil {
			httpx.Error(w, 422, "Invalid verification request")
			return
		}
		id, err := d.Registration.Complete(r.Context(), body.ID, body.Token, time.Now().UTC())
		if errors.Is(err, registrationverification.ErrVerification) || errors.Is(err, registrationverification.ErrEmailExists) {
			httpx.Error(w, 422, "Verification link is invalid, expired or already used. Please log in or register again.")
			return
		}
		if err != nil {
			httpx.Error(w, 503, "Registration verification temporarily unavailable")
			return
		}
		// The session must be issued from the stored row: its credential
		// version is derived from the password hash and re-checked on every
		// request, so a bare {ID, Role} user produced a session that was
		// silently rejected ("Registration ownership required" at step 2).
		var created models.User
		if err := d.DB.Get(&created, "SELECT * FROM users WHERE id = ?", id); err != nil {
			httpx.Error(w, 500, "Email verified and account created. Please log in to continue.")
			return
		}
		if err := d.Auth.IssueSession(w, &created); err != nil {
			httpx.Error(w, 500, "Email verified and account created. Please log in to continue.")
			return
		}
		httpx.OK(w, map[string]interface{}{"status": "success", "user_id": id, "blockchain_enabled": d.Cfg.BlockchainEnabled})
	}
}

func registerStep2FormHandler(d *app.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if d.Cfg.BlockchainEnabled {
			user := auth.UserFromContext(r.Context())
			if user == nil || strconv.FormatUint(user.ID, 10) != id {
				httpx.Error(w, 403, "Registration ownership required")
				return
			}
		}
		var packages []models.Package
		if err := d.DB.Select(&packages, "SELECT * FROM packages WHERE status = 'active'"); err != nil {
			httpx.Error(w, 503, "Package catalog temporarily unavailable")
			return
		}
		httpx.OK(w, map[string]interface{}{"status": "success", "id": id, "packages": purchasePackageChoices(packages, d.Cfg.BlockchainEnabled), "blockchain_enabled": d.Cfg.BlockchainEnabled})
	}
}

func registerStep2SubmitHandler(d *app.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Package   string `json:"package"`
			NewUserID string `json:"newUserID"`
		}
		if err := decodeJSON(r, &body); err != nil || body.Package == "" || body.NewUserID == "" {
			httpx.ValidationError(w, map[string][]string{"package": {"The package field is required."}})
			return
		}
		newUserID, ok := parseUintParam(body.NewUserID)
		if !ok {
			httpx.Error(w, http.StatusBadRequest, "Invalid user id")
			return
		}
		packageID, err := strconv.ParseUint(body.Package, 10, 64)
		if d.Cfg.BlockchainEnabled {
			user := auth.UserFromContext(r.Context())
			if user == nil || user.ID != newUserID {
				httpx.Error(w, 403, "Registration ownership required")
				return
			}
			var existing int
			if err := d.DB.Get(&existing, "SELECT COUNT(*) FROM package_purchase_intents WHERE user_id=?", newUserID); err != nil {
				httpx.Error(w, 500, "Database error")
				return
			}
			if existing > 0 {
				httpx.OK(w, map[string]interface{}{"status": "success", "user_id": newUserID, "blockchain_enabled": true})
				return
			}
		}
		if err != nil {
			httpx.Error(w, http.StatusBadRequest, "Invalid package id")
			return
		}

		var userData models.User
		if err := d.DB.Get(&userData, "SELECT * FROM users WHERE id = ?", newUserID); err != nil {
			httpx.Error(w, http.StatusNotFound, "User not found")
			return
		}

		tx, err := d.DB.Beginx()
		if err != nil {
			httpx.Error(w, 500, "Database error")
			return
		}
		defer tx.Rollback()
		if d.Cfg.BlockchainEnabled {
			var lockedID uint64
			if e := tx.Get(&lockedID, "SELECT id FROM users WHERE id=? FOR UPDATE", newUserID); e != nil {
				httpx.Error(w, 500, "Database error")
				return
			}
			var existing int
			if e := tx.Get(&existing, "SELECT COUNT(*) FROM package_purchase_intents WHERE user_id=?", newUserID); e != nil {
				httpx.Error(w, 500, "Database error")
				return
			}
			if existing > 0 {
				httpx.OK(w, map[string]interface{}{"status": "success", "user_id": newUserID, "blockchain_enabled": true})
				return
			}
		}
		var createdIDs []uint64
		referredBy := uint64(0)
		if userData.ReferredBy.Valid {
			referredBy = uint64(userData.ReferredBy.Int64)
		}
		parentID, err := tree.ParentFind(tx, referredBy, packageID, newUserID, 1, &createdIDs)
		if err != nil {
			httpx.Error(w, http.StatusInternalServerError, "Could not resolve upline placement")
			return
		}

		var userparent uint64
		if parentID == 1 {
			userparent = referredBy
		} else {
			userparent = parentID
		}

		res, err := tx.Exec(`INSERT INTO user_parents (user_id, virtual_id, parent_id, node, created_at, updated_at)
			VALUES (?, ?, ?, 'deactive', NOW(), NOW())`, newUserID, referredBy, userparent)
		if err != nil {
			httpx.Error(w, http.StatusInternalServerError, "Could not place user in tree")
			return
		}
		rootRowID, _ := res.LastInsertId()
		createdIDs = append(createdIDs, uint64(rootRowID))

		if err := correctLegacySignupPlacement(tx, d.Cfg.BlockchainEnabled, referredBy, rootRowID, &createdIDs); err != nil {
			httpx.Error(w, http.StatusInternalServerError, "Could not place user in tree")
			return
		}

		var findParent struct {
			ParentID uint64 `db:"parent_id"`
		}
		_ = tx.Get(&findParent, "SELECT parent_id FROM user_parents WHERE user_id = ? LIMIT 1", newUserID)

		var parentData models.User
		_ = tx.Get(&parentData, "SELECT * FROM users WHERE id = ?", parentID)

		var activeExists int
		_ = tx.Get(&activeExists, "SELECT COUNT(*) FROM user_packages WHERE user_id = ? AND status = 'active'", newUserID)
		if activeExists > 0 {
			httpx.OK(w, map[string]interface{}{"status": "error", "message": "You already have an active package", "redirect": "login"})
			return
		}

		var buyPackage models.Package
		if err := tx.Get(&buyPackage, "SELECT * FROM packages WHERE id = ? AND status = 'active'", packageID); err != nil {
			httpx.Error(w, http.StatusNotFound, "Package not found")
			return
		}

		var existingUserPackageID models.NullInt64
		_ = tx.Get(&existingUserPackageID, "SELECT id FROM user_packages WHERE user_id = ? LIMIT 1", newUserID)
		var newUserPackageID int64
		if existingUserPackageID.Valid {
			newUserPackageID = existingUserPackageID.Int64
			_, err = tx.Exec(`UPDATE user_packages SET package=?, status='pending', ref_id=?, sale='first', updated_at=NOW() WHERE id=?`,
				strconv.FormatUint(buyPackage.ID, 10), strconv.FormatUint(findParent.ParentID, 10), newUserPackageID)
		} else {
			var r2 sql.Result
			r2, err = tx.Exec(`INSERT INTO user_packages (user_id, package, status, ref_id, sale, created_at, updated_at)
				VALUES (?, ?, 'pending', ?, 'first', NOW(), NOW())`, newUserID, strconv.FormatUint(buyPackage.ID, 10), strconv.FormatUint(findParent.ParentID, 10))
			if err == nil {
				newUserPackageID, _ = r2.LastInsertId()
			}
		}
		if err != nil {
			httpx.Error(w, http.StatusInternalServerError, "Could not create package purchase")
			return
		}

		// SuperParentLog::where(...)->first()->update(): only one row.
		_, _ = tx.Exec(`UPDATE super_parent_logs SET user_package = ? WHERE gratitude_user = ? AND created_at >= ? ORDER BY id LIMIT 1`,
			newUserPackageID, newUserID, time.Now().Add(-1*time.Minute))

		createdIDsJSON := "["
		for i, id := range createdIDs {
			if i > 0 {
				createdIDsJSON += ","
			}
			createdIDsJSON += strconv.FormatUint(id, 10)
		}
		createdIDsJSON += "]"
		if d.Cfg.BlockchainEnabled {
			if buyPackage.Status != "active" || buyPackage.Price <= 0 {
				httpx.Error(w, 400, "Package is unavailable")
				return
			}
			var percentage int64
			if e := tx.Get(&percentage, `SELECT p.commission FROM user_packages up JOIN packages p ON p.id=CAST(up.package AS UNSIGNED) WHERE up.user_id=? AND up.status='active' ORDER BY p.price DESC LIMIT 1`, findParent.ParentID); e != nil && e != sql.ErrNoRows {
				httpx.Error(w, 500, "Could not determine commission")
				return
			}
			terms, e := blockchain.CommercialPackageTerms(buyPackage.Price)
			if e != nil {
				httpx.Error(w, 422, "Package commercial terms are not approved")
				return
			}
			if _, e := tx.Exec(`INSERT INTO package_purchase_intents (user_id,user_package_id,package_id,package_price,remaining_usdt,commission_user_id,commission_percent,service_fee_usdt,required_usdt,bonus_percent) VALUES (?,?,?,?,?,?,?,?,?,?)`, newUserID, newUserPackageID, buyPackage.ID, buyPackage.Price, terms.Required, findParent.ParentID, percentage, terms.Fee, terms.Required, terms.BonusPercent); e != nil {
				httpx.Error(w, 500, "Could not create purchase intent")
				return
			}
		}
		_, _ = tx.Exec(`INSERT INTO user_parent_map_logs (user_id, parent_id, created_row_ids, note, created_at, updated_at)
			VALUES (?, ?, ?, 'UserParent rows created in processStep2', NOW(), NOW())`, newUserID, parentID, createdIDsJSON)

		if err := tx.Commit(); err != nil {
			httpx.Error(w, http.StatusInternalServerError, "Database error")
			return
		}

		// Global Share Wallet bootstrap for the new user (best-effort,
		// mirrors the original's inline duplicate of the same logic).
		_ = bootstrapNewUserGlobalShareWallet(d, newUserID)

		// NewUserPackageMail — best effort, does not block the response.
		go sendNewUserPackageMail(d, &userData, &parentData, &buyPackage)

		httpx.OK(w, map[string]interface{}{
			"status": "success",
			"parent": map[string]interface{}{
				"binance_pay_id":  parentData.BinancePayID.String,
				"whatsapp_number": parentData.WhatsappNumber.String,
				"on_vacation":     parentData.OnVacation,
			},
			"user_id":            newUserID,
			"blockchain_enabled": d.Cfg.BlockchainEnabled,
		})
	}
}

func bootstrapNewUserGlobalShareWallet(d *app.Deps, userID uint64) error {
	var exists int
	if err := d.DB.Get(&exists, "SELECT COUNT(*) FROM global_share_wallets WHERE user_id = ?", userID); err != nil || exists > 0 {
		return err
	}
	var highest models.NullInt64
	_ = d.DB.Get(&highest, `
		SELECT MAX(p.price) FROM user_packages up JOIN packages p ON p.id = CAST(up.package AS UNSIGNED)
		WHERE up.user_id = ? AND p.price IN (5000,10000,25000,50000,100000,500000,1000000)`, userID)
	if !highest.Valid {
		return nil
	}
	maxOut := float64(highest.Int64) * 1.5
	_, err := d.DB.Exec("INSERT INTO global_share_wallets (user_id, balance, max_out, created_at, updated_at) VALUES (?, 0, ?, NOW(), NOW())", userID, maxOut)
	return err
}

func registerStatusHandler(d *app.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.URL.Query().Get("id")
		var user models.User
		if err := d.DB.Get(&user, "SELECT * FROM users WHERE id = ?", id); err != nil {
			httpx.Error(w, http.StatusNotFound, "User not found")
			return
		}
		httpx.OK(w, map[string]interface{}{"status": "success", "user": map[string]interface{}{"id": user.ID, "status": user.Status}})
	}
}

// --- password reset ---

func passwordEmailHandler(d *app.Deps) http.HandlerFunc {
	return passwordEmailRequestHandler(func(r *http.Request, email string) error {
		return issuePasswordReset(r.Context(), d.DB.DB, email, d.Cfg.AppURL, time.Now().UTC(), securitymail.New(d.Cfg))
	})
}

func passwordResetTokenHandler(d *app.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := chi.URLParam(r, "token")
		lookup, err := passwordResetLookup(token)
		if err != nil {
			httpx.Error(w, 422, "Invalid reset token")
			return
		}
		var email string
		w.Header().Set("Cache-Control", "no-store")
		if err := d.DB.Get(&email, "SELECT email FROM password_reset WHERE token = ? AND delivery_status IN ('legacy','delivered') AND created_at > DATE_SUB(NOW(), INTERVAL 1 HOUR) AND created_at <= NOW() LIMIT 1", lookup); err != nil {
			httpx.OK(w, map[string]interface{}{"status": "error", "message": "This password reset token is invalid."})
			return
		}
		httpx.OK(w, map[string]interface{}{"status": "success", "token": token, "email": email})
	}
}

func passwordResetHandler(d *app.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Email           string `json:"email"`
			Password        string `json:"password"`
			PasswordConfirm string `json:"password_confirmation"`
			Token           string `json:"token"`
		}
		if err := decodeJSON(r, &body); err != nil {
			httpx.Error(w, http.StatusBadRequest, "Invalid request body")
			return
		}
		errs := map[string][]string{}
		if !emailRe.MatchString(body.Email) {
			errs["email"] = []string{"The email field must be a valid email address."}
		}
		if len(body.Password) < 8 {
			errs["password"] = []string{"The password field must be at least 8 characters."}
		} else if body.Password != body.PasswordConfirm {
			errs["password"] = []string{"The password field confirmation does not match."}
		}
		if body.Token == "" {
			errs["token"] = []string{"The token field is required."}
		}
		if len(errs) > 0 {
			httpx.ValidationError(w, errs)
			return
		}

		w.Header().Set("Cache-Control", "no-store")
		err := completePasswordReset(r.Context(), d.DB.DB, body.Email, body.Token, body.Password, time.Now().UTC())
		if errors.Is(err, errResetProof) {
			httpx.ValidationError(w, map[string][]string{"token": {"This password reset token is invalid."}})
			return
		}
		if err != nil {
			httpx.Error(w, http.StatusInternalServerError, "Could not update password")
			return
		}

		httpx.OK(w, map[string]interface{}{"status": "success", "message": "Your password has been reset!"})
	}
}

// Blockchain purchases bind placement and commission evidence to each user.
// Never reuse another signup's row just because timestamps share one second.
// Keep the legacy correction only for the legacy funding flow.
func correctLegacySignupPlacement(tx *sqlx.Tx, blockchainEnabled bool, referredBy uint64, rootRowID int64, createdIDs *[]uint64) error {
	if blockchainEnabled {
		return nil
	}
	if referredBy != 0 {
		var siblings []struct {
			ID        uint64 `db:"id"`
			UserID    uint64 `db:"user_id"`
			VirtualID uint64 `db:"virtual_id"`
			ParentID  uint64 `db:"parent_id"`
		}
		if err := tx.Select(&siblings, `SELECT id, user_id, virtual_id, parent_id FROM user_parents
				WHERE virtual_id = ? AND created_at = (SELECT created_at FROM user_parents WHERE id = ?)
				ORDER BY created_at ASC, id ASC FOR UPDATE`, referredBy, rootRowID); err != nil {
			return err
		}
		if len(siblings) >= 2 {
			gratitudeRow := siblings[0]
			activeRow := siblings[len(siblings)-1]
			if _, err := tx.Exec(`UPDATE user_parents SET user_id=?, virtual_id=?, parent_id=?, updated_at=NOW() WHERE id=?`,
				activeRow.UserID, activeRow.VirtualID, activeRow.ParentID, gratitudeRow.ID); err != nil {
				return err
			}
			res, err := tx.Exec(`INSERT INTO user_parents (user_id, virtual_id, parent_id, node, created_at, updated_at)
					VALUES (?, ?, ?, 'correct', NOW(), NOW())`, activeRow.UserID, activeRow.ParentID, activeRow.ParentID)
			if err != nil {
				return err
			}
			correctID, _ := res.LastInsertId()
			*createdIDs = append(*createdIDs, uint64(correctID))
			if _, err := tx.Exec("DELETE FROM user_parents WHERE id = ?", activeRow.ID); err != nil {
				return err
			}
		}
	}

	return nil
}
