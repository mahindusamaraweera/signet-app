package handlers

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"

	"signet-backend/internal/withdrawalsecurity"
)

func issuePasswordReset(ctx context.Context, db *sql.DB, email, baseURL string, now time.Time, mailer withdrawalsecurity.Mailer) error {
	u, err := url.Parse(baseURL)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Scheme != "https" && !(u.Scheme == "http" && net.ParseIP(u.Hostname()) != nil && net.ParseIP(u.Hostname()).IsLoopback())) || mailer == nil {
		return errors.New("invalid reset delivery configuration")
	}
	token, err := newPasswordResetToken()
	if err != nil {
		return err
	}
	stored, err := passwordResetLookup(token)
	if err != nil {
		return err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var userID uint64
	if err := tx.QueryRowContext(ctx, `SELECT id FROM users WHERE email=? FOR UPDATE`, email).Scan(&userID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		return err
	}
	var recent, hourly int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM password_reset WHERE email=? AND created_at>=?`, email, now.UTC().Add(-time.Minute)).Scan(&recent); err != nil {
		return err
	}
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM password_reset WHERE email=? AND created_at>=?`, email, now.UTC().Add(-time.Hour)).Scan(&hourly); err != nil {
		return err
	}
	if recent > 0 || hourly >= 5 {
		return withdrawalsecurity.ErrChallengeCooldown
	}
	if _, err := tx.ExecContext(ctx, `UPDATE password_reset SET delivery_status='superseded' WHERE email=? AND delivery_status IN ('legacy','pending_delivery','delivered')`, email); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO password_reset (email,token,created_at,delivery_status) VALUES (?,?,?,'pending_delivery')`, email, stored, now.UTC()); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	// Fragments are not sent in HTTP requests or Referer headers.
	link := strings.TrimRight(baseURL, "/") + "/password/reset/verify#token=" + token
	deliveryErr := mailer.Send(ctx, email, "Password Reset", fmt.Sprintf("Reset your password: %s\nThis link expires in one hour. If you did not request it, ignore this message. Do not share the link.", link))
	status := "delivered"
	if deliveryErr != nil {
		status = "delivery_failed"
	}
	recordCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
	defer cancel()
	result, err := db.ExecContext(recordCtx, `UPDATE password_reset SET delivery_status=? WHERE email=? AND token=? AND delivery_status='pending_delivery'`, status, email, stored)
	if err != nil {
		return err
	}
	if rows, err := result.RowsAffected(); err != nil {
		return err
	} else if rows != 1 {
		return errResetProof
	}
	if deliveryErr != nil {
		return errors.New("reset email delivery failed")
	}
	return nil
}
