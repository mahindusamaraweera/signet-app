package handlers

import (
	"errors"
	"github.com/go-chi/chi/v5"
	"log"
	"net/http"
	"regexp"
	"signet-backend/internal/app"
	"signet-backend/internal/auth"
	"signet-backend/internal/blockchain"
	"signet-backend/internal/httpx"
	"strings"
)

var adjustmentUUID = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

func RegisterBlockchainVestingRoutes(r chi.Router, d *app.Deps) {
	r.Group(func(r chi.Router) {
		r.Use(d.Auth.RequireAuth)
		r.Get("/api/v1/blockchain/vesting", func(w http.ResponseWriter, r *http.Request) {
			serveVesting(d, w, r, auth.UserFromContext(r.Context()).ID)
		})
		r.Post("/api/v1/blockchain/vesting/switch/{action}", func(w http.ResponseWriter, r *http.Request) {
			if d.Blockchain == nil {
				httpx.Error(w, 503, "Blockchain integration is disabled")
				return
			}
			if d.Cfg.FrontendOrigin == "" || r.Header.Get("Origin") != d.Cfg.FrontendOrigin {
				httpx.Error(w, 403, "Invalid request origin")
				return
			}
			var body blockchain.VestingSwitch
			if decodeJSON(r, &body) != nil || body.ScheduleID == 0 {
				httpx.Error(w, 422, "A valid schedule is required")
				return
			}
			id := auth.UserFromContext(r.Context()).ID
			var response []byte
			var err error
			switch chi.URLParam(r, "action") {
			case "preview":
				if body.Mode == "" {
					body.Mode = "existing"
				}
				if body.Mode != "existing" && body.Mode != "tier" {
					httpx.Error(w, 422, "Invalid preview mode")
					return
				}
				response, err = d.Blockchain.PreviewVestingSwitch(r.Context(), id, body.ScheduleID, body.Mode)
			case "commit":
				if !body.Consent || !adjustmentUUID.MatchString(body.RequestID) || body.ConfirmationToken == "" || len(body.ConfirmationToken) > 8192 {
					httpx.Error(w, 422, "Consent and the original preview confirmation are required")
					return
				}
				response, err = d.Blockchain.CommitVestingSwitch(r.Context(), id, body)
			default:
				httpx.Error(w, 404, "Unknown switch action")
				return
			}
			if err != nil {
				if errors.Is(err, blockchain.ErrRejected) {
					httpx.Error(w, 422, "Switch rejected; refresh and review a new preview")
				} else {
					httpx.Error(w, 503, "Switch was not confirmed; retry with the same request")
				}
				return
			}
			w.Header().Set("Cache-Control", "no-store")
			w.Header().Set("Content-Type", "application/json")
			w.Write(response)
		})
	})
	r.Group(func(r chi.Router) {
		r.Use(d.Auth.RequireAuth)
		r.Use(auth.RequireRole("company", "admin"))
		r.Get("/api/v1/blockchain/users/{userId}/vesting", func(w http.ResponseWriter, r *http.Request) {
			id, ok := parseUintParam(chi.URLParam(r, "userId"))
			if !ok {
				httpx.Error(w, 400, "Invalid user")
				return
			}
			serveVesting(d, w, r, id)
		})
		r.Post("/api/v1/blockchain/users/{userId}/vesting/adjust", func(w http.ResponseWriter, r *http.Request) {
			if d.Blockchain == nil {
				httpx.Error(w, 503, "Blockchain integration is disabled")
				return
			}
			id, ok := parseUintParam(chi.URLParam(r, "userId"))
			if !ok {
				httpx.Error(w, 400, "Invalid user")
				return
			}
			var body blockchain.VestingAdjustment
			if decodeJSON(r, &body) != nil || body.ScheduleID == 0 || !adjustmentUUID.MatchString(body.AdjustmentID) || (body.Direction != "add" && body.Direction != "subtract") || body.Days == 0 || body.Days > 36500 || len(strings.TrimSpace(body.Reason)) < 3 || len(body.Reason) > 2000 {
				httpx.Error(w, 422, "Valid adjustment ID, schedule, days, direction and reason are required")
				return
			}
			response, err := d.Blockchain.AdjustVesting(r.Context(), id, auth.UserFromContext(r.Context()).ID, body)
			if err != nil {
				if errors.Is(err, blockchain.ErrRejected) {
					httpx.Error(w, 422, "Adjustment rejected; refresh the schedule and review the terms before submitting again")
					return
				}
				httpx.Error(w, 503, "Adjustment was not confirmed; refresh and retry using the same adjustment ID")
				return
			}
			httpx.JSON(w, 200, response)
		})
	})
}

func serveVesting(d *app.Deps, w http.ResponseWriter, r *http.Request, id uint64) {
	if d.Blockchain == nil {
		httpx.Error(w, 503, "Blockchain integration is disabled")
		return
	}
	response, err := d.Blockchain.Vesting(r.Context(), id)
	if err != nil {
		log.Printf("blockchain/vesting: user %d: %v", id, err)
		if errors.Is(err, blockchain.ErrConflict) {
			httpx.Error(w, 409, "Your vesting identity needs reconciliation. Contact support; do not reactivate your package or deposit again.")
			return
		}
		httpx.Error(w, 503, "Vesting service temporarily unavailable")
		return
	}
	httpx.JSON(w, 200, response)
}
