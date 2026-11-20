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
	execute := flag.Bool("execute", false, "hold fixed account 486 and package 521; no financial transfers")
	reason := flag.String("reason", "", "operator reason")
	completion := flag.String("finalize-proof", "", "signed native recovery proof; reverse source accounting once, leave buyer inactive")
	flag.Parse()
	c := config.Load()
	if c.AppEnv != "production" {
		fmt.Fprintln(os.Stderr, "production only")
		os.Exit(1)
	}
	d, err := db.Connect(c)
	if err == nil {
		defer d.Close()
		var result map[string]interface{}
		if *completion != "" {
			raw, e := os.ReadFile(*completion)
			if e != nil {
				err = e
			} else {
				result, err = handlers.Finalize486(d, raw, c.BlockchainServiceToken, time.Now().UTC())
			}
		} else {
			result, err = handlers.Hold486(d, *execute, *reason, c.BlockchainServiceToken, time.Now().UTC())
		}
		if err == nil {
			err = json.NewEncoder(os.Stdout).Encode(result)
		}
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
