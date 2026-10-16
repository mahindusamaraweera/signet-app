package handlers

import (
	"database/sql"
	"errors"
	"github.com/go-chi/chi/v5"
	"log"
	"net/http"
	"regexp"
	"signet-backend/internal/app"
	"signet-backend/internal/auth"
	"signet-backend/internal/blockchain"
	"signet-backend/internal/httpx"
	"time"
)

var linkProofPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{64}$`)

func RegisterTierAccountLinkRoutes(r chi.Router, d *app.Deps) {
	r.Group(func(r chi.Router) {
		r.Use(d.Auth.RequireAuth)
		r.Get("/api/v1/blockchain/tier-link", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Cache-Control", "no-store")
			var id string
			err := d.DB.Get(&id, "SELECT tier_user_id FROM tier_account_links WHERE user_id=?", auth.UserFromContext(r.Context()).ID)
			if err == sql.ErrNoRows {
				httpx.JSON(w, 200, map[string]interface{}{"linked": false})
				return
			}
			if err != nil {
				httpx.Error(w, 503, "Account linking temporarily unavailable")
				return
			}
			httpx.JSON(w, 200, map[string]interface{}{"linked": true, "tier_user_id": id})
		})
		r.Post("/api/v1/blockchain/tier-link/begin", func(w http.ResponseWriter, r *http.Request) {
			if !linkRequestAllowed(d, w, r) {
				return
			}
			userID := auth.UserFromContext(r.Context()).ID
			var count int
			if err := d.DB.Get(&count, "SELECT COUNT(*) FROM tier_account_links WHERE user_id=?", userID); err != nil {
				httpx.Error(w, 503, "Account linking temporarily unavailable")
				return
			}
			if count > 0 {
				httpx.Error(w, 409, "Your Tier account is already linked")
				return
			}
			cookie, err := r.Cookie(auth.SessionCookieName)
			if err != nil {
				httpx.Error(w, 401, "Sign in again")
				return
			}
			state, err := blockchain.LinkSecret()
			if err != nil {
				httpx.Error(w, 503, "Could not create link proof")
				return
			}
			verifier, err := blockchain.LinkSecret()
			if err != nil {
				httpx.Error(w, 503, "Could not create link proof")
				return
			}
			sealed, err := blockchain.SealLinkVerifier(d.Auth.Secret, userID, state, verifier)
			if err != nil {
				httpx.Error(w, 503, "Account linking needs secure server configuration")
				return
			}
			authorizationURL, err := d.Blockchain.BeginTierLink(r.Context(), userID, state, verifier)
			if err != nil {
				log.Printf("tier-link/begin: user %d: %v", userID, err)
				if errors.Is(err, blockchain.ErrConflict) {
					httpx.Error(w, 409, "Tier connection needs identity review. Contact support; no balances or vesting have changed.")
					return
				}
				httpx.Error(w, 503, "Tier authorization temporarily unavailable")
				return
			}
			// Tier allows 10 minutes to approve, then 5 minutes to return the code.
			// The local proof must cover both windows; custody enforces each grant expiry.
			_, err = d.DB.Exec("INSERT INTO tier_account_link_requests (user_id,state_hash,session_hash,verifier_encrypted,expires_at) VALUES (?,?,?,?,?)", userID, blockchain.LinkHash(state), blockchain.LinkHash(cookie.Value), sealed, time.Now().Add(15*time.Minute))
			if err != nil {
				httpx.Error(w, 503, "Could not save link request; please start again")
				return
			}
			httpx.JSON(w, 200, map[string]string{"authorization_url": authorizationURL})
		})
		r.Post("/api/v1/blockchain/tier-link/complete", func(w http.ResponseWriter, r *http.Request) {
			if !linkRequestAllowed(d, w, r) {
				return
			}
			var body struct {
				Code  string `json:"code"`
				State string `json:"state"`
			}
			if decodeJSON(r, &body) != nil || !linkProofPattern.MatchString(body.Code) || !linkProofPattern.MatchString(body.State) {
				httpx.Error(w, 422, "Invalid account-link proof")
				return
			}
			cookie, err := r.Cookie(auth.SessionCookieName)
			if err != nil {
				httpx.Error(w, 401, "Sign in again")
				return
			}
			userID := auth.UserFromContext(r.Context()).ID
			tx, err := d.DB.Beginx()
			if err != nil {
				httpx.Error(w, 503, "Account linking temporarily unavailable")
				return
			}
			defer tx.Rollback()
			var request struct {
				ID          uint64    `db:"id"`
				SessionHash string    `db:"session_hash"`
				Sealed      string    `db:"verifier_encrypted"`
				Status      string    `db:"status"`
				Expires     time.Time `db:"expires_at"`
			}
			err = tx.Get(&request, "SELECT id,session_hash,verifier_encrypted,status,expires_at FROM tier_account_link_requests WHERE user_id=? AND state_hash=? FOR UPDATE", userID, blockchain.LinkHash(body.State))
			if err != nil || request.SessionHash != blockchain.LinkHash(cookie.Value) || !request.Expires.After(time.Now()) || request.Status != "pending" {
				httpx.Error(w, 409, "Link request expired, completed or belongs to another login; start again")
				return
			}
			verifier, err := blockchain.OpenLinkVerifier(d.Auth.Secret, userID, body.State, request.Sealed)
			if err != nil {
				httpx.Error(w, 503, "Could not recover link proof")
				return
			}
			result, err := d.Blockchain.ExchangeTierLink(r.Context(), userID, body.Code, verifier, body.State)
			if err != nil {
				result, err = d.Blockchain.TierLinkStatus(r.Context(), userID)
			}
			if err != nil || result == nil || result.Linked == nil || !*result.Linked || result.TierUserID == nil {
				httpx.Error(w, 503, "Link was not confirmed; retry this callback before it expires")
				return
			}
			if _, err = tx.Exec("INSERT IGNORE INTO tier_account_links (user_id,tier_user_id) VALUES (?,?)", userID, *result.TierUserID); err != nil {
				httpx.Error(w, 503, "Could not save verified link")
				return
			}
			var linkedID string
			if err = tx.Get(&linkedID, "SELECT tier_user_id FROM tier_account_links WHERE user_id=?", userID); err != nil || linkedID != *result.TierUserID {
				httpx.Error(w, 409, "Account-link conflict needs support review")
				return
			}
			if _, err = tx.Exec("UPDATE tier_account_link_requests SET status='completed',completed_at=NOW(),verifier_encrypted='' WHERE id=?", request.ID); err != nil {
				httpx.Error(w, 503, "Could not complete local link record")
				return
			}
			if err = tx.Commit(); err != nil {
				httpx.Error(w, 503, "Link result uncertain; check account-link status")
				return
			}
			httpx.JSON(w, 200, map[string]interface{}{"linked": true, "tier_user_id": linkedID})
		})
	})
}

func linkRequestAllowed(d *app.Deps, w http.ResponseWriter, r *http.Request) bool {
	w.Header().Set("Cache-Control", "no-store")
	if d.Blockchain == nil {
		httpx.Error(w, 503, "Blockchain integration is disabled")
		return false
	}
	if d.Cfg.FrontendOrigin == "" || r.Header.Get("Origin") != d.Cfg.FrontendOrigin {
		httpx.Error(w, 403, "Invalid account-link request origin")
		return false
	}
	return true
}
