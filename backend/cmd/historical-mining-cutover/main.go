package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"signet-backend/internal/config"
	"signet-backend/internal/db"
	"signet-backend/internal/historicalmining"
	"time"
)

func run() error {
	batch := flag.String("batch", historicalmining.Batch, "reviewed fixed batch: historical-mining-435-439-448-v1 or historical-option-one-nine-v1")
	apply := flag.Bool("execute", false, "archive and stop only the selected reviewed batch; no transfers")
	paused := flag.Bool("services-paused", false, "operator stopped ALL source API/scheduler instances and drained in-flight webhooks")
	export := flag.Bool("export-proof", false, "verify frozen records and export a fresh signed proof, without rewriting mining")
	flag.Parse()
	if flag.NArg() != 0 || (*apply && *export) || (*apply && !*paused) {
		return fmt.Errorf("execution requires --services-paused; --execute and --export-proof are separate modes")
	}
	cfg := config.Load()
	if cfg.AppEnv != "production" || len(cfg.BlockchainServiceToken) < 32 {
		return fmt.Errorf("production environment and integration credential required")
	}
	conn, err := db.Connect(cfg)
	if err != nil {
		return fmt.Errorf("database connection failed")
	}
	defer conn.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	rows, err := historicalmining.RunBatch(ctx, conn, *apply, *export, time.Now().UTC(), *batch)
	if err != nil {
		return err
	}
	if *apply || *export {
		proof, err := historicalmining.ProofBatch(rows, cfg.BlockchainServiceToken, time.Now().UTC(), *batch)
		if err != nil {
			return err
		}
		fmt.Println(string(proof))
		return nil
	}
	return json.NewEncoder(os.Stdout).Encode(map[string]interface{}{"mode": "dry_run", "snapshots": rows, "funds_sent": false, "mining_changed": false})
}
func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
