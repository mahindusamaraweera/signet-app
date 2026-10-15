package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"signet-backend/internal/app"
	"signet-backend/internal/config"
	"signet-backend/internal/db"
	"signet-backend/internal/handlers"
	"strconv"
	"strings"
)

func main() {
	users := flag.String("users", "", "comma-separated production users; discovers each pending purchase and sole eligible deposit")
	var e handlers.CustodyRecoveryEvidence
	flag.Uint64Var(&e.UserID, "user", 0, "production user ID")
	flag.Uint64Var(&e.IntentID, "intent", 0, "production purchase ID")
	flag.Uint64Var(&e.OldCustodyUserID, "old-custody-user", 0, "expected conflicting custody user")
	flag.Uint64Var(&e.OldOrderID, "old-order", 0, "expected completed test custody order")
	flag.StringVar(&e.DepositHash, "deposit-tx", "", "production deposit hash")
	flag.StringVar(&e.DepositAmount, "deposit-amount", "", "exact verified deposit amount")
	flag.StringVar(&e.OldAddress, "old-address", "", "address that received the deposit")
	flag.Parse()
	cfg := config.Load()
	conn, err := db.Connect(cfg)
	if err != nil {
		fail(err)
	}
	defer conn.Close()
	if *users != "" {
		if e.UserID != 0 || e.IntentID != 0 {
			fail(fmt.Errorf("use --users or --user/--intent"))
		}
		proofs := make([]json.RawMessage, 0)
		skipped := make(map[string]string)
		seen := make(map[uint64]bool)
		for _, text := range strings.Split(*users, ",") {
			id, err := strconv.ParseUint(strings.TrimSpace(text), 10, 64)
			if err != nil || id == 0 || seen[id] {
				fail(fmt.Errorf("invalid or duplicate production user ID"))
			}
			seen[id] = true
			var ids []uint64
			if err := conn.Select(&ids, "SELECT id FROM package_purchase_intents WHERE user_id=? AND purchase_kind='first' AND status='awaiting_deposit' ORDER BY id", id); err != nil {
				fail(err)
			}
			if len(ids) != 1 {
				skipped[text] = "expected exactly one pending first purchase; no recovery proof issued"
				continue
			}
			proof, err := handlers.CustodyRecoveryProof(&app.Deps{DB: conn, Cfg: cfg}, handlers.CustodyRecoveryEvidence{UserID: id, IntentID: ids[0], DiscoverDeposit: true})
			if err != nil {
				skipped[text] = err.Error()
				continue
			}
			proofs = append(proofs, json.RawMessage(proof))
		}
		out, err := json.MarshalIndent(map[string]interface{}{"version": "custody-recovery-batch-v1", "proofs": proofs, "skipped": skipped}, "", "  ")
		if err != nil {
			fail(err)
		}
		fmt.Println(string(out))
		return
	}
	proof, err := handlers.CustodyRecoveryProof(&app.Deps{DB: conn, Cfg: cfg}, e)
	if err != nil {
		fail(err)
	}
	fmt.Println(string(proof))
}
func fail(e error) { fmt.Fprintln(os.Stderr, e); os.Exit(1) }
