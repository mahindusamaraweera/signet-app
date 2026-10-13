package handlers

import (
	"database/sql"
	"fmt"
	"github.com/jmoiron/sqlx"
	"math/big"
	"signet-backend/internal/blockchain"
	"signet-backend/internal/models"
	"signet-backend/internal/wallet"
)

type topupCommissionPlan struct {
	ID        uint64 `db:"id"`
	Original  uint64 `db:"original_user_id"`
	Recipient uint64 `db:"beneficiary_user_id"`
	Kind      string `db:"commission_kind"`
	USD       string `db:"commission_usd"`
}

func planTopupCommissions(tx *sqlx.Tx, intent blockchain.PurchaseIntent) error {
	var planned bool
	if err := tx.Get(&planned, "SELECT commission_plan_created FROM package_purchase_intents WHERE id=?", intent.ID); err != nil {
		return err
	}
	if planned {
		return nil
	}
	if intent.CommissionUserID == 1 {
		_, err := tx.Exec("UPDATE package_purchase_intents SET commission_plan_created=TRUE WHERE id=?", intent.ID)
		return err
	}
	price, ok := new(big.Rat).SetString(intent.Price)
	if !ok || !price.IsInt() || price.Sign() <= 0 {
		return fmt.Errorf("invalid top-up price")
	}
	add := func(recipient uint64, percent int64, kind string) error {
		if percent == 0 {
			return nil
		}
		if recipient == 0 || percent < 0 || percent > 100 {
			return fmt.Errorf("invalid commission role")
		}
		amount := new(big.Rat).Mul(price, new(big.Rat).SetFrac(big.NewInt(percent), big.NewInt(100)))
		usd := amount.FloatString(18)
		original := recipient
		if recipient != 1 {
			var id uint64
			if err := tx.Get(&id, "SELECT id FROM users WHERE id=? FOR UPDATE", recipient); err != nil {
				return err
			}
			var balance sql.NullFloat64
			err := tx.Get(&balance, "SELECT balance FROM wallets WHERE user_id=? FOR UPDATE", recipient)
			if err != nil && err != sql.ErrNoRows {
				return err
			}
			cap, err := activePackagesCapValueTx(tx, recipient)
			if err != nil {
				return err
			}
			var reserved float64
			if err := tx.Get(&reserved, "SELECT COALESCE(SUM(commission_usd),0) FROM blockchain_topup_commission_plans WHERE beneficiary_user_id=? AND status='reserved'", recipient); err != nil {
				return err
			}
			value, _ := amount.Float64()
			if !wallet.CommissionFitsCap(balance.Float64+reserved, value, cap) {
				recipient = 1
			}
		}
		_, err := tx.Exec(`INSERT INTO blockchain_topup_commission_plans (purchase_intent_id,original_user_id,beneficiary_user_id,commission_kind,commission_usd) VALUES (?,?,?,?,?)`, intent.ID, original, recipient, kind, usd)
		if err != nil {
			return err
		}
		if original != recipient {
			_, err = tx.Exec(`INSERT INTO income_cap_diversions (user_id,purchase_intent_id,commission_usd,commission_kind) VALUES (?,?,?,?)`, original, intent.ID, usd, kind)
		}
		return err
	}
	var node struct {
		Virtual uint64 `db:"virtual_id"`
		Node    string `db:"node"`
	}
	if err := tx.Get(&node, "SELECT virtual_id,node FROM user_parents WHERE user_id=? LIMIT 1", intent.UserID); err != nil {
		return err
	}
	var firstID uint64
	err := tx.Get(&firstID, "SELECT id FROM user_packages WHERE user_id=? AND status='active' ORDER BY id LIMIT 1", intent.UserID)
	if err != nil {
		return err
	}
	var spillover uint64
	err = tx.Get(&spillover, "SELECT current_user_id FROM super_parent_logs WHERE user_package=? LIMIT 1", firstID)
	if err != nil && err != sql.ErrNoRows {
		return err
	}
	if err == nil {
		if err = add(spillover, 20, "spillover"); err != nil {
			return err
		}
	}
	if node.Node == "gratitude" {
		if err = add(node.Virtual, 20, "gratitude"); err != nil {
			return err
		}
	}
	if err = add(intent.CommissionUserID, intent.CommissionPercent, "direct_team"); err != nil {
		return err
	}
	var owner models.User
	if err = tx.Get(&owner, "SELECT * FROM users WHERE id=?", intent.UserID); err != nil {
		return err
	}
	for _, role := range []struct {
		Code models.NullString
		Kind string
	}{{owner.LeaderCode, "leadership"}, {owner.ExecutiveCode, "executive"}} {
		if role.Code.Valid {
			if id, ok := parseUintParam(role.Code.String); ok && id != 0 {
				if err = add(id, 5, role.Kind); err != nil {
					return err
				}
			}
		}
	}
	_, err = tx.Exec("UPDATE package_purchase_intents SET commission_plan_created=TRUE WHERE id=?", intent.ID)
	return err
}

func completeBlockchainTopup(tx *sqlx.Tx, intent blockchain.PurchaseIntent) error {
	var planned bool
	if err := tx.Get(&planned, "SELECT commission_plan_created FROM package_purchase_intents WHERE id=?", intent.ID); err != nil {
		return err
	}
	if !planned {
		return fmt.Errorf("top-up commission plan is missing")
	}
	var plans []topupCommissionPlan
	if err := tx.Select(&plans, "SELECT id,original_user_id,beneficiary_user_id,commission_kind,commission_usd FROM blockchain_topup_commission_plans WHERE purchase_intent_id=? AND status='reserved' ORDER BY id FOR UPDATE", intent.ID); err != nil {
		return err
	}
	for _, plan := range plans {
		value, ok := new(big.Rat).SetString(plan.USD)
		if !ok || value.Sign() <= 0 {
			return fmt.Errorf("invalid reserved commission")
		}
		amount, _ := value.Float64()
		if plan.Recipient == 1 {
			if err := creditDivertedCommissionTx(tx, amount); err != nil {
				return err
			}
		} else {
			if err := creditWalletTx(tx, plan.Recipient, amount, "Top-up team commission"); err != nil {
				return err
			}
		}
		if _, err := tx.Exec("UPDATE blockchain_topup_commission_plans SET status='credited' WHERE id=?", plan.ID); err != nil {
			return err
		}
	}
	result, err := tx.Exec("UPDATE user_packages SET status='active',activated_at=?,updated_at=NOW() WHERE id=? AND user_id=? AND status='pending' AND sale='other'", colomboWallClock(), intent.UserPackageID, intent.UserID)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil || count != 1 {
		return fmt.Errorf("top-up package state changed")
	}
	if !excludedPoolUserIDs[intent.UserID] {
		price, ok := new(big.Rat).SetString(intent.Price)
		if !ok {
			return fmt.Errorf("invalid top-up price")
		}
		pool := new(big.Rat).Mul(price, big.NewRat(5, 100)).FloatString(18)
		_, err = tx.Exec(`INSERT INTO package_pools (user_id,package_id,pool_amount,created_at,updated_at) VALUES (?,?,?,NOW(),NOW())`, intent.UserID, intent.PackageID, pool)
	}
	return err
}
