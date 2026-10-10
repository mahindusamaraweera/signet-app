package jobs

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
	"signet-backend/internal/blockchain"
)

func StartRegistrationExpiry(db *sqlx.DB, gateway *blockchain.Gateway, enabled bool) {
	go runOnTicker("registration:expire", time.Minute, func() error { return RunRegistrationExpiry(db, gateway, enabled) })
}

// RunRegistrationExpiry deletes only unpaid, financially clear user accounts.
// Custody identities and independent audit records are never deleted or reused.
func RunRegistrationExpiry(db *sqlx.DB, gateway *blockchain.Gateway, enabled bool) error {
	var candidates []struct {
		UserID uint64 `db:"user_id"`
		LogID  uint64 `db:"log_id"`
	}
	cutoff := time.Now().Add(-10 * time.Hour)
	if err := db.Select(&candidates, `SELECT u.id AS user_id,
		COALESCE((SELECT MIN(l.id) FROM user_parent_map_logs l WHERE l.user_id=u.id),0) AS log_id
		FROM users u JOIN registration_expiry_enrollments e ON e.user_id=u.id
		WHERE e.policy_version='new_unpaid_v1' AND u.status='pending' AND u.role='user' AND u.id<>1 AND
		(EXISTS (SELECT 1 FROM user_parent_map_logs l WHERE l.user_id=u.id AND l.created_at<=?) OR
		(NOT EXISTS (SELECT 1 FROM user_parent_map_logs l WHERE l.user_id=u.id)
		AND u.email_verified_at IS NOT NULL AND u.created_at<=?))
		ORDER BY COALESCE(e.last_checked_at,e.registered_at),u.id LIMIT 100`, cutoff, cutoff); err != nil {
		return err
	}
	var failures []error
	for _, candidate := range candidates {
		// Rotate protected/unavailable candidates so they cannot starve later users.
		if _, err := db.Exec("UPDATE registration_expiry_enrollments SET last_checked_at=NOW() WHERE user_id=? AND policy_version='new_unpaid_v1'", candidate.UserID); err != nil {
			failures = append(failures, fmt.Errorf("expiry check scheduling user %d: %w", candidate.UserID, err))
			continue
		}
		if err := expireUnpaidAccount(db, gateway, enabled, candidate.LogID, candidate.UserID, cutoff); err != nil {
			failures = append(failures, fmt.Errorf("registration expiry user %d: %w", candidate.UserID, err))
		}
	}
	return errors.Join(failures...)
}

func expireRegistration(db *sqlx.DB, gateway *blockchain.Gateway, enabled bool, id uint64, cutoff time.Time) error {
	return expireUnpaidAccount(db, gateway, enabled, id, 0, cutoff)
}

func expireUnpaidAccount(db *sqlx.DB, gateway *blockchain.Gateway, enabled bool, id, owner uint64, cutoff time.Time) error {
	tx, err := db.Beginx()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	// Resolve owner without a lock, then use the same user-first ordering as
	// activation. Re-read the placement under lock before trusting that owner.
	if id != 0 {
		var placementOwner uint64
		if err := tx.Get(&placementOwner, "SELECT user_id FROM user_parent_map_logs WHERE id=?", id); err == sql.ErrNoRows {
			return nil
		} else if err != nil {
			return err
		}
		if owner != 0 && owner != placementOwner {
			return nil
		}
		owner = placementOwner
	}
	var user struct {
		Status   string       `db:"status"`
		Role     string       `db:"role"`
		Email    string       `db:"email"`
		Created  time.Time    `db:"created_at"`
		Verified sql.NullTime `db:"email_verified_at"`
	}
	if err := tx.Get(&user, "SELECT status,role,email,created_at,email_verified_at FROM users WHERE id=? FOR UPDATE", owner); errors.Is(err, sql.ErrNoRows) {
		return nil
	} else if err != nil {
		return err
	}
	if owner == 0 || owner == 1 || user.Status != "pending" || user.Role != "user" {
		return nil
	}
	// No backfill: every account that predates this release stays untouched.
	var policy string
	if err := tx.Get(&policy, "SELECT policy_version FROM registration_expiry_enrollments WHERE user_id=? FOR UPDATE", owner); errors.Is(err, sql.ErrNoRows) {
		return nil
	} else if err != nil {
		return err
	}
	if policy != "new_unpaid_v1" {
		return nil
	}
	var row struct {
		UserID  uint64    `db:"user_id"`
		Rows    string    `db:"created_row_ids"`
		Created time.Time `db:"created_at"`
	}
	if id != 0 {
		err = tx.Get(&row, "SELECT user_id, created_row_ids, created_at FROM user_parent_map_logs WHERE id=? FOR UPDATE", id)
		if err == sql.ErrNoRows {
			return nil
		}
		if err != nil {
			return err
		}
		if row.UserID != owner || row.Created.After(cutoff) {
			return nil
		}
	} else {
		if !user.Verified.Valid || user.Created.After(cutoff) {
			return nil
		}
		var placements int
		if err := tx.Get(&placements, "SELECT COUNT(*) FROM user_parent_map_logs WHERE user_id=? FOR UPDATE", owner); err != nil {
			return err
		}
		if placements != 0 {
			return nil
		}
		row.UserID, row.Rows, row.Created = owner, "[]", user.Created
	}
	var protected int
	err = tx.Get(&protected, `SELECT
		(SELECT COUNT(*) FROM user_packages WHERE user_id=? AND (status<>'pending' OR activated_at IS NOT NULL OR CAST(earn AS DECIMAL(36,18))<>0) FOR UPDATE) +
		(SELECT COUNT(*) FROM wallets WHERE user_id=? AND (balance<>0 OR roc_balance<>0) FOR UPDATE) +
		(SELECT COUNT(*) FROM tokens WHERE user_id=? FOR UPDATE) +
		(SELECT COUNT(*) FROM user_minings WHERE user_id=? FOR UPDATE) +
		(SELECT COUNT(*) FROM global_share_wallets WHERE user_id=? AND balance<>0 FOR UPDATE)`, row.UserID, row.UserID, row.UserID, row.UserID, row.UserID)
	if err != nil {
		return err
	}
	if protected > 0 {
		return nil
	}
	if protected, err = expiryHistoryProtection(tx, owner); err != nil {
		return err
	} else if protected > 0 {
		return nil
	}
	// Always check integration records, even if the blockchain flag was disabled
	// after an order was created. A flag change must not erase a funded account.
	var intents struct {
		Total     int `db:"total"`
		Protected int `db:"protected"`
	}
	if err := tx.Get(&intents, `SELECT COUNT(*) AS total, COALESCE(SUM(deposit_seen=TRUE OR available_usdt<>0 OR status NOT IN ('awaiting_deposit','expired')),0) AS protected FROM package_purchase_intents WHERE user_id=? FOR UPDATE`, owner); err != nil {
		return err
	}
	if intents.Protected > 0 {
		return nil
	}
	if !enabled {
		// An account can have a provisioned custody wallet even without a local
		// purchase intent. Disabling blockchain must never bypass its clearance.
		return blockchain.ErrUnavailable
	}
	var rowIDs []uint64
	if err := json.Unmarshal([]byte(row.Rows), &rowIDs); err != nil {
		return err
	}
	if id != 0 && len(rowIDs) == 0 {
		return fmt.Errorf("missing placement rows")
	}
	var custodySeal interface{}
	if enabled || intents.Total > 0 {
		if gateway == nil {
			return blockchain.ErrUnavailable
		}
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		// Provisioning is source-qualified and idempotent. It also lets accounts
		// that never opened the wallet page obtain authoritative custody clearance.
		wallet, err := gateway.EnsureWallet(ctx, owner)
		if err != nil {
			return err
		}
		if !expiryWalletEmpty(wallet) {
			return nil
		}
		sealID := id
		if id == 0 {
			// Stable separate reference namespace for verified registrations that
			// never selected a package; never reuse their old external user ID.
			if owner >= uint64(1)<<63 {
				return fmt.Errorf("expiry identity out of range")
			}
			sealID = owner | uint64(1)<<63
		}
		sealRequestedAt := time.Now().UTC().Truncate(time.Second)
		seal, err := gateway.SealRegistrationExpiry(ctx, owner, sealID)
		if err != nil {
			return err
		}
		if seal == nil {
			return nil
		}
		sealedAt, _ := time.Parse(time.RFC3339, seal.SealedAt)
		if sealedAt.Before(sealRequestedAt) {
			// The existing custody API replays its original seal without another
			// on-chain balance/scanner check. Never delete against stale evidence.
			return fmt.Errorf("stale custody expiry seal: account preserved for reconciliation")
		}
		// A previous attempt may have sealed custody then rolled back locally.
		// The idempotent seal alone is not proof that no deposit arrived since.
		if clear, err := expiryDepositsEmpty(ctx, gateway, owner); err != nil {
			return err
		} else if !clear {
			return nil
		}
		evidence, err := json.Marshal(seal)
		if err != nil {
			return err
		}
		custodySeal = string(evidence)
	}
	// Logs can contain other users' rows; remove only this user's inactive placement.
	if err := archiveAndDeleteUnpaidAccount(tx, owner, id, row.Rows, user.Email, row.Created.Add(10*time.Hour), custodySeal); err != nil {
		return err
	}
	return tx.Commit()
}

func expiryWalletEmpty(wallet *blockchain.Wallet) bool {
	if wallet == nil || wallet.ProvisioningStatus != "ready" || wallet.ChainID != 56 || len(wallet.Balances) != 2 {
		return false
	}
	for _, balance := range wallet.Balances {
		for _, amount := range []string{balance.Available, balance.Locked, balance.PendingWithdrawal} {
			// The gateway validates canonical decimals; reject any non-zero digit.
			for _, digit := range amount {
				if digit >= '1' && digit <= '9' {
					return false
				}
			}
		}
	}
	return true
}

func expiryDepositsEmpty(ctx context.Context, gateway *blockchain.Gateway, owner uint64) (bool, error) {
	raw, err := gateway.Deposits(ctx, owner, 0)
	if err != nil {
		return false, err
	}
	var result struct {
		Deposits *[]json.RawMessage `json:"deposits"`
	}
	if json.Unmarshal(raw, &result) != nil || result.Deposits == nil {
		return false, blockchain.ErrUnavailable
	}
	return len(*result.Deposits) == 0, nil
}

func expiryHistoryProtection(tx *sqlx.Tx, owner uint64) (int, error) {
	predicates := []string{
		"SELECT COUNT(*) FROM earn_logs WHERE user_id=?",
		"SELECT COUNT(*) FROM token_logs WHERE user_id=? OR shared_by=?",
		"SELECT COUNT(*) FROM roc_income_log WHERE user_id=?",
		"SELECT COUNT(*) FROM global_share_wallets_log WHERE user_id=?",
		"SELECT COUNT(*) FROM package_pools WHERE user_id=?",
		"SELECT COUNT(*) FROM salaries WHERE user_id=?",
		"SELECT COUNT(*) FROM kycs WHERE user_id=?",
		"SELECT COUNT(*) FROM leader_code_logs WHERE user_id=? OR changed_by=?",
		"SELECT COUNT(*) FROM executive_code_logs WHERE user_id=? OR changed_by=?",
		"SELECT COUNT(*) FROM users WHERE referred_by=?",
		"SELECT COUNT(*) FROM users WHERE id=? AND (global_director_share<>0 OR global_director_share_status<>0 OR leader_status='active')",
		"SELECT COUNT(*) FROM user_parents WHERE (user_id=? AND node<>'deactive') OR (user_id<>? AND (parent_id=? OR virtual_id=?))",
		"SELECT COUNT(*) FROM user_parent_map_logs WHERE user_id<>? AND parent_id=?",
		"SELECT COUNT(*) FROM tier_account_links WHERE user_id=?",
		"SELECT COUNT(*) FROM withdrawal_authorizations WHERE user_id=?",
		"SELECT COUNT(*) FROM withdrawal_submission_outbox WHERE user_id=?",
		"SELECT COUNT(*) FROM blockchain_commission_outbox WHERE beneficiary_user_id=?",
		"SELECT COUNT(*) FROM blockchain_topup_commission_plans WHERE original_user_id=? OR beneficiary_user_id=?",
		"SELECT COUNT(*) FROM income_cap_diversions WHERE user_id=?",
		"SELECT COUNT(*) FROM registration_reopening_outbox WHERE user_id=?",
		"SELECT COUNT(*) FROM blockchain_commission_outbox WHERE purchase_intent_id IN (SELECT id FROM package_purchase_intents WHERE user_id=?)",
		"SELECT IF(COUNT(*)>1,1,0) FROM user_parent_map_logs WHERE user_id=?",
	}
	var args []interface{}
	for _, predicate := range predicates {
		for n := 0; n < strings.Count(predicate, "?"); n++ {
			args = append(args, owner)
		}
	}
	var count int
	// Locking reads see current committed records, not the earlier owner lookup's
	// repeatable-read snapshot. Their locks are retained through deletion.
	err := tx.Get(&count, "SELECT ("+strings.Join(predicates, " FOR UPDATE) + (")+" FOR UPDATE)", args...)
	return count, err
}

func archiveAndDeleteUnpaidAccount(tx *sqlx.Tx, owner, logID uint64, rows, email string, deadline time.Time, seal interface{}) error {
	account, err := expirySnapshot(tx, "SELECT id,name,email,whatsapp_number,status,role,referred_by,created_at,email_verified_at FROM users WHERE id=?", owner)
	if err != nil {
		return err
	}
	packages, err := expirySnapshot(tx, "SELECT id,package,status,ref_id,sale,earn,created_at,activated_at FROM user_packages WHERE user_id=? ORDER BY id", owner)
	if err != nil {
		return err
	}
	intents, err := expirySnapshot(tx, "SELECT id,user_package_id,package_price,status,available_usdt,remaining_usdt,deposit_seen,remote_order_id,service_fee_usdt,required_usdt,bonus_percent,created_at FROM package_purchase_intents WHERE user_id=? ORDER BY id", owner)
	if err != nil {
		return err
	}
	placements, err := expirySnapshot(tx, "SELECT id,user_id,virtual_id,parent_id,node,created_at FROM user_parents WHERE user_id=? ORDER BY id", owner)
	if err != nil {
		return err
	}
	var placement interface{}
	if logID != 0 {
		placement = logID
	}
	if _, err := tx.Exec(`INSERT INTO registration_account_deletions (user_id,placement_log_id,reason,deadline,custody_identity,account_snapshot,package_snapshot,purchase_snapshot,placement_snapshot,custody_seal) VALUES (?,?,'unpaid_registration',?,?,?,?,?,?,?)`, owner, placement, deadline, "signet-app:"+strconv.FormatUint(owner, 10), account, packages, intents, placements, seal); err != nil {
		return err
	}
	if logID != 0 {
		if _, err := tx.Exec("INSERT INTO registration_expiries (placement_log_id,user_id,created_row_ids,custody_seal) VALUES (?,?,?,?)", logID, owner, rows, seal); err != nil {
			return err
		}
	}
	// Keep intents and audits as immutable old-identity recovery evidence.
	if _, err := tx.Exec("UPDATE package_purchase_intents SET status='expired' WHERE user_id=? AND status='awaiting_deposit' AND deposit_seen=FALSE", owner); err != nil {
		return err
	}
	for _, table := range []string{"referral_codes", "user_parent_map_logs", "user_parents", "user_packages", "wallets", "global_share_wallets", "user_secret_keys", "sessions", "withdrawal_security_challenges", "withdrawal_authenticator_enrollments", "withdrawal_totp_counters", "withdrawal_security_profiles", "withdrawal_security_addresses", "withdrawal_security_devices", "tier_account_link_requests"} {
		if _, err := tx.Exec("DELETE FROM "+table+" WHERE user_id=?", owner); err != nil {
			return err
		}
	}
	for _, table := range []string{"password_reset", "password_reset_tokens"} {
		if _, err := tx.Exec("DELETE FROM "+table+" WHERE email=?", email); err != nil {
			return err
		}
	}
	if _, err := tx.Exec("UPDATE pending_email_registrations SET status='account_expired',password_hash='',verification_hash='',registration_data=JSON_OBJECT() WHERE user_id=?", owner); err != nil {
		return err
	}
	result, err := tx.Exec("DELETE FROM users WHERE id=? AND status='pending' AND role='user'", owner)
	if err != nil {
		return err
	}
	if affected, err := result.RowsAffected(); err != nil {
		return err
	} else if affected != 1 {
		return fmt.Errorf("unpaid account deletion lost ownership")
	}
	return nil
}

func expirySnapshot(tx *sqlx.Tx, query string, owner uint64) (string, error) {
	rows, err := tx.Queryx(query+" FOR UPDATE", owner)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	result := []map[string]interface{}{}
	for rows.Next() {
		row := map[string]interface{}{}
		if err := rows.MapScan(row); err != nil {
			return "", err
		}
		for key, value := range row {
			if bytes, ok := value.([]byte); ok {
				row[key] = string(bytes)
			}
		}
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	encoded, err := json.Marshal(result)
	return string(encoded), err
}
