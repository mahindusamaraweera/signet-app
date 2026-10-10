// Package auth implements the two principal types described in
// docs/analysis/ARCHITECTURE.md: an end-user session (JWT in an httpOnly
// cookie, replacing Laravel's `web` session guard) and an API-client bearer
// token compatible with the existing `personal_access_tokens` table
// (replacing Sanctum's `api` guard for ApiUser).
package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/jmoiron/sqlx"
	"golang.org/x/crypto/bcrypt"

	"signet-backend/internal/models"
)

const SessionCookieName = "signet_session"

type ctxKey string

const userCtxKey ctxKey = "signet_user"
const apiUserCtxKey ctxKey = "signet_api_user"

type Service struct {
	DB       *sqlx.DB
	Secret   []byte
	Lifetime time.Duration
	Secure   bool // set true in production (HTTPS) so cookies get Secure flag
}

func New(db *sqlx.DB, secret string, lifetime time.Duration, secure bool) *Service {
	return &Service{DB: db, Secret: []byte(secret), Lifetime: lifetime, Secure: secure}
}

// --- Password hashing (bcrypt, algorithm-compatible with PHP's Hash::make) ---

func HashPassword(plain string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(plain), 12) // BCRYPT_ROUNDS=12 in the original .env
	return string(b), err
}

func CheckPassword(hash, plain string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(plain)) == nil
}

// --- Session JWT (web guard equivalent) ---

type sessionClaims struct {
	UserID            uint64 `json:"uid"`
	Role              string `json:"role"`
	CredentialVersion string `json:"cv"`
	jwt.RegisteredClaims
}

func (s *Service) IssueSession(w http.ResponseWriter, user *models.User) error {
	loginID := make([]byte, 32)
	if _, err := rand.Read(loginID); err != nil {
		return err
	}
	claims := sessionClaims{
		UserID:            user.ID,
		Role:              user.Role,
		CredentialVersion: s.credentialVersion(user),
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        hex.EncodeToString(loginID),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(s.Lifetime)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(s.Secret)
	if err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    signed,
		Path:     "/",
		HttpOnly: true,
		Secure:   s.Secure,
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Now().Add(s.Lifetime),
	})
	return nil
}

func (s *Service) ClearSession(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   s.Secure,
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
	})
}

func (s *Service) parseSession(r *http.Request) (*sessionClaims, error) {
	cookie, err := r.Cookie(SessionCookieName)
	if err != nil {
		return nil, err
	}
	claims := &sessionClaims{}
	token, err := jwt.ParseWithClaims(cookie.Value, claims, func(t *jwt.Token) (interface{}, error) {
		return s.Secret, nil
	})
	if err != nil || !token.Valid {
		return nil, errors.New("invalid session")
	}
	return claims, nil
}

// RequireAuth loads the current user (by session cookie) into the request
// context, or responds 401 if there is none. This is the equivalent of the
// original app's implicit "if no auth()->user(), things break" endpoints —
// deliberately upgraded to a real 401, see ARCHITECTURE.md.
func (s *Service) RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims, err := s.parseSession(r)
		if err != nil {
			http.Error(w, `{"message":"Unauthenticated."}`, http.StatusUnauthorized)
			return
		}
		var user models.User
		if err := s.DB.Get(&user, "SELECT * FROM users WHERE id = ?", claims.UserID); err != nil || !s.sessionMatches(claims, &user) {
			http.Error(w, `{"message":"Unauthenticated."}`, http.StatusUnauthorized)
			return
		}
		ctx := context.WithValue(r.Context(), userCtxKey, &user)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// OptionalAuth loads the current user if a valid session cookie is present,
// but never blocks the request. Used for endpoints the original left public
// on purpose (welcome page, kyc.index's dual guest/company branch, etc).
func (s *Service) OptionalAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims, err := s.parseSession(r)
		if err == nil {
			var user models.User
			if dberr := s.DB.Get(&user, "SELECT * FROM users WHERE id = ?", claims.UserID); dberr == nil && s.sessionMatches(claims, &user) {
				ctx := context.WithValue(r.Context(), userCtxKey, &user)
				r = r.WithContext(ctx)
			}
		}
		next.ServeHTTP(w, r)
	})
}

// RequireRole is the Go equivalent of RoleMiddleware: comma-list of allowed
// roles, strict match against the session user's role column. Must be
// chained after RequireAuth. Responds 403 with the same message text the
// original used ("Unauthorized action.") for parity with any UI that
// special-cases it.
func RequireRole(roles ...string) func(http.Handler) http.Handler {
	allowed := make(map[string]bool, len(roles))
	for _, r := range roles {
		allowed[r] = true
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user := UserFromContext(r.Context())
			if user == nil || !allowed[user.Role] {
				http.Error(w, `{"message":"Unauthorized action."}`, http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func UserFromContext(ctx context.Context) *models.User {
	u, _ := ctx.Value(userCtxKey).(*models.User)
	return u
}

// --- API bearer tokens (Sanctum/ApiUser guard equivalent) ---

// NewAPIToken creates a personal_access_tokens row shaped exactly like
// Sanctum's, so tokens already issued by the original app keep validating
// unchanged, and returns the Sanctum-style "{id}|{plaintext}" string.
func (s *Service) NewAPIToken(apiUserID uint64, name string) (string, error) {
	raw := make([]byte, 40)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	plaintext := hex.EncodeToString(raw)
	hash := sha256.Sum256([]byte(plaintext))
	hashHex := hex.EncodeToString(hash[:])

	res, err := s.DB.Exec(
		`INSERT INTO personal_access_tokens (tokenable_type, tokenable_id, name, token, abilities, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, NOW(), NOW())`,
		"App\\Models\\ApiUser", apiUserID, name, hashHex, `["*"]`,
	)
	if err != nil {
		return "", err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return "", err
	}
	return strconv.FormatInt(id, 10) + "|" + plaintext, nil
}

// RequireAPIToken validates an `Authorization: Bearer {id}|{plaintext}` (or
// bare hash) header against personal_access_tokens, loads the owning
// ApiUser into context, and touches last_used_at.
func (s *Service) RequireAPIToken(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authz := r.Header.Get("Authorization")
		token := strings.TrimPrefix(authz, "Bearer ")
		if token == "" || token == authz && !strings.HasPrefix(authz, "Bearer ") {
			http.Error(w, `{"message":"Unauthenticated."}`, http.StatusUnauthorized)
			return
		}
		plaintext := token
		if idx := strings.Index(token, "|"); idx != -1 {
			plaintext = token[idx+1:]
		}
		hash := sha256.Sum256([]byte(plaintext))
		hashHex := hex.EncodeToString(hash[:])

		var pat models.PersonalAccessToken
		err := s.DB.Get(&pat, `SELECT * FROM personal_access_tokens WHERE token = ? AND tokenable_type = ?`, hashHex, "App\\Models\\ApiUser")
		if err != nil {
			http.Error(w, `{"message":"Unauthenticated."}`, http.StatusUnauthorized)
			return
		}
		var apiUser models.ApiUser
		if err := s.DB.Get(&apiUser, "SELECT * FROM api_users WHERE id = ?", pat.TokenableID); err != nil {
			http.Error(w, `{"message":"Unauthenticated."}`, http.StatusUnauthorized)
			return
		}
		_, _ = s.DB.Exec("UPDATE personal_access_tokens SET last_used_at = NOW() WHERE id = ?", pat.ID)

		ctx := context.WithValue(r.Context(), apiUserCtxKey, &apiUser)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func APIUserFromContext(ctx context.Context) *models.ApiUser {
	u, _ := ctx.Value(apiUserCtxKey).(*models.ApiUser)
	return u
}
