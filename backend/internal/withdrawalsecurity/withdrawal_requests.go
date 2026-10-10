package withdrawalsecurity

import (
	"context"
	"database/sql"
	"encoding/json"
	"strconv"
	"time"
)

type WithdrawalRequestStatus struct {
	ID                  string          `json:"id"`
	Terms               WithdrawalTerms `json:"terms"`
	AuthorizationStatus string          `json:"authorization_status"`
	QueueStatus         string          `json:"queue_status"`
	ExpiresAt           time.Time       `json:"expires_at"`
	CreatedAt           time.Time       `json:"created_at"`
	RemoteWithdrawalID  *string         `json:"remote_withdrawal_id"`
}

// Explicit owner projection. Never expose proof hashes, credential revisions,
// assertions, OTP evidence or raw worker errors. Reading does not authorize.
func (s *Service) ListWithdrawalRequests(ctx context.Context, userID uint64, now time.Time) ([]WithdrawalRequestStatus, error) {
	if userID == 0 {
		return nil, ErrSecurityCredentials
	}
	rows, err := s.db.QueryContext(ctx, `SELECT a.id,a.terms,a.status,a.expires_at,a.created_at,o.status,o.remote_withdrawal_id FROM withdrawal_authorizations a LEFT JOIN withdrawal_submission_outbox o ON o.authorization_id=a.id AND o.user_id=a.user_id WHERE a.user_id=? ORDER BY a.created_at DESC,a.id DESC LIMIT 50`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]WithdrawalRequestStatus, 0)
	for rows.Next() {
		var row WithdrawalRequestStatus
		var encoded string
		var queue sql.NullString
		var remote sql.NullInt64
		if err := rows.Scan(&row.ID, &encoded, &row.AuthorizationStatus, &row.ExpiresAt, &row.CreatedAt, &queue, &remote); err != nil {
			return nil, err
		}
		if json.Unmarshal([]byte(encoded), &row.Terms) != nil || row.Terms.UserID != userID {
			return nil, ErrWithdrawalAuthorization
		}
		canonical, err := NewWithdrawalTerms(userID, row.Terms.SecurityVersion, row.Terms.AddressID, row.Terms.Destination, row.Terms.Gross)
		if err != nil || canonical != row.Terms || !challengeIDPattern.MatchString(row.ID) {
			return nil, ErrWithdrawalAuthorization
		}
		if remote.Valid {
			if remote.Int64 <= 0 {
				return nil, ErrWithdrawalAuthorization
			}
			value := strconv.FormatInt(remote.Int64, 10)
			row.RemoteWithdrawalID = &value
		}
		row.QueueStatus = queue.String
		if (row.AuthorizationStatus == "pending" || row.AuthorizationStatus == "authorized") && !now.Before(row.ExpiresAt) {
			row.AuthorizationStatus = "expired"
		}
		result = append(result, row)
	}
	return result, rows.Err()
}
