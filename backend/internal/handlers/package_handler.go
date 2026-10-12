// package_handler.go covers PackageController (api_spec.md "## PackageController"):
// company-only package CRUD, plus the public-in-the-original
// buy-package/buy-packages flow, which we gate behind RequireAuth per the
// one deliberate fix documented in ARCHITECTURE.md (Cross-cutting note #2).
package handlers

import (
	"math"
	"net/http"
	"signet-backend/internal/blockchain"
	"strings"

	"github.com/go-chi/chi/v5"

	"signet-backend/internal/app"
	"signet-backend/internal/auth"
	"signet-backend/internal/httpx"
	"signet-backend/internal/models"
	"signet-backend/internal/wallet"
)

func RegisterPackageRoutes(r chi.Router, d *app.Deps) {
	r.Group(func(r chi.Router) {
		r.Use(d.Auth.RequireAuth)
		r.Use(auth.RequireRole("company"))
		r.Get("/api/v1/packages", packageIndexHandler(d))
		r.Post("/api/v1/packages", packageStoreHandler(d))
		r.Get("/api/v1/packages/{id}/edit", packageEditHandler(d))
		r.Put("/api/v1/packages/{id}", packageUpdateHandler(d))
		r.Delete("/api/v1/packages/{id}", packageDestroyHandler(d))
	})

	// Originally unguarded in the source app (no `auth` middleware on these
	// three routes — see ARCHITECTURE.md / api_spec.md Cross-cutting note
	// #2) — deliberately required here.
	r.Group(func(r chi.Router) {
		r.Use(d.Auth.RequireAuth)
		r.Get("/api/v1/buy-package-history", buyPackageHistoryHandler(d))
		r.Get("/api/v1/buy-package", buyPackageHandler(d))
		r.Post("/api/v1/buy-packages", buyPackagesHandler(d))
	})
}

// --- company CRUD ---

type packagePayload struct {
	Name         *string  `json:"name"`
	Price        *float64 `json:"price"`
	Commission   *float64 `json:"commission"`
	Rank         *string  `json:"rank"`
	TelegramLink *string  `json:"telegram_link"`
	Status       *string  `json:"status"`
}

func validatePackagePayload(body *packagePayload) map[string][]string {
	errs := map[string][]string{}
	if body.Name == nil || strings.TrimSpace(*body.Name) == "" {
		errs["name"] = []string{"The name field is required."}
	}
	if body.Price == nil || *body.Price <= 0 || math.IsNaN(*body.Price) || math.IsInf(*body.Price, 0) || math.Trunc(*body.Price) != *body.Price || *body.Price > 2147483647 {
		errs["price"] = []string{"The price must be a positive whole number within the supported range."}
	}
	if body.Commission == nil || *body.Commission < 0 || *body.Commission > 100 || math.IsNaN(*body.Commission) || math.Trunc(*body.Commission) != *body.Commission {
		errs["commission"] = []string{"The commission must be a whole percentage between 0 and 100."}
	}
	if body.Status == nil || strings.TrimSpace(*body.Status) == "" {
		errs["status"] = []string{"The status field is required."}
	} else if *body.Status != "active" && *body.Status != "deactive" {
		errs["status"] = []string{"The selected status is invalid."}
	}
	return errs
}

func packageIndexHandler(d *app.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var packages []models.Package
		if err := d.DB.Select(&packages, "SELECT * FROM packages ORDER BY created_at DESC, id DESC"); err != nil {
			httpx.Error(w, http.StatusInternalServerError, "Database error")
			return
		}
		httpx.OK(w, map[string]interface{}{"status": "success", "packages": packages})
	}
}

func packageStoreHandler(d *app.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body packagePayload
		if err := decodeJSON(r, &body); err != nil {
			httpx.Error(w, http.StatusBadRequest, "Invalid request body")
			return
		}
		errs := validatePackagePayload(&body)
		validateCommercialPackagePayload(d, &body, errs)
		if len(errs) > 0 {
			httpx.ValidationError(w, errs)
			return
		}
		res, err := d.DB.Exec(`INSERT INTO packages (name, price, commission, rank, telegram_link, status, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, NOW(), NOW())`,
			*body.Name, int64(*body.Price), int64(*body.Commission), optionalPackageText(body.Rank), optionalPackageText(body.TelegramLink), *body.Status)
		if err != nil {
			httpx.Error(w, http.StatusInternalServerError, "Database error")
			return
		}
		id, _ := res.LastInsertId()
		httpx.OK(w, map[string]interface{}{"status": "success", "message": "Package created successfully.", "id": id})
	}
}

func packageEditHandler(d *app.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := parseUintParam(chi.URLParam(r, "id"))
		if !ok {
			httpx.Error(w, http.StatusBadRequest, "Invalid package id")
			return
		}
		var pkg models.Package
		if err := d.DB.Get(&pkg, "SELECT * FROM packages WHERE id = ?", id); err != nil {
			httpx.Error(w, http.StatusNotFound, "Package not found")
			return
		}
		httpx.OK(w, map[string]interface{}{"status": "success", "package": pkg})
	}
}

func packageUpdateHandler(d *app.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := parseUintParam(chi.URLParam(r, "id"))
		if !ok {
			httpx.Error(w, http.StatusBadRequest, "Invalid package id")
			return
		}
		var existing models.Package
		if err := d.DB.Get(&existing, "SELECT * FROM packages WHERE id = ?", id); err != nil {
			httpx.Error(w, http.StatusNotFound, "Package not found")
			return
		}
		var body packagePayload
		if err := decodeJSON(r, &body); err != nil {
			httpx.Error(w, http.StatusBadRequest, "Invalid request body")
			return
		}
		errs := validatePackagePayload(&body)
		validateCommercialPackagePayload(d, &body, errs)
		if len(errs) > 0 {
			httpx.ValidationError(w, errs)
			return
		}
		if _, err := d.DB.Exec(`UPDATE packages SET name=?, price=?, commission=?, rank=?, telegram_link=?, status=?, updated_at=NOW() WHERE id=?`,
			*body.Name, int64(*body.Price), int64(*body.Commission), optionalPackageText(body.Rank), optionalPackageText(body.TelegramLink), *body.Status, id); err != nil {
			httpx.Error(w, http.StatusInternalServerError, "Database error")
			return
		}
		httpx.OK(w, map[string]interface{}{"status": "success", "message": "Package updated successfully."})
	}
}

func packageDestroyHandler(d *app.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := parseUintParam(chi.URLParam(r, "id"))
		if !ok {
			httpx.Error(w, http.StatusBadRequest, "Invalid package id")
			return
		}
		var existing models.Package
		if err := d.DB.Get(&existing, "SELECT * FROM packages WHERE id = ?", id); err != nil {
			httpx.Error(w, http.StatusNotFound, "Package not found")
			return
		}
		if _, err := d.DB.Exec("DELETE FROM packages WHERE id = ?", id); err != nil {
			httpx.Error(w, http.StatusInternalServerError, "Database error")
			return
		}
		httpx.OK(w, map[string]interface{}{"status": "success", "message": "Package deleted successfully."})
	}
}

// --- buy-package flow (end user) ---

func buyPackageHistoryHandler(d *app.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := auth.UserFromContext(r.Context())
		page, perPage, offset := httpx.PageParams(r, 10)

		var total int
		_ = d.DB.Get(&total, "SELECT COUNT(*) FROM user_packages WHERE user_id = ?", user.ID)

		// PackageController@buyPackageHistory: UserPackage::where(user_id)
		// ->paginate(10) — no explicit order — and the view shows
		// $package->userpackage->name (the joined package's name).
		type historyRow struct {
			models.UserPackage
			PackageName models.NullString `db:"package_name" json:"package_name"`
		}
		var packages []historyRow
		_ = d.DB.Select(&packages, `
			SELECT up.*, p.name AS package_name
			FROM user_packages up
			LEFT JOIN packages p ON p.id = CAST(up.package AS UNSIGNED)
			WHERE up.user_id = ?
			ORDER BY up.id
			LIMIT ? OFFSET ?`, user.ID, perPage, offset)

		var activePackage models.UserPackage
		var activePackagePtr interface{}
		if err := d.DB.Get(&activePackage, "SELECT * FROM user_packages WHERE user_id = ? AND status = 'active' ORDER BY id DESC LIMIT 1", user.ID); err == nil {
			activePackagePtr = activePackage
		}

		httpx.OK(w, map[string]interface{}{
			"status":         "success",
			"packages":       httpx.Paginate(packages, total, page, perPage),
			"active_package": activePackagePtr,
		})
	}
}

func buyPackageHandler(d *app.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var packages []models.Package
		if err := d.DB.Select(&packages, "SELECT * FROM packages WHERE status = 'active'"); err != nil {
			httpx.Error(w, 503, "Package catalog temporarily unavailable")
			return
		}
		httpx.OK(w, map[string]interface{}{"status": "success", "packages": purchasePackageChoices(packages, d.Cfg.BlockchainEnabled), "blockchain_enabled": d.Cfg.BlockchainEnabled, "topup_enabled": d.Cfg.BlockchainTopupEnabled})
	}
}

// buyPackagesHandler ports PackageController::buyPackages (api_spec.md
// "### POST /buy-packages"). id-assignment logic, step by step, matching
// the numbered steps in api_spec.md:
//  1. loggedUserID = the acting session user.
//  2. Walk up the ancestor chain (max 50 hops) via user_parents.parent_id
//     starting from the logged-in user, until we find a User with
//     status == 'active'. api_spec.md describes this in prose only (no
//     literal PHP source was provided for this specific loop); we resolve
//     the ambiguity by reusing the same `user_parents` access pattern the
//     rest of the port already uses elsewhere (auth_handler.go's
//     registerStep2SubmitHandler: `SELECT parent_id FROM user_parents
//     WHERE user_id = ? LIMIT 1`) to walk user_id -> parent_id repeatedly.
//  3. checkActivation = wallet.CheckWallet(activeParentID, package) if an
//     active ancestor was found, else 0.
//  4. saveUser = loggedUserID always (the purchasing user).
//  5. user = activeParentID if checkActivation==1 AND an ancestor was
//     found; else user = 1 (company fallback sentinel).
//     6/7. Load parentData / the package being bought; 404-equivalent errors
//     if either is missing.
//  8. ref_id = user if checkActivation==1, else 0.
//  9. Insert the new user_packages row: status=pending, sale=other.
func buyPackagesHandler(d *app.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if d.Cfg.BlockchainEnabled {
			createBlockchainTopup(d, w, r)
			return
		}
		var body struct {
			Package string `json:"package"`
		}
		_ = decodeJSON(r, &body) // not validated in the original either — see api_spec.md

		user := auth.UserFromContext(r.Context())
		loggedUserID := user.ID

		var activeParentID uint64
		foundActiveAncestor := false
		current := loggedUserID
		for hop := 0; hop < 50; hop++ {
			var parentID uint64
			err := d.DB.Get(&parentID, "SELECT parent_id FROM user_parents WHERE user_id = ? LIMIT 1", current)
			if err != nil || parentID == 0 {
				break
			}
			var parentUser models.User
			if uerr := d.DB.Get(&parentUser, "SELECT * FROM users WHERE id = ?", parentID); uerr != nil {
				break // `if (!$parentUser) break;`
			}
			if parentUser.Status == "active" {
				activeParentID = parentID
				foundActiveAncestor = true
				break
			}
			current = parentID
		}

		packageID, _ := parseUintParam(body.Package)

		checkActivation := 0
		if foundActiveAncestor {
			ca, err := wallet.CheckWallet(d.DB, activeParentID, packageID)
			if err == nil {
				checkActivation = ca
			}
		}

		saveUser := loggedUserID
		var refUser uint64 = 1 // company fallback sentinel
		if checkActivation == 1 && foundActiveAncestor {
			refUser = activeParentID
		}

		var parentData models.User
		if err := d.DB.Get(&parentData, "SELECT * FROM users WHERE id = ?", refUser); err != nil {
			httpx.OK(w, map[string]interface{}{"status": "error", "message": "User not found"})
			return
		}

		var buyPackage models.Package
		if err := d.DB.Get(&buyPackage, "SELECT * FROM packages WHERE id = ? AND status = 'active'", packageID); err != nil {
			httpx.OK(w, map[string]interface{}{"status": "error", "message": "Package not found"})
			return
		}

		refID := uint64(0)
		if checkActivation == 1 {
			refID = refUser
		}

		if _, err := d.DB.Exec(`INSERT INTO user_packages (user_id, package, status, ref_id, sale, created_at, updated_at)
			VALUES (?, ?, 'pending', ?, 'other', NOW(), NOW())`,
			saveUser, itoaU(buyPackage.ID), itoaU(refID)); err != nil {
			httpx.Error(w, http.StatusInternalServerError, "Database error")
			return
		}

		httpx.OK(w, map[string]interface{}{"status": "success", "parent_data": parentData})
	}
}

// Commercial terms come from the backend so selection screens cannot drift.
type purchasePackageChoice struct {
	models.Package
	Terms *blockchain.PackageTerms `json:"commercial_terms,omitempty"`
}

func purchasePackageChoices(packages []models.Package, blockchainEnabled bool) []purchasePackageChoice {
	choices := make([]purchasePackageChoice, 0, len(packages))
	for _, pkg := range packages {
		choice := purchasePackageChoice{Package: pkg}
		if blockchainEnabled {
			terms, err := blockchain.CommercialPackageTerms(pkg.Price)
			if err != nil {
				continue
			}
			choice.Terms = &terms
		}
		choices = append(choices, choice)
	}
	return choices
}

func optionalPackageText(value *string) interface{} {
	if value == nil || strings.TrimSpace(*value) == "" {
		return nil
	}
	return strings.TrimSpace(*value)
}

func validateCommercialPackagePayload(d *app.Deps, body *packagePayload, errs map[string][]string) {
	if d.Cfg.BlockchainEnabled && body.Status != nil && *body.Status == "active" && body.Price != nil && len(errs["price"]) == 0 {
		if _, err := blockchain.CommercialPackageTerms(int64(*body.Price)); err != nil {
			errs["price"] = []string{"This price has no approved blockchain commercial terms."}
		}
	}
}
