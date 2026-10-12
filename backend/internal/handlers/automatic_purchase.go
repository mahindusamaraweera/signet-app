package handlers

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"math/big"
	"signet-backend/internal/app"
	"signet-backend/internal/blockchain"
	"signet-backend/internal/wallet"
	"time"
)

var errPurchasePlacementRequired = errors.New("a valid placement is required before purchase funding")

// Retry the same durable intent; the financial service alone reserves and swaps funds.
func StartAutomaticPurchases(d *app.Deps) {
	go func() {
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for {
			var ids []uint64
			if err := d.DB.Select(&ids, "SELECT id FROM package_purchase_intents WHERE status NOT IN ('completed','expired') ORDER BY COALESCE(last_checked_at,created_at),id LIMIT 50"); err != nil {
				log.Printf("purchase intents: %v", err)
			} else {
				for _, id := range ids {
					if err := retryAutomaticPurchase(d, id, 0); err != nil {
						log.Printf("purchase intent %d: %v", id, err)
					}
				}
			}
			<-ticker.C
		}
	}()
}

// Both the worker and the authenticated retry use the same audited,
// idempotent settlement flow. expectedUserID is zero only for the worker.
func retryAutomaticPurchase(d *app.Deps, id, expectedUserID uint64) error {
	if _, err := d.DB.Exec("UPDATE package_purchase_intents SET last_checked_at=NOW(),last_attempt_at=NOW() WHERE id=? AND status NOT IN ('completed','expired')", id); err != nil {
		return err
	}
	err := syncAutomaticPurchaseForUser(d, id, expectedUserID)
	var code interface{}
	if err != nil {
		code = "activation_sync_unavailable"
		if errors.Is(err, errPurchasePlacementRequired) {
			code = "placement_required"
		}
	}
	_, recordErr := d.DB.Exec("UPDATE package_purchase_intents SET last_sync_error=? WHERE id=? AND status NOT IN ('completed','expired')", code, id)
	return errors.Join(err, recordErr)
}

func syncAutomaticPurchase(d *app.Deps, id uint64) error {
	return syncAutomaticPurchaseForUser(d, id, 0)
}

func syncAutomaticPurchaseForUser(d *app.Deps, id, expectedUserID uint64) error {
	var userID uint64
	if err := d.DB.Get(&userID, "SELECT user_id FROM package_purchase_intents WHERE id=?", id); err != nil {
		return err
	}
	if expectedUserID != 0 && userID != expectedUserID {
		return fmt.Errorf("%w: intent %d belongs to user %d, not %d", blockchain.ErrUnavailable, id, userID, expectedUserID)
	}
	tx, err := d.DB.Beginx()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var userStatus string
	if err := tx.Get(&userStatus, "SELECT status FROM users WHERE id=? FOR UPDATE", userID); err != nil {
		return err
	}
	var intent blockchain.PurchaseIntent
	if err := tx.Get(&intent, `SELECT id,user_id,user_package_id,package_price,purchase_kind,status,COALESCE(bonus_percent,0) AS bonus_percent,COALESCE(CAST(service_fee_usdt AS CHAR),'') AS service_fee_usdt,COALESCE(CAST(required_usdt AS CHAR),'') AS required_usdt,commission_user_id,commission_percent,(SELECT CAST(package AS UNSIGNED) FROM user_packages WHERE id=user_package_id) AS package_id FROM package_purchase_intents WHERE id=? AND status NOT IN ('completed','expired') FOR UPDATE`, id); err == sql.ErrNoRows {
		return nil
	} else if err != nil {
		return err
	}
	// The initial lookup was not locked. Do not authorize a remote debit for an
	// intent whose owner changed, or whose linked package belongs to another user.
	if intent.UserID != userID {
		return fmt.Errorf("%w: intent %d owner changed (%d -> %d)", blockchain.ErrUnavailable, id, userID, intent.UserID)
	}
	if intent.Status == "awaiting_reopening" {
		// Only the durable replacement coordinator can unlock funding.
		return nil
	}
	var packageOwner uint64
	if err := tx.Get(&packageOwner, "SELECT user_id FROM user_packages WHERE id=? FOR UPDATE", intent.UserPackageID); err != nil {
		return err
	}
	if packageOwner != userID {
		return fmt.Errorf("%w: intent %d: user_package %d belongs to user %d, not %d", blockchain.ErrUnavailable, id, intent.UserPackageID, packageOwner, userID)
	}
	if intent.Kind != "topup" && intent.Status == "awaiting_deposit" {
		var placementID uint64
		if err := tx.Get(&placementID, "SELECT id FROM user_parents WHERE user_id=? AND node IN ('deactive','active') ORDER BY id LIMIT 1 FOR UPDATE", userID); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return errPurchasePlacementRequired
			}
			return err
		}
	}
	if intent.Kind == "topup" && !d.Cfg.BlockchainTopupEnabled {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	result, err := d.Blockchain.SyncPurchase(ctx, intent)
	if err != nil {
		return err
	}
	if result.Status == "completed" {
		if intent.Kind == "topup" {
			if err := completeBlockchainTopup(tx, intent); err != nil {
				return err
			}
		} else {
			var pkg activatingPackage
			if err := tx.Get(&pkg, `SELECT up.id,up.user_id,up.package,up.status,up.sale,up.earn,p.price,p.commission FROM user_packages up JOIN packages p ON p.id=CAST(up.package AS UNSIGNED) WHERE up.id=? FOR UPDATE`, intent.UserPackageID); err != nil {
				return err
			}
			if pkg.Status != "pending" {
				return fmt.Errorf("intent %d: service completed the purchase but user_package %d is %q, not pending — needs manual reconciliation: %w", id, intent.UserPackageID, pkg.Status, sql.ErrNoRows)
			}
			price, ok := new(big.Rat).SetString(intent.Price)
			if !ok || !price.IsInt() || !price.Num().IsInt64() {
				return fmt.Errorf("%w: intent %d: package price %q is not a whole number", blockchain.ErrUnavailable, id, intent.Price)
			}
			pkg.PackagePrice = price.Num().Int64()
			// Reuse commission calculations without the legacy sponsor funding/token burn.
			if intent.CommissionUserID == 1 {
				err = processPackageActivationCompany(tx, &pkg)
			} else {
				// Coordinate automatic activations sharing this beneficiary. Existing
				// wallet writes also serialize on its row; no cap check is performed
				// through another DB connection while this transaction holds locks.
				var beneficiaryID uint64
				if err := tx.Get(&beneficiaryID, "SELECT id FROM users WHERE id=? FOR UPDATE", intent.CommissionUserID); err != nil {
					return err
				}
				var walletID uint64
				if err := tx.Get(&walletID, "SELECT id FROM wallets WHERE user_id=? FOR UPDATE", intent.CommissionUserID); err != nil && err != sql.ErrNoRows {
					return err
				}
				allowed, capErr := wallet.CheckWalletTx(tx, intent.CommissionUserID, intent.PackageID)
				if capErr != nil {
					return capErr
				}
				if allowed != 1 {
					// Owner policy: company receives the denied team commission, while
					// the buyer activates normally and the original recipient is notified.
					err = processPackageActivation(tx, &pkg, 1, 0, intent.CommissionPercent, activationCommissionContext{IntentID: intent.ID})
					if err == nil {
						_, err = tx.Exec(`INSERT INTO income_cap_diversions (user_id,purchase_intent_id,commission_usd) VALUES (?,?,?)`, intent.CommissionUserID, intent.ID, ftoaF(float64(pkg.PackagePrice)*(float64(intent.CommissionPercent)/100)))
					}
				} else {
					err = processPackageActivation(tx, &pkg, intent.CommissionUserID, 0, intent.CommissionPercent, activationCommissionContext{IntentID: intent.ID})
				}
			}
			if err != nil {
				return err
			}
		}
	}
	if result.Status == "awaiting_commission_settlement" && intent.Kind == "topup" {
		if err := planTopupCommissions(tx, intent); err != nil {
			return err
		}
	}
	_, err = tx.Exec(`UPDATE package_purchase_intents SET status=?,available_usdt=?,remaining_usdt=?,deposit_seen=GREATEST(deposit_seen,?),remote_order_id=?,last_checked_at=NOW() WHERE id=?`, result.Status, result.Available, result.Remaining, *result.DepositSeen, result.OrderID, id)
	if err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	if result.Status == "awaiting_commission_settlement" && intent.Kind == "topup" {
		var plans []topupCommissionPlan
		if err := d.DB.Select(&plans, "SELECT id,original_user_id,beneficiary_user_id,commission_kind,commission_usd FROM blockchain_topup_commission_plans WHERE purchase_intent_id=? ORDER BY id", intent.ID); err != nil {
			return err
		}
		rows := make([]blockchain.TopupCommission, 0, len(plans))
		for _, p := range plans {
			rows = append(rows, blockchain.TopupCommission{ID: p.ID, Recipient: p.Recipient, Kind: p.Kind, USD: p.USD})
		}
		// Commit the immutable source plan before an ambiguous remote acceptance.
		return d.Blockchain.SealTopupManifest(ctx, intent, rows)
	}
	return nil
}
