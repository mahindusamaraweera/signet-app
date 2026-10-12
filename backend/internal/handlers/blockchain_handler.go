package handlers

import (
	"database/sql"
	"errors"
	"github.com/go-chi/chi/v5"
	"io"
	"log"
	"math/big"
	"net/http"
	"signet-backend/internal/app"
	"signet-backend/internal/auth"
	"signet-backend/internal/httpx"
	"strconv"
	"time"
)

// The local authenticated identity is always used; clients cannot select another user's wallet.
func RegisterBlockchainRoutes(r chi.Router, d *app.Deps) {
	r.Group(func(r chi.Router) {
		r.Use(d.Auth.RequireAuth)
		r.Use(httpx.SecurityRateLimit(10, time.Minute))
		r.Get("/api/v1/blockchain/security", withdrawalSecurityStatusHandler(d))
		// Device verification is deferred; its groundwork is deliberately not routed.
		r.Post("/api/v1/blockchain/security/email", withdrawalEmailIssueHandler(d))
		r.Post("/api/v1/blockchain/security/email/verify", withdrawalEmailVerifyHandler(d))
		r.Post("/api/v1/blockchain/security/authenticator", withdrawalAuthenticatorHandler(d, false))
		r.Post("/api/v1/blockchain/security/authenticator/confirm", withdrawalAuthenticatorHandler(d, true))
		r.Post("/api/v1/blockchain/security/address", withdrawalAddressHandler(d, false))
		r.Post("/api/v1/blockchain/security/address/confirm", withdrawalAddressHandler(d, true))
		r.Get("/api/v1/blockchain/security/addresses", withdrawalAddressListHandler(d))
		r.Post("/api/v1/blockchain/security/address/revoke", withdrawalAddressActionHandler(d, false, true))
		r.Post("/api/v1/blockchain/security/address/revoke/confirm", withdrawalAddressActionHandler(d, true, true))
		r.Post("/api/v1/blockchain/withdrawals/prepare", withdrawalRequestHandler(d, "prepare"))
		r.Post("/api/v1/blockchain/withdrawals/email", withdrawalRequestHandler(d, "email"))
		r.Post("/api/v1/blockchain/withdrawals/confirm", withdrawalRequestHandler(d, "confirm"))
		r.Post("/api/v1/blockchain/withdrawals/queue", withdrawalRequestHandler(d, "queue"))
		r.Get("/api/v1/blockchain/withdrawals/requests", withdrawalRequestsHandler(d))
		r.Post("/api/v1/blockchain/withdrawals/cancel", withdrawalCancelHandler(d))
	})
	r.Get("/api/v1/blockchain/withdrawals", d.Auth.RequireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if d.Blockchain == nil {
			httpx.Error(w, 503, "Blockchain integration is disabled")
			return
		}
		var before uint64
		if raw := r.URL.Query().Get("before_id"); raw != "" {
			var err error
			before, err = strconv.ParseUint(raw, 10, 64)
			if err != nil || before == 0 {
				httpx.Error(w, 422, "Invalid history cursor")
				return
			}
		}
		response, err := d.Blockchain.WithdrawalHistory(r.Context(), auth.UserFromContext(r.Context()).ID, before)
		if err != nil {
			httpx.Error(w, 503, "Withdrawal history temporarily unavailable")
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		httpx.JSON(w, 200, response)
	})).ServeHTTP)
	r.Get("/api/v1/blockchain/deposits", d.Auth.RequireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if d.Blockchain == nil {
			httpx.Error(w, 503, "Blockchain integration is disabled")
			return
		}
		var before uint64
		if raw := r.URL.Query().Get("before_id"); raw != "" {
			var err error
			before, err = strconv.ParseUint(raw, 10, 64)
			if err != nil || before == 0 {
				httpx.Error(w, 422, "Invalid history cursor")
				return
			}
		}
		response, err := d.Blockchain.Deposits(r.Context(), auth.UserFromContext(r.Context()).ID, before)
		if err != nil {
			httpx.Error(w, 503, "Deposit history temporarily unavailable")
			return
		}
		httpx.JSON(w, 200, response)
	})).ServeHTTP)
	r.With(d.Auth.RequireAuth, httpx.SecurityRateLimit(2, time.Minute)).Post("/api/v1/blockchain/purchase/sync", purchaseSyncHandler(d))
	r.Get("/api/v1/blockchain/purchase", d.Auth.RequireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if d.Blockchain == nil {
			httpx.Error(w, 503, "Blockchain integration is disabled")
			return
		}
		var result struct {
			ID                uint64  `db:"id" json:"id"`
			ReopeningStatus   string  `json:"reopening_status,omitempty"`
			PlacementRequired bool    `json:"placement_required"`
			Kind              string  `db:"purchase_kind" json:"purchase_kind"`
			Fee               string  `db:"service_fee_usdt" json:"service_fee_usdt"`
			Required          string  `db:"required_usdt" json:"required_usdt"`
			Price             string  `db:"package_price" json:"package_price"`
			Available         string  `db:"available_usdt" json:"available_usdt"`
			Remaining         string  `db:"remaining_usdt" json:"remaining_usdt"`
			LastAttempt       *string `db:"last_attempt_at" json:"last_attempt_at"`
			LastSyncError     *string `db:"last_sync_error" json:"last_sync_error"`
			Status            string  `db:"status" json:"status"`
			DepositSeen       bool    `db:"deposit_seen" json:"deposit_seen"`
		}
		if err := d.DB.Get(&result, `SELECT id,purchase_kind,package_price,COALESCE(CAST(service_fee_usdt AS CHAR),'') AS service_fee_usdt,COALESCE(CAST(required_usdt AS CHAR),'') AS required_usdt,available_usdt,remaining_usdt,status,deposit_seen,CAST(last_attempt_at AS CHAR) AS last_attempt_at,last_sync_error FROM package_purchase_intents WHERE user_id=? ORDER BY id DESC LIMIT 1`, auth.UserFromContext(r.Context()).ID); err != nil {
			if err == sql.ErrNoRows {
				httpx.Error(w, 404, "No package selected")
			} else {
				// e.g. "Unknown column 'last_attempt_at'" when a migration is
				// pending — the reason stays in the server log.
				log.Printf("blockchain/purchase: user %d: purchase status query failed: %v", auth.UserFromContext(r.Context()).ID, err)
				httpx.Error(w, 503, "Purchase status temporarily unavailable")
			}
			return
		}
		if result.Status == "awaiting_reopening" {
			if err := d.DB.Get(&result.ReopeningStatus, "SELECT status FROM registration_reopening_outbox WHERE purchase_intent_id=? AND user_id=?", result.ID, auth.UserFromContext(r.Context()).ID); err != nil {
				log.Printf("blockchain/purchase: user %d: reopening status for intent %d failed: %v", auth.UserFromContext(r.Context()).ID, result.ID, err)
				httpx.Error(w, 503, "Replacement confirmation status temporarily unavailable")
				return
			}
		}
		if result.Kind != "topup" && (result.Status == "awaiting_deposit" || result.Status == "expired") {
			var placements int
			if err := d.DB.Get(&placements, "SELECT COUNT(*) FROM user_parents WHERE user_id=? AND node IN ('deactive','active')", auth.UserFromContext(r.Context()).ID); err != nil {
				log.Printf("blockchain/purchase: user %d: placement status failed: %v", auth.UserFromContext(r.Context()).ID, err)
				httpx.Error(w, 503, "Placement status temporarily unavailable")
				return
			}
			result.PlacementRequired = placements == 0
		}
		w.Header().Set("Cache-Control", "no-store")
		httpx.OK(w, map[string]interface{}{"status": "success", "purchase": result})
	})).ServeHTTP)
	r.Post("/api/v1/blockchain/wallet", d.Auth.RequireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if d.Blockchain == nil {
			httpx.Error(w, http.StatusServiceUnavailable, "Blockchain integration is disabled")
			return
		}
		wallet, err := d.Blockchain.EnsureWallet(r.Context(), auth.UserFromContext(r.Context()).ID)
		if err != nil {
			httpx.Error(w, http.StatusServiceUnavailable, "Deposit wallet temporarily unavailable; please retry")
			return
		}
		httpx.OK(w, map[string]interface{}{"status": "success", "wallet": wallet})
	})).ServeHTTP)
}

func purchaseSyncHandler(d *app.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !d.Cfg.BlockchainEnabled || d.Blockchain == nil {
			httpx.Error(w, 503, "Blockchain integration is disabled")
			return
		}
		user := auth.UserFromContext(r.Context())
		var body struct {
			PurchaseID string `json:"purchase_id"`
		}
		if err := decodeJSON(r, &body); err != nil && !errors.Is(err, io.EOF) {
			httpx.Error(w, 422, "Select a valid purchase")
			return
		}
		var id uint64
		query := "SELECT id FROM package_purchase_intents WHERE user_id=? ORDER BY id DESC LIMIT 1"
		args := []interface{}{user.ID}
		if body.PurchaseID != "" {
			purchaseID, ok := parseUintParam(body.PurchaseID)
			if !ok || purchaseID == 0 {
				httpx.Error(w, 422, "Select a valid purchase")
				return
			}
			query = "SELECT id FROM package_purchase_intents WHERE user_id=? AND id=?"
			args = append(args, purchaseID)
		}
		if err := d.DB.Get(&id, query, args...); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				httpx.Error(w, 404, "Purchase not found. Refresh your package status.")
			} else {
				httpx.Error(w, 503, "Purchase status temporarily unavailable")
			}
			return
		}
		if err := retryAutomaticPurchase(d, id, user.ID); err != nil {
			message := "Activation check could not complete. Your account and recorded deposit remain available; please retry or contact support."
			if errors.Is(err, errPurchasePlacementRequired) {
				message = "Your placement needs review before activation can proceed. Your recorded deposit remains available."
			}
			httpx.Error(w, 503, message)
			return
		}
		var purchase struct {
			ID        uint64 `db:"id" json:"id"`
			Status    string `db:"status" json:"status"`
			Kind      string `db:"purchase_kind" json:"purchase_kind"`
			Available string `db:"available_usdt" json:"available_usdt"`
			Remaining string `db:"remaining_usdt" json:"remaining_usdt"`
		}
		if err := d.DB.Get(&purchase, "SELECT id,status,purchase_kind,available_usdt,remaining_usdt FROM package_purchase_intents WHERE id=? AND user_id=?", id, user.ID); err != nil {
			httpx.Error(w, 503, "The activation request was received, but status could not be refreshed. Refresh before trying again.")
			return
		}
		if purchase.Kind == "topup" && purchase.Status != "completed" && !d.Cfg.BlockchainTopupEnabled {
			httpx.Error(w, 503, "Top-up processing is temporarily disabled. Your recorded funds are preserved.")
			return
		}
		message := "Activation is processing. You can leave this page and return later; do not deposit again for this purchase."
		switch purchase.Status {
		case "completed":
			message = "Your package is active."
			if purchase.Kind == "topup" {
				message = "Your top-up is complete and your income cap has increased."
			}
		case "awaiting_deposit":
			message = "Balance checked. More confirmed USDT is needed to cover the package and service fee."
			if remaining, ok := new(big.Rat).SetString(purchase.Remaining); ok && remaining.Sign() == 0 {
				message = "Your confirmed balance covers the package and service fee. Activation is being processed; no further deposit is needed."
			}
		case "expired":
			httpx.Error(w, 409, "This purchase has expired. Contact support if you already deposited funds.")
			return
		case "awaiting_reopening":
			httpx.Error(w, 409, "Your placement is awaiting confirmation. Funds remain preserved.")
			return
		}
		httpx.OK(w, map[string]interface{}{"status": "success", "message": message, "purchase": purchase})
	}
}
