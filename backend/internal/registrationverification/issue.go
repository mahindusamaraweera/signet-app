package registrationverification

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"

	"signet-backend/internal/auth"
)

type Data struct {
	Name          string `json:"name"`
	Whatsapp      string `json:"whatsapp_number"`
	CountryID     uint64 `json:"country_id"`
	ReferralCode  string `json:"referral_code"`
	LeaderCode    string `json:"leader_code"`
	ExecutiveCode string `json:"executive_code"`
	BinancePayID  string `json:"binance_pay_id,omitempty"`
}

type Mailer interface {
	Send(context.Context, string, string, string) error
}
type Service struct {
	db      *sql.DB
	key     []byte
	baseURL string
	mailer  Mailer
}

var ErrResendLimit = errors.New("registration verification resend limit reached")
var ErrEmailExists = errors.New("email is already registered")

func NewService(db *sql.DB, key []byte, baseURL string, mailer Mailer) (*Service, error) {
	u, err := url.Parse(baseURL)
	if db == nil || len(key) < 32 || mailer == nil || err != nil || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return nil, errors.New("invalid registration verification service configuration")
	}
	ip := net.ParseIP(u.Hostname())
	if u.Scheme != "https" && !(u.Scheme == "http" && ip != nil && ip.IsLoopback()) {
		return nil, errors.New("registration verification requires HTTPS")
	}
	return &Service{db: db, key: append([]byte(nil), key...), baseURL: strings.TrimRight(baseURL, "/"), mailer: mailer}, nil
}

// Issue accepts previously validated referral/country/details. Completion must
// revalidate them because email verification can occur up to a day later.
func (s *Service) Issue(ctx context.Context, email, password string, data Data, now time.Time) (string, error) {
	if data.Name == "" || data.CountryID == 0 || data.ReferralCode == "" || data.Whatsapp == "" || len(password) < 6 || len(password) > 72 {
		return "", errors.New("invalid pending registration data")
	}
	p, token, err := New(s.key, email, now)
	if err != nil {
		return "", err
	}
	hash, err := auth.HashPassword(password)
	if err != nil {
		return "", err
	}
	payload, err := json.Marshal(data)
	if err != nil {
		return "", err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `INSERT INTO registration_email_guards (email) VALUES (?) ON DUPLICATE KEY UPDATE email=email`, email); err != nil {
		return "", err
	}
	var lockedEmail string
	if err := tx.QueryRowContext(ctx, `SELECT email FROM registration_email_guards WHERE email=? FOR UPDATE`, email).Scan(&lockedEmail); err != nil {
		return "", err
	}
	var existing, recent, hourly int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM users WHERE email=?`, email).Scan(&existing); err != nil {
		return "", err
	}
	if existing != 0 {
		return "", ErrEmailExists
	}
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM pending_email_registrations WHERE email=? AND created_at>=?`, email, now.UTC().Add(-time.Minute)).Scan(&recent); err != nil {
		return "", err
	}
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM pending_email_registrations WHERE email=? AND created_at>=?`, email, now.UTC().Add(-time.Hour)).Scan(&hourly); err != nil {
		return "", err
	}
	if recent > 0 || hourly >= 5 {
		return "", ErrResendLimit
	}
	if _, err := tx.ExecContext(ctx, `UPDATE pending_email_registrations SET status='superseded' WHERE email=? AND status IN ('pending_delivery','delivered')`, email); err != nil {
		return "", err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO pending_email_registrations (id,email,registration_data,password_hash,verification_hash,status,expires_at,created_at) VALUES (?,?,?,?,?,'pending_delivery',?,?)`, p.ID, email, string(payload), hash, p.VerificationHash, p.ExpiresAt, now.UTC()); err != nil {
		return "", err
	}
	if err := tx.Commit(); err != nil {
		return "", err
	}
	// Fragment avoids sending the secret to frontend HTTP/access-log handlers.
	fragment := url.Values{"id": {p.ID}, "token": {token}}.Encode()
	link := fmt.Sprintf("%s/register/verify#%s", s.baseURL, fragment)
	var deliveryErr error
	if branded, ok := s.mailer.(interface {
		SendVerification(context.Context, string, string) error
	}); ok {
		deliveryErr = branded.SendVerification(ctx, email, link)
	} else {
		deliveryErr = s.mailer.Send(ctx, email, "Verify your SIGNET registration", fmt.Sprintf("Verify your email to create your account: %s\nThis link expires in 24 hours. Ignore it if you did not register.", link))
	}
	status := "delivered"
	if deliveryErr != nil {
		status = "delivery_failed"
	}
	recordCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
	defer cancel()
	result, err := s.db.ExecContext(recordCtx, `UPDATE pending_email_registrations SET status=? WHERE id=? AND status='pending_delivery'`, status, p.ID)
	if err != nil {
		return "", err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return "", err
	}
	if rows != 1 {
		return "", errors.New("registration verification request was superseded")
	}
	if deliveryErr != nil {
		return "", fmt.Errorf("registration verification email could not be delivered: %w", deliveryErr)
	}
	return p.ID, nil
}
