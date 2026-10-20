package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
	"signet-backend/internal/blockchain"
)

type DepositMailSender interface {
	Send(context.Context, string, string, string) error
}
type confirmedMailDeposit struct {
	ID         uint64  `json:"id"`
	Asset      string  `json:"asset"`
	Amount     string  `json:"amount"`
	Hash       string  `json:"tx_hash"`
	LogIndex   uint64  `json:"log_index"`
	Status     string  `json:"status"`
	CreditedAt *string `json:"credited_at"`
}

var mailAmount = regexp.MustCompile(`^[0-9]{1,18}(\.[0-9]{1,18})?$`)
var mailHash = regexp.MustCompile(`^0x[0-9a-fA-F]{64}$`)

func depositMailEligible(d confirmedMailDeposit) bool {
	return d.ID > 0 && d.Asset == "USDT" && d.Status == "credited" && d.CreditedAt != nil && *d.CreditedAt != "" && mailAmount.MatchString(d.Amount) && strings.Trim(d.Amount, "0.") != "" && mailHash.MatchString(d.Hash)
}

func StartDepositNotifications(db *sqlx.DB, gateway *blockchain.Gateway, sender DepositMailSender) {
	if gateway == nil || sender == nil {
		return
	}
	go runOnTicker("deposit:notify", time.Minute, func() error { return RunDepositNotifications(db, gateway, sender) })
}

// Separate from money movement. SMTP's ambiguous acceptance is never retried
// automatically: a claimed row remains sending/review_required for reconciliation.
func RunDepositNotifications(db *sqlx.DB, gateway *blockchain.Gateway, sender DepositMailSender) error {
	var users []struct {
		ID     uint64 `db:"id"`
		Before uint64 `db:"before_id"`
	}
	if err := db.Select(&users, `SELECT u.id,COALESCE(s.before_id,0) AS before_id FROM users u
 JOIN registration_expiry_enrollments e ON e.user_id=u.id AND e.policy_version='new_unpaid_v1'
 LEFT JOIN deposit_notification_scans s ON s.user_id=u.id
 WHERE u.email_verified_at IS NOT NULL AND (EXISTS (SELECT 1 FROM package_purchase_intents p WHERE p.user_id=u.id)
 OR EXISTS (SELECT 1 FROM blockchain_wallet_provisioning_audit w WHERE w.source_user_id=u.id))
 ORDER BY COALESCE(s.last_checked_at,'1970-01-01'),u.id LIMIT 25`); err != nil {
		return err
	}
	var failures []error
	for _, user := range users {
		// Rotate unavailable users as well as successful ones.
		if _, err := db.Exec(`INSERT INTO deposit_notification_scans(user_id,before_id,last_checked_at) VALUES(?,?,NOW()) ON DUPLICATE KEY UPDATE last_checked_at=NOW()`, user.ID, user.Before); err != nil {
			failures = append(failures, err)
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		raw, err := gateway.Deposits(ctx, user.ID, user.Before)
		cancel()
		if err != nil {
			failures = append(failures, fmt.Errorf("deposit notification source unavailable: user_id=%d before_id=%d %s", user.ID, user.Before, blockchain.DepositFailureDiagnostic(err)))
			continue
		}
		var page struct {
			Deposits []confirmedMailDeposit `json:"deposits"`
			Next     *uint64                `json:"next_before_id"`
		}
		if err = json.Unmarshal(raw, &page); err != nil {
			failures = append(failures, fmt.Errorf("deposit notification invalid response: user_id=%d before_id=%d reason=invalid_deposit_page", user.ID, user.Before))
			continue
		}
		good := true
		for _, deposit := range page.Deposits {
			if !depositMailEligible(deposit) {
				continue
			}
			if _, err = db.Exec(`INSERT INTO deposit_notification_audit(remote_deposit_id,user_id,amount,tx_hash,log_index,status,created_at) VALUES(?,?,?,?,?,'pending',NOW()) ON DUPLICATE KEY UPDATE remote_deposit_id=remote_deposit_id`, deposit.ID, user.ID, deposit.Amount, deposit.Hash, deposit.LogIndex); err != nil {
				failures = append(failures, err)
				good = false
				break
			}
			var matches int
			if err = db.Get(&matches, `SELECT COUNT(*) FROM deposit_notification_audit WHERE remote_deposit_id=? AND user_id=? AND amount=? AND tx_hash=? AND log_index=?`, deposit.ID, user.ID, deposit.Amount, deposit.Hash, deposit.LogIndex); err != nil || matches != 1 {
				failures = append(failures, errors.New("deposit notification identity conflict"))
				good = false
				break
			}
		}
		if good {
			next := uint64(0)
			if page.Next != nil {
				next = *page.Next
			}
			if next != 0 && (len(page.Deposits) == 0 || (user.Before != 0 && next >= user.Before)) {
				failures = append(failures, errors.New("invalid deposit notification cursor"))
				continue
			}
			if _, err = db.Exec(`UPDATE deposit_notification_scans SET before_id=? WHERE user_id=?`, next, user.ID); err != nil {
				failures = append(failures, err)
			}
		}
	}
	var pending []struct {
		ID     uint64 `db:"remote_deposit_id"`
		Email  string `db:"email"`
		Amount string `db:"amount"`
		Hash   string `db:"tx_hash"`
	}
	if err := db.Select(&pending, `SELECT n.remote_deposit_id,u.email,n.amount,n.tx_hash FROM deposit_notification_audit n JOIN users u ON u.id=n.user_id WHERE n.status='pending' AND u.email_verified_at IS NOT NULL ORDER BY n.created_at,n.remote_deposit_id LIMIT 25`); err != nil {
		return errors.Join(append(failures, err)...)
	}
	for _, n := range pending {
		result, err := db.Exec(`UPDATE deposit_notification_audit SET status='sending',attempted_at=NOW() WHERE remote_deposit_id=? AND status='pending'`, n.ID)
		if err != nil {
			failures = append(failures, err)
			continue
		}
		count, err := result.RowsAffected()
		if err != nil || count != 1 {
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		err = sender.Send(ctx, n.Email, "Deposit confirmed", fmt.Sprintf("Your USDT deposit is confirmed.\nAmount credited: %s USDT.\nTransaction: https://bscscan.com/tx/%s\nCheck your SIGNET wallet for package funding progress. Deposit confirmation is not package activation; partial deposits may still need additional funds.\n", n.Amount, n.Hash))
		cancel()
		status := "sent"
		if err != nil {
			status = "review_required"
			failures = append(failures, errors.New("deposit mail requires delivery review"))
		}
		if _, err = db.Exec(`UPDATE deposit_notification_audit SET status=?,sent_at=CASE WHEN ?='sent' THEN NOW() ELSE NULL END WHERE remote_deposit_id=? AND status='sending'`, status, status, n.ID); err != nil {
			failures = append(failures, err)
		}
	}
	return errors.Join(failures...)
}
