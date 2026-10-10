package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"signet-backend/internal/blockchain"
	"signet-backend/internal/config"
	"signet-backend/internal/db"
	"time"
)

func main() {
	apply := flag.Bool("apply", false, "provision wallets; default only reviews source user IDs")
	after := flag.Uint64("after-user-id", 0, "exclusive resume cursor")
	limit := flag.Int("limit", 100, "maximum users in this run, 1..1000")
	chainID := flag.Uint64("chain-id", 56, "expected BSC network: 56 mainnet or 97 testnet")
	flag.Parse()
	if *limit < 1 || *limit > 1000 || (*chainID != 56 && *chainID != 97) || flag.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "invalid arguments")
		os.Exit(1)
	}
	cfg := config.Load()
	var gateway *blockchain.Gateway
	if *apply {
		if !cfg.BlockchainEnabled {
			fmt.Fprintln(os.Stderr, "apply requires approved BLOCKCHAIN_ENABLED configuration")
			os.Exit(1)
		}
		var err error
		gateway, err = blockchain.New(cfg.BlockchainServiceURL, cfg.BlockchainServiceToken)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
	conn, err := db.Connect(cfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, "database connection failed")
		os.Exit(1)
	}
	defer conn.Close()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	queryCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	var ids []uint64
	err = conn.SelectContext(queryCtx, &ids, "SELECT id FROM users WHERE id>? ORDER BY id LIMIT ?", *after, *limit)
	cancel()
	if err != nil {
		fmt.Fprintln(os.Stderr, "user inventory query failed")
		os.Exit(1)
	}
	var audit blockchain.WalletAuditRecorder
	if *apply {
		audit = blockchain.SQLWalletAuditRecorder{DB: conn.DB}
	}
	cursor, err := blockchain.BackfillWalletsAudited(ctx, ids, *apply, *chainID, gateway, audit, os.Stdout)
	if cursor == 0 {
		cursor = *after
	}
	fmt.Printf("resume_after_user_id=%d apply=%t selected=%d\n", cursor, *apply, len(ids))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
