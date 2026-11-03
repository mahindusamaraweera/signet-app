package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"signet-backend/internal/config"
	"signet-backend/internal/db"
	"signet-backend/internal/handlers"
	"time"
)

func main() {
	execute := flag.Bool("execute", false, "prepare fixed missing $200 correction; no transfer")
	reason := flag.String("reason", "", "operator approval reason")
	completion := flag.String("confirm-proof", "", "custody signed completion proof; no payment")
	flag.Parse()
	cfg := config.Load()
	if cfg.AppEnv != "production" {
		fmt.Fprintln(os.Stderr, "production only")
		os.Exit(1)
	}
	conn, err := db.Connect(cfg)
	if err == nil {
		defer conn.Close()
		if *completion != "" {
			raw, e := os.ReadFile(*completion)
			if e == nil {
				e = handlers.ConfirmGratitude470(conn, raw, cfg.BlockchainServiceToken, time.Now().UTC())
			}
			if e != nil {
				fmt.Fprintln(os.Stderr, e)
				os.Exit(1)
			}
			fmt.Println(`{"status":"confirmed","funds_sent":false}`)
			return
		}
		var result map[string]interface{}
		result, err = handlers.RecoverGratitude470(conn, *execute, *reason, cfg.BlockchainServiceToken, time.Now().UTC())
		if err == nil {
			err = json.NewEncoder(os.Stdout).Encode(result)
		}
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
