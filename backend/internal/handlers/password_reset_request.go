package handlers

import (
	"errors"
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
		// An unknown account is reported (owner policy, see
		// errPasswordResetUnknownEmail). Cooldown and delivery failure still
		// get the uniform success body. Never log the email, proof or SMTP error.
		if err := issue(r, body.Email); err != nil {
			if errors.Is(err, errPasswordResetUnknownEmail) {
				httpx.ValidationError(w, map[string][]string{"email": {"We couldn't find an account with that email address."}})
				return
			}
			log.Print("password reset issuance did not complete")
		}
		httpx.OK(w, map[string]interface{}{"status": "success", "message": "If your email is registered, a password reset link has been sent."})
	}
}
