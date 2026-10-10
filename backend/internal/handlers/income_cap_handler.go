package handlers

import (
	"database/sql"
	"net/http"
	"signet-backend/internal/app"
	"signet-backend/internal/auth"
	"signet-backend/internal/httpx"

	"github.com/go-chi/chi/v5"
)

func RegisterIncomeCapRoutes(r chi.Router, d *app.Deps) {
	r.Get("/api/v1/blockchain/income-cap", d.Auth.RequireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if d.Blockchain == nil {
			httpx.Error(w, 503, "Blockchain integration is disabled")
			return
		}
		id := auth.UserFromContext(r.Context()).ID
		var cap, balance sql.NullFloat64
		if err := d.DB.Get(&cap, `SELECT COALESCE(SUM(p.price*4),0) FROM user_packages up JOIN packages p ON p.id=CAST(up.package AS UNSIGNED) WHERE up.user_id=? AND up.status='active'`, id); err != nil {
			httpx.Error(w, 503, "Income cap temporarily unavailable")
			return
		}
		err := d.DB.Get(&balance, "SELECT balance FROM wallets WHERE user_id=?", id)
		if err != nil && err != sql.ErrNoRows {
			httpx.Error(w, 503, "Income balance temporarily unavailable")
			return
		}
		var notices []struct {
			ID      uint64 `db:"id" json:"id"`
			Amount  string `db:"commission_usd" json:"commission_usd"`
			Created string `db:"created_at" json:"created_at"`
			Kind    string `db:"commission_kind" json:"commission_kind"`
		}
		if err := d.DB.Select(&notices, "SELECT id,commission_usd,commission_kind,CAST(created_at AS CHAR) AS created_at FROM income_cap_diversions WHERE user_id=? ORDER BY id DESC LIMIT 10", id); err != nil {
			httpx.Error(w, 503, "Income cap notices temporarily unavailable")
			return
		}
		remaining := cap.Float64 - balance.Float64
		if remaining < 0 {
			remaining = 0
		}
		w.Header().Set("Cache-Control", "no-store")
		topupEnabled := incomeCapTopupEnabled(d)
		httpx.JSON(w, 200, map[string]interface{}{"cap_usd": ftoaF(cap.Float64), "income_balance_usd": ftoaF(balance.Float64), "remaining_usd": ftoaF(remaining), "cap_reached": cap.Float64 > 0 && balance.Float64 >= cap.Float64, "topup_enabled": topupEnabled, "diversions": notices})
	})).ServeHTTP)
}

func incomeCapTopupEnabled(d *app.Deps) bool {
	return d != nil && d.Cfg != nil && d.Blockchain != nil && d.Cfg.BlockchainEnabled && d.Cfg.BlockchainTopupEnabled
}
