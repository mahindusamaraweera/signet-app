// Read-only historical evidence export. This command never creates custody
// allocations and never interprets mining_token as paid or released tokens.
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/jmoiron/sqlx"
	"signet-backend/internal/config"
	"signet-backend/internal/db"
)

type miningEvidence struct {
	ID      uint64 `db:"id" json:"source_record_id"`
	UserID  uint64 `db:"user_id" json:"source_user_id"`
	Total   string `db:"total_token" json:"recorded_total_token"`
	Accrued string `db:"mining_token" json:"recorded_mining_token"`
	Daily   string `db:"daily_mining" json:"recorded_daily_mining"`
	Status  string `db:"status" json:"status"`
	Created string `db:"created_at" json:"recorded_created_at"`
	Updated string `db:"updated_at" json:"recorded_updated_at"`
}

type packageEvidence struct {
	ID      uint64 `db:"id" json:"source_record_id"`
	UserID  uint64 `db:"user_id" json:"source_user_id"`
	Package string `db:"package" json:"package_id"`
	Price   string `db:"price" json:"current_catalog_price_usdt"`
	Earn    string `db:"earn" json:"recorded_earn"`
	Status  string `db:"status" json:"status"`
	Created string `db:"created_at" json:"recorded_created_at"`
}

func run() error {
	after := flag.Uint64("after-user-id", 0, "exclusive source user resume cursor")
	limit := flag.Int("limit", 100, "maximum users to inspect, 1..1000")
	flag.Parse()
	if *limit < 1 || *limit > 1000 || flag.NArg() != 0 {
		return fmt.Errorf("invalid arguments")
	}
	conn, err := db.Connect(config.Load())
	if err != nil {
		return fmt.Errorf("database connection failed")
	}
	defer conn.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	tx, err := conn.BeginTxx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return fmt.Errorf("read-only snapshot unavailable")
	}
	defer tx.Rollback()
	var ids []uint64
	if err := tx.SelectContext(ctx, &ids, "SELECT id FROM users WHERE id>? ORDER BY id LIMIT ?", *after, *limit); err != nil {
		return fmt.Errorf("user inventory query failed")
	}
	minings := []miningEvidence{}
	packages := []packageEvidence{}
	if len(ids) > 0 {
		query, args, err := sqlx.In(`SELECT id,user_id,CAST(total_token AS CHAR) AS total_token,CAST(mining_token AS CHAR) AS mining_token,CAST(daily_mining AS CHAR) AS daily_mining,COALESCE(status,'') AS status,COALESCE(CAST(created_at AS CHAR),'') AS created_at,COALESCE(CAST(updated_at AS CHAR),'') AS updated_at FROM user_minings WHERE user_id IN (?) ORDER BY user_id,id`, ids)
		if err != nil {
			return err
		}
		if err := tx.SelectContext(ctx, &minings, tx.Rebind(query), args...); err != nil {
			return fmt.Errorf("mining evidence query failed")
		}
		query, args, err = sqlx.In(`SELECT up.id,up.user_id,up.package,COALESCE(CAST(p.price AS CHAR),'') AS price,up.earn,up.status,COALESCE(CAST(up.created_at AS CHAR),'') AS created_at FROM user_packages up LEFT JOIN packages p ON CAST(p.id AS CHAR)=up.package WHERE up.user_id IN (?) ORDER BY up.user_id,up.id`, ids)
		if err != nil {
			return err
		}
		if err := tx.SelectContext(ctx, &packages, tx.Rebind(query), args...); err != nil {
			return fmt.Errorf("package evidence query failed")
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("snapshot completion failed")
	}
	cursor := *after
	if len(ids) > 0 {
		cursor = ids[len(ids)-1]
	}
	return json.NewEncoder(os.Stdout).Encode(map[string]interface{}{
		"version": "historical-evidence-v1", "source": "signet-app", "exported_at": time.Now().UTC().Format(time.RFC3339),
		"user_ids": ids, "resume_after_user_id": cursor, "mining_records": minings, "package_records": packages,
		"import_ready": false, "requires_reconciliation": "Verify original purchase dates, allocation, released and externally paid amounts, lock terms and admin overrides. Mining accrual is not proof of payment. No new deposits/swaps/bonuses/commissions may be fabricated.",
	})
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
