// dashboard_handler.go covers AdminController, AgentController,
// CompanyController and — ONLY — UserController@index (GET /user/dashboard).
// Everything else on UserController (search/update/leader-codes/etc) lives
// in a different handler file; don't duplicate it here.
//
// See api_spec.md's "## AdminController" / "## AgentController" /
// "## CompanyController" / "## UserController" sections. All the per-role
// dashboard-aggregation quirks documented there are preserved verbatim,
// including ones that look like bugs:
//   - Admin's myGlobleDirectorShare reads GlobalShareWalletLog (first(), no
//     ordering — an arbitrary row); Agent/User's reads GlobalShareWallet
//     instead — a genuine inconsistency in the original, not a typo here.
//   - Company's myWallet is the raw `wallets` row; Admin/Agent/User's
//     myWallet is the active-UserPackage collection summed price*4.
//   - The myshareValue formula (poolAmount/totalPoolshareValue * share) is
//     identical everywhere it appears — flagged for the math agent in
//     api_spec.md, reproduced verbatim here, not re-derived.
//
// The original's rank()/roc()/allUsers()/newActivations() raw-HTML Blade
// helpers are replaced by internal/tree's structured equivalents (see
// ui_spec.md's "Helper-driven inline widgets" section for what each showed).
package handlers

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"signet-backend/internal/app"
	"signet-backend/internal/auth"
	"signet-backend/internal/httpx"
	"signet-backend/internal/models"
	"signet-backend/internal/tree"
	"signet-backend/internal/wallet"
)

func RegisterDashboardRoutes(r chi.Router, d *app.Deps) {
	r.Group(func(r chi.Router) {
		r.Use(d.Auth.RequireAuth)
		r.Use(auth.RequireRole("admin"))
		r.Get("/api/v1/admin/dashboard", adminDashboardHandler(d))
		r.Post("/api/v1/admin/activate/{user}", activateUserHandler(d, "admin.dashboard"))
	})

	r.Group(func(r chi.Router) {
		r.Use(d.Auth.RequireAuth)
		r.Use(auth.RequireRole("agent"))
		r.Get("/api/v1/agent/dashboard", agentDashboardHandler(d))
	})

	r.Group(func(r chi.Router) {
		r.Use(d.Auth.RequireAuth)
		r.Use(auth.RequireRole("company"))
		r.Get("/api/v1/company/dashboard", companyDashboardHandler(d))
		r.Post("/api/v1/company/activate/{user}", activateUserHandler(d, "company.dashboard"))
		r.Get("/api/v1/company/pending-activation", companyPendingActivationHandler(d))
		r.Post("/api/v1/company/new-active-package", companyNewActivePackageHandler(d))
		r.Get("/api/v1/company/new-activations", companyNewActivationsHandler(d))
	})

	r.Group(func(r chi.Router) {
		r.Use(d.Auth.RequireAuth)
		r.Use(auth.RequireRole("user"))
		r.Get("/api/v1/user/dashboard", userDashboardHandler(d))
	})
}

// --- activate-user: identical bodies on AdminController & CompanyController ---

// activateUserHandler ports AdminController@activateUser /
// CompanyController@activateUser — both just flip users.status to 'active'
// and save; no package/wallet side effects (that's the separate
// token_handler.go active-package flow). The original redirects to the
// role's dashboard route with a flash message; `dashboardName` is carried
// through only as an informational value in the JSON reply.
func activateUserHandler(d *app.Deps, dashboardName string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := parseUintParam(chi.URLParam(r, "user"))
		if !ok {
			httpx.Error(w, http.StatusNotFound, "User not found")
			return
		}
		res, err := d.DB.Exec("UPDATE users SET status = 'active', updated_at = NOW() WHERE id = ?", userID)
		if err != nil {
			httpx.Error(w, http.StatusInternalServerError, "Database error")
			return
		}
		if n, _ := res.RowsAffected(); n == 0 {
			httpx.OK(w, map[string]interface{}{"status": "error", "message": "User not found"})
			return
		}
		httpx.OK(w, map[string]interface{}{"status": "success", "message": "User activated successfully.", "redirect": dashboardName})
	}
}

// --- shared dashboard aggregation helpers (Admin/Agent/Company/User) ---

func dashRefLink(d *app.Deps, userID uint64) string {
	var code string
	_ = d.DB.Get(&code, "SELECT code FROM referral_codes WHERE user_id = ? LIMIT 1", userID)
	return strings.TrimRight(d.Cfg.AppURL, "/") + "/register?ref=" + code
}

func dashActiveTokenCount(d *app.Deps, userID uint64) (int, error) {
	var count int
	err := d.DB.Get(&count, "SELECT COUNT(*) FROM tokens WHERE user_id = ? AND status = 'active'", userID)
	return count, err
}

// dashTokenCountRow ports the Token::selectRaw(...)->groupBy('user_id')
// active/deactive breakdown used by Admin & Company dashboards.
type dashTokenCountRow struct {
	UserID        uint64            `db:"user_id" json:"user_id"`
	ActiveCount   int               `db:"active_count" json:"active_count"`
	DeactiveCount int               `db:"deactive_count" json:"deactive_count"`
	UserName      models.NullString `db:"user_name" json:"user_name"`
	UserWhatsapp  models.NullString `db:"user_whatsapp" json:"user_whatsapp"`
}

func dashTokenCounts(d *app.Deps, perPage, offset int) []dashTokenCountRow {
	var rows []dashTokenCountRow
	_ = d.DB.Select(&rows, `
		SELECT t.user_id,
		       SUM(CASE WHEN t.status = 'active' THEN 1 ELSE 0 END) AS active_count,
		       SUM(CASE WHEN t.status = 'deactive' THEN 1 ELSE 0 END) AS deactive_count,
		       u.name AS user_name, u.whatsapp_number AS user_whatsapp
		FROM tokens t LEFT JOIN users u ON u.id = t.user_id
		GROUP BY t.user_id
		ORDER BY t.user_id
		LIMIT ? OFFSET ?`, perPage, offset)
	return rows
}

type dashPackageRow struct {
	ID          uint64            `db:"id" json:"id"`
	Package     string            `db:"package" json:"package"`
	PackageName models.NullString `db:"name" json:"package_name"`
	Price       models.NullInt64  `db:"price" json:"price"`
	ActivatedAt models.NullTime   `db:"activated_at" json:"activated_at"`
}

// dashMyWallet ports the current user's active UserPackages (with the
// `userpackage` Package relation) plus totalValue = sum(price*4) — used as
// `myWallet`/`totalValue` on Admin/Agent/User dashboards. Company's
// dashboard uses the raw `wallets` row instead (see companyDashboardHandler).
func dashMyWallet(d *app.Deps, userID uint64) ([]dashPackageRow, float64, error) {
	var rows []dashPackageRow
	err := d.DB.Select(&rows, `
		SELECT up.id, up.package, up.activated_at, p.name, p.price
		FROM user_packages up
		JOIN packages p ON p.id = CAST(up.package AS UNSIGNED)
		WHERE up.user_id = ? AND up.status = 'active'`, userID)
	if err != nil {
		return nil, 0, err
	}
	var total float64
	for _, row := range rows {
		total += float64(row.Price.Int64) * 4
	}
	return rows, total, nil
}

type dashPackageDetail struct {
	ID         uint64            `db:"id" json:"id"`
	Package    string            `db:"package" json:"package"`
	Name       models.NullString `db:"name" json:"name"`
	Price      models.NullInt64  `db:"price" json:"price"`
	Commission models.NullInt64  `db:"commission" json:"commission"`
	Rank       models.NullString `db:"rank" json:"rank"`
}

// dashMyPackage ports "$myPackage = highest-price active UserPackage (join
// packages, order by price desc); $feePercentage = that package's
// commission" — used by Admin/Agent's dashboards. Rank is
// $myPackage->userpackage->rank, shown next to the user's Signet ID in the
// welcome card header on every Admin/Agent/User dashboard.
func dashMyPackage(d *app.Deps, userID uint64) (*dashPackageDetail, int64) {
	var row dashPackageDetail
	err := d.DB.Get(&row, `
		SELECT up.id, up.package, p.name, p.price, p.commission, p.rank
		FROM user_packages up JOIN packages p ON p.id = CAST(up.package AS UNSIGNED)
		WHERE up.user_id = ? AND up.status = 'active'
		ORDER BY p.price DESC LIMIT 1`, userID)
	if err != nil {
		return nil, 0
	}
	return &row, row.Commission.Int64
}

func dashPoolAmountCurrentMonth(d *app.Deps) (float64, error) {
	var total models.NullFloat64
	err := d.DB.Get(&total, `
		SELECT COALESCE(SUM(pool_amount), 0) FROM package_pools
		WHERE MONTH(created_at) = MONTH(CURDATE()) AND YEAR(created_at) = YEAR(CURDATE())`)
	return total.Float64, err
}

func dashTotalPoolshareValue(d *app.Deps) (float64, error) {
	var total models.NullFloat64
	err := d.DB.Get(&total, "SELECT COALESCE(SUM(global_director_share), 0) FROM users WHERE global_director_share_status = 1")
	return total.Float64, err
}

// dashMyShareValue ports "$myshareValue = ($poolAmount/$totalPoolshareValue)
// * auth()->user()->global_director_share if totalPoolshareValue>0, else 0"
// — the identical formula on every dashboard that computes it (flagged for
// the math agent in api_spec.md; reproduced verbatim here, not re-derived).
func dashMyShareValue(poolAmount, totalPoolshareValue, myShare float64) float64 {
	if totalPoolshareValue > 0 {
		return (poolAmount / totalPoolshareValue) * myShare
	}
	return 0
}

type dashActivationRow struct {
	ID            uint64            `db:"id" json:"id"`
	UserID        uint64            `db:"user_id" json:"user_id"`
	Package       string            `db:"package" json:"package"`
	Status        string            `db:"status" json:"status"`
	Sale          string            `db:"sale" json:"sale"`
	CompanyStatus int               `db:"company_status" json:"company_status"`
	CreatedAt     models.NullTime   `db:"created_at" json:"created_at"`
	UserName      models.NullString `db:"user_name" json:"user_name"`
	UserEmail     models.NullString `db:"user_email" json:"user_email"`
	UserWhatsapp  models.NullString `db:"user_whatsapp" json:"user_whatsapp"`
	PackageName   models.NullString `db:"package_name" json:"package_name"`
	PackagePrice  models.NullInt64  `db:"package_price" json:"package_price"`
}

// dashActivations ports "$activations = pending UserPackages where
// ref_id = auth()->id(), with user,userpackage" — Admin/Agent's index, and
// User's index (which states status='pending' explicitly; same query
// either way since Admin/Agent's "pending UserPackage" already implies
// status='pending').
func dashActivations(d *app.Deps, refID uint64, perPage, offset int) ([]dashActivationRow, int, error) {
	var total int
	if err := d.DB.Get(&total, "SELECT COUNT(*) FROM user_packages WHERE ref_id = ? AND status = 'pending'", refID); err != nil {
		return nil, 0, err
	}
	var rows []dashActivationRow
	err := d.DB.Select(&rows, `
		SELECT up.id, up.user_id, up.package, up.status, up.sale, up.company_status, up.created_at,
		       u.name AS user_name, u.email AS user_email, u.whatsapp_number AS user_whatsapp,
		       p.name AS package_name, p.price AS package_price
		FROM user_packages up
		LEFT JOIN users u ON u.id = up.user_id
		LEFT JOIN packages p ON p.id = CAST(up.package AS UNSIGNED)
		WHERE up.ref_id = ? AND up.status = 'pending'
		ORDER BY up.id DESC LIMIT ? OFFSET ?`, refID, perPage, offset)
	return rows, total, err
}

// dashTreeWidgets bundles the tree.Rank/tree.Roc/tree.AllUsers/
// tree.NewActivations calls every dashboard makes (replacing the original's
// rank()/roc()/allUsers()/newActivations() inline-HTML helpers).
func dashTreeWidgets(d *app.Deps, user *models.User) (*tree.RankResult, *tree.RocSummary, int, int) {
	rankResult, _ := tree.Rank(d.DB, user.ID)
	var rocResult *tree.RocSummary
	if user.RocStatus.Valid && user.RocStatus.String == "active" {
		rocResult, _ = tree.Roc(d.DB, user.ID)
	}
	allUsersCount, _ := tree.AllUsers(d.DB)
	newActivationsCount, _ := tree.NewActivations(d.DB)
	return rankResult, rocResult, allUsersCount, newActivationsCount
}

// --- AdminController@index ---

func adminDashboardHandler(d *app.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := auth.UserFromContext(r.Context())
		page, perPage, offset := httpx.PageParams(r, 10)

		refLink := dashRefLink(d, user.ID)

		var tokenTotal int
		_ = d.DB.Get(&tokenTotal, "SELECT COUNT(DISTINCT user_id) FROM tokens")
		tokenCounts := dashTokenCounts(d, perPage, offset)

		myTokens, _ := dashActiveTokenCount(d, user.ID)
		myPackages, totalValue, _ := dashMyWallet(d, user.ID)
		myPackage, feePercentage := dashMyPackage(d, user.ID)
		poolAmount, _ := dashPoolAmountCurrentMonth(d)
		totalPoolshareValue, _ := dashTotalPoolshareValue(d)
		myshareValue := dashMyShareValue(poolAmount, totalPoolshareValue, user.GlobalDirectorShare)
		walletBalance, _ := wallet.Balance(d.DB, user.ID)

		// AdminController@index: GlobalShareWalletLog::where('user_id',
		// auth()->id())->first() — no ordering, an arbitrary single row if
		// several exist. Preserved verbatim (see api_spec.md).
		var gdsLog models.GlobalShareWalletLog
		var myGlobalDirectorShare interface{}
		if err := d.DB.Get(&gdsLog, "SELECT * FROM global_share_wallets_log WHERE user_id = ? LIMIT 1", strconv.FormatUint(user.ID, 10)); err == nil {
			myGlobalDirectorShare = gdsLog
		}

		activationRows, activationsTotal, _ := dashActivations(d, user.ID, perPage, offset)
		rankResult, rocResult, allUsersCount, newActivationsCount := dashTreeWidgets(d, user)

		httpx.OK(w, map[string]interface{}{
			"ref_link":                 refLink,
			"token_counts":             httpx.Paginate(tokenCounts, tokenTotal, page, perPage),
			"my_tokens":                myTokens,
			"my_wallet":                myPackages,
			"total_value":              totalValue,
			"wallet_balance":           walletBalance,
			"my_package":               myPackage,
			"fee_percentage":           feePercentage,
			"pool_amount":              poolAmount,
			"total_poolshare_value":    totalPoolshareValue,
			"my_share_value":           myshareValue,
			"my_globle_director_share": myGlobalDirectorShare,
			"activations":              httpx.Paginate(activationRows, activationsTotal, page, perPage),
			"rank":                     rankResult,
			"roc":                      rocResult,
			"all_users":                allUsersCount,
			"new_activations":          newActivationsCount,
		})
	}
}

// --- AgentController@index ---

func agentDashboardHandler(d *app.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := auth.UserFromContext(r.Context())
		page, perPage, offset := httpx.PageParams(r, 10)

		refLink := dashRefLink(d, user.ID)
		myTokens, _ := dashActiveTokenCount(d, user.ID)
		myPackages, totalValue, _ := dashMyWallet(d, user.ID)
		myPackage, feePercentage := dashMyPackage(d, user.ID)
		poolAmount, _ := dashPoolAmountCurrentMonth(d)
		totalPoolshareValue, _ := dashTotalPoolshareValue(d)
		myshareValue := dashMyShareValue(poolAmount, totalPoolshareValue, user.GlobalDirectorShare)
		walletBalance, _ := wallet.Balance(d.DB, user.ID)

		// AgentController@index queries GlobalShareWallet here, NOT
		// GlobalShareWalletLog like Admin does — an inconsistent model
		// choice in the original, preserved verbatim (see api_spec.md).
		var gsw models.GlobalShareWallet
		var myGlobalDirectorShare interface{}
		if err := d.DB.Get(&gsw, "SELECT * FROM global_share_wallets WHERE user_id = ? LIMIT 1", user.ID); err == nil {
			myGlobalDirectorShare = gsw
		}

		activationRows, activationsTotal, _ := dashActivations(d, user.ID, perPage, offset)
		rankResult, rocResult, allUsersCount, newActivationsCount := dashTreeWidgets(d, user)

		httpx.OK(w, map[string]interface{}{
			"ref_link":                 refLink,
			"my_tokens":                myTokens,
			"my_wallet":                myPackages,
			"total_value":              totalValue,
			"wallet_balance":           walletBalance,
			"my_package":               myPackage,
			"fee_percentage":           feePercentage,
			"pool_amount":              poolAmount,
			"total_poolshare_value":    totalPoolshareValue,
			"my_share_value":           myshareValue,
			"my_globle_director_share": myGlobalDirectorShare,
			"activations":              httpx.Paginate(activationRows, activationsTotal, page, perPage),
			"rank":                     rankResult,
			"roc":                      rocResult,
			"all_users":                allUsersCount,
			"new_activations":          newActivationsCount,
		})
	}
}

// --- CompanyController@index / activateUser / pendingActivation / newActivations / newActivepackage ---

func companyDashboardHandler(d *app.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := auth.UserFromContext(r.Context())
		page, perPage, offset := httpx.PageParams(r, 10)

		refLink := dashRefLink(d, user.ID)

		var tokenTotal int
		_ = d.DB.Get(&tokenTotal, "SELECT COUNT(DISTINCT user_id) FROM tokens")
		tokenCounts := dashTokenCounts(d, perPage, offset)

		myTokens, _ := dashActiveTokenCount(d, user.ID)

		// Unlike Admin/Agent/User (active-UserPackage sum), Company's
		// dashboard uses the raw `wallets` row for `myWallet` — preserved
		// verbatim (see api_spec.md).
		var companyWallet models.Wallet
		var myWallet interface{}
		if err := d.DB.Get(&companyWallet, "SELECT * FROM wallets WHERE user_id = ? LIMIT 1", user.ID); err == nil {
			myWallet = companyWallet
		}

		fromDate := r.URL.Query().Get("from_date")
		toDate := r.URL.Query().Get("to_date")

		// CompanyController@index only filters when BOTH dates are given
		// (whereBetween activated_at, startOfDay..endOfDay).
		where := "up.status = 'active'"
		var args []interface{}
		if fromDate != "" && toDate != "" {
			where += " AND up.activated_at BETWEEN ? AND ?"
			args = append(args, fromDate+" 00:00:00", toDate+" 23:59:59")
		}
		// Optional country filter (activating user's users.country_id); combines
		// with the date range or applies on its own.
		if countryID, err := strconv.ParseUint(r.URL.Query().Get("country_id"), 10, 64); err == nil && countryID > 0 {
			where += " AND up.user_id IN (SELECT id FROM users WHERE country_id = ?)"
			args = append(args, countryID)
		}

		type packageWiseRow struct {
			PackageID   uint64  `db:"package_id" json:"package_id"`
			PackageName string  `db:"name" json:"name"`
			Price       int64   `db:"price" json:"price"`
			ActiveCount int     `db:"active_count" json:"active_count"`
			TotalValue  float64 `db:"total_value" json:"total_value"`
		}
		var packageWiseCounts []packageWiseRow
		query := `
			SELECT p.id AS package_id, p.name, p.price,
			       COUNT(*) AS active_count, (p.price * COUNT(*)) AS total_value
			FROM user_packages up
			JOIN packages p ON p.id = CAST(up.package AS UNSIGNED)
			WHERE ` + where + `
			GROUP BY p.id, p.name, p.price
			ORDER BY p.id`
		_ = d.DB.Select(&packageWiseCounts, query, args...)

		var grandTotal float64
		for _, row := range packageWiseCounts {
			grandTotal += row.TotalValue
		}

		// company/dashboard.blade.php's "Generate Token" picker lists
		// AppModelsUser::all() — every user, any role or status.
		var userIDs []uint64
		_ = d.DB.Select(&userIDs, "SELECT id FROM users ORDER BY id")

		// Country filter options.
		var countries []struct {
			ID   uint64 `db:"id" json:"id"`
			Code string `db:"code" json:"code"`
			Name string `db:"name" json:"name"`
		}
		_ = d.DB.Select(&countries, "SELECT id, code, name FROM countries WHERE deleted_at IS NULL ORDER BY name")

		rankResult, rocResult, allUsersCount, newActivationsCount := dashTreeWidgets(d, user)

		httpx.OK(w, map[string]interface{}{
			"ref_link":            refLink,
			"user_ids":            userIDs,
			"token_counts":        httpx.Paginate(tokenCounts, tokenTotal, page, perPage),
			"my_tokens":           myTokens,
			"my_wallet":           myWallet,
			"package_wise_counts": packageWiseCounts,
			"grand_total":         grandTotal,
			"countries":           countries,
			"rank":                rankResult,
			"roc":                 rocResult,
			"all_users":           allUsersCount,
			"new_activations":     newActivationsCount,
		})
	}
}

func companyPendingActivationHandler(d *app.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		page, perPage, offset := httpx.PageParams(r, 10)
		var total int
		_ = d.DB.Get(&total, "SELECT COUNT(*) FROM user_packages WHERE status = 'pending'")

		// NOTE: unlike Admin/Agent's pending-activation lists, this is NOT
		// filtered by ref_id — it shows ALL pending packages company-wide,
		// preserved verbatim (see api_spec.md).
		var rows []dashActivationRow
		_ = d.DB.Select(&rows, `
			SELECT up.id, up.user_id, up.package, up.status, up.sale, up.created_at,
			       u.name AS user_name, u.email AS user_email, u.whatsapp_number AS user_whatsapp,
			       p.name AS package_name, p.price AS package_price
			FROM user_packages up
			LEFT JOIN users u ON u.id = up.user_id
			LEFT JOIN packages p ON p.id = CAST(up.package AS UNSIGNED)
			WHERE up.status = 'pending'
			ORDER BY up.id DESC LIMIT ? OFFSET ?`, perPage, offset)

		httpx.OK(w, map[string]interface{}{
			"activations": httpx.Paginate(rows, total, page, perPage),
		})
	}
}

func companyNewActivationsHandler(d *app.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		page, perPage, offset := httpx.PageParams(r, 10)
		var total int
		_ = d.DB.Get(&total, "SELECT COUNT(*) FROM user_packages WHERE company_status = 0")

		var rows []dashActivationRow
		_ = d.DB.Select(&rows, `
			SELECT up.id, up.user_id, up.package, up.status, up.sale, up.created_at,
			       u.name AS user_name, u.email AS user_email, u.whatsapp_number AS user_whatsapp,
			       p.name AS package_name, p.price AS package_price
			FROM user_packages up
			LEFT JOIN users u ON u.id = up.user_id
			LEFT JOIN packages p ON p.id = CAST(up.package AS UNSIGNED)
			WHERE up.company_status = 0
			ORDER BY up.id DESC LIMIT ? OFFSET ?`, perPage, offset)

		httpx.OK(w, map[string]interface{}{
			"activations": httpx.Paginate(rows, total, page, perPage),
		})
	}
}

// companyNewActivePackageHandler ports CompanyController@newActivepackage —
// flips company_status to 1 on a UserPackage; does NOT set status='active'
// (that's the separate token_handler.go active-package flow) — preserved
// verbatim, see api_spec.md.
func companyNewActivePackageHandler(d *app.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			PackageID uint64 `json:"package_id"`
		}
		_ = decodeJSON(r, &body)

		var totalExists int
		if body.PackageID != 0 {
			_ = d.DB.Get(&totalExists, "SELECT COUNT(*) FROM user_packages WHERE id = ?", body.PackageID)
		}
		if body.PackageID == 0 || totalExists == 0 {
			httpx.JSON(w, http.StatusUnprocessableEntity, map[string]interface{}{
				"success": false,
				"message": "Invalid package ID.",
				"errors":  map[string][]string{"package_id": {"The package id field is required."}},
			})
			return
		}

		var pendingExists int
		_ = d.DB.Get(&pendingExists, "SELECT COUNT(*) FROM user_packages WHERE id = ? AND company_status = 0", body.PackageID)
		if pendingExists == 0 {
			httpx.JSON(w, http.StatusNotFound, map[string]interface{}{"success": false, "message": "Package not found or already activated."})
			return
		}

		if _, err := d.DB.Exec("UPDATE user_packages SET company_status = 1, updated_at = NOW() WHERE id = ?", body.PackageID); err != nil {
			httpx.JSON(w, http.StatusInternalServerError, map[string]interface{}{"success": false, "message": "Something went wrong. Please try again."})
			return
		}

		httpx.OK(w, map[string]interface{}{"success": true, "message": "Package activated successfully!"})
	}
}

// --- UserController@index (dashboard ONLY — every other UserController method lives elsewhere) ---

func userDashboardHandler(d *app.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := auth.UserFromContext(r.Context())
		page, perPage, offset := httpx.PageParams(r, 10)

		refLink := dashRefLink(d, user.ID)
		myTokens, _ := dashActiveTokenCount(d, user.ID)
		myPackages, totalValue, _ := dashMyWallet(d, user.ID)

		// UserController@index computes $myPackage/$feePercentage twice; the
		// second query is the same order-by-price-desc lookup plus
		// ->latest() as a tie-break. Captured here as one query with that
		// tie-break rather than reproducing the redundant duplicate lookup.
		var myPackage dashPackageDetail
		var myPackagePtr *dashPackageDetail
		var feePercentage int64
		err := d.DB.Get(&myPackage, `
			SELECT up.id, up.package, p.name, p.price, p.commission, p.rank
			FROM user_packages up JOIN packages p ON p.id = CAST(up.package AS UNSIGNED)
			WHERE up.user_id = ? AND up.status = 'active'
			ORDER BY p.price DESC, up.created_at DESC LIMIT 1`, user.ID)
		if err == nil {
			myPackagePtr = &myPackage
			feePercentage = myPackage.Commission.Int64
		}

		poolAmount, _ := dashPoolAmountCurrentMonth(d)
		totalPoolshareValue, _ := dashTotalPoolshareValue(d)
		myshareValue := dashMyShareValue(poolAmount, totalPoolshareValue, user.GlobalDirectorShare)
		walletBalance, _ := wallet.Balance(d.DB, user.ID)

		// Same GlobalShareWallet (not …Log) source table as Agent's dashboard.
		var gsw models.GlobalShareWallet
		var myGlobalDirectorShare interface{}
		if err := d.DB.Get(&gsw, "SELECT * FROM global_share_wallets WHERE user_id = ? LIMIT 1", user.ID); err == nil {
			myGlobalDirectorShare = gsw
		}

		activationRows, activationsTotal, _ := dashActivations(d, user.ID, perPage, offset)
		rankResult, rocResult, allUsersCount, newActivationsCount := dashTreeWidgets(d, user)

		httpx.OK(w, map[string]interface{}{
			"ref_link":                 refLink,
			"my_tokens":                myTokens,
			"my_wallet":                myPackages,
			"total_value":              totalValue,
			"wallet_balance":           walletBalance,
			"my_package":               myPackagePtr,
			"fee_percentage":           feePercentage,
			"pool_amount":              poolAmount,
			"total_poolshare_value":    totalPoolshareValue,
			"my_share_value":           myshareValue,
			"my_globle_director_share": myGlobalDirectorShare,
			"activations":              httpx.Paginate(activationRows, activationsTotal, page, perPage),
			"rank":                     rankResult,
			"roc":                      rocResult,
			"all_users":                allUsersCount,
			"new_activations":          newActivationsCount,
		})
	}
}
