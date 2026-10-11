package handlers

import (
	"log"
	"net/http"

	"signet-backend/internal/httpx"
)

func passwordEmailRequestHandler(issue func(*http.Request, string) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		var body struct {
			Email string `json:"email"`
		}
		if err := decodeJSON(r, &body); err != nil || !emailRe.MatchString(body.Email) {
			httpx.ValidationError(w, map[string][]string{"email": {"The email field must be a valid email address."}})
			return
		}
		// Cooldown, delivery failure and unknown account must not expose existence
		// through HTTP status or body. Never log the email, proof or SMTP error.
		if err := issue(r, body.Email); err != nil {
			log.Print("password reset issuance did not complete")
		}
		httpx.OK(w, map[string]interface{}{"status": "success", "message": "If your email is registered, a password reset link has been sent."})
	}
}
