package handlers

import (
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"database/sql/driver"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"github.com/jmoiron/sqlx"
	"signet-backend/internal/app"
	"signet-backend/internal/config"
	"strings"
	"testing"
)

func TestCustodyProofIsReadOnlySignedAndRejectsPayoutEvidence(t *testing.T) {
	for _, count := range []int64{0, 1} {
		t.Run(string(rune('0'+count)), func(t *testing.T) {
			fixture := &commissionSQLFixture{steps: []commissionSQLStep{
				{contains: "SELECT id,status,uuid FROM users", columns: []string{"id", "status", "uuid"}, values: []driver.Value{int64(461), "pending", "01a13b42-28d0-7000-8000-0000000001cd"}},
				{contains: "SELECT p.id,p.user_id", columns: []string{"id", "user_id", "user_package_id", "purchase_kind", "status", "package_price", "package_id", "current_package_id", "service_fee_usdt", "required_usdt", "bonus_percent", "package_owner", "package_status"}, values: []driver.Value{int64(2), int64(461), int64(496), "first", "awaiting_deposit", "1000", int64(3), int64(3), "130", "1130", int64(400), int64(461), "pending"}},
				{contains: "SELECT COUNT(*) FROM blockchain_commission_outbox", columns: []string{"count"}, values: []driver.Value{count}},
			}}
			if count == 0 {
				fixture.steps = append(fixture.steps, commissionSQLStep{contains: "SELECT COUNT(*) FROM package_purchase_intents", columns: []string{"count"}, values: []driver.Value{int64(0)}})
			}
			db := sqlx.NewDb(sql.OpenDB(fixture), "commission-fixture")
			defer db.Close()
			key := strings.Repeat("k", 32)
			result, err := CustodyRecoveryProof(&app.Deps{DB: db, Cfg: &config.Config{AppEnv: "production", BlockchainServiceToken: key, DBDatabase: "production"}}, CustodyRecoveryEvidence{UserID: 461, IntentID: 2, OldCustodyUserID: 1077, OldOrderID: 2, DepositHash: "0x" + strings.Repeat("b", 64), DepositAmount: "1130.99", OldAddress: "0x" + strings.Repeat("a", 40)})
			if count > 0 {
				if err == nil {
					t.Fatal("proof authorized existing payouts")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			var envelope map[string]string
			if err := json.Unmarshal(result, &envelope); err != nil {
				t.Fatal(err)
			}
			raw, err := base64.StdEncoding.DecodeString(envelope["payload"])
			if err != nil {
				t.Fatal(err)
			}
			mac := hmac.New(sha256.New, []byte(key))
			mac.Write(raw)
			if !hmac.Equal([]byte(envelope["signature"]), []byte(hex.EncodeToString(mac.Sum(nil)))) {
				t.Fatal("invalid proof signature")
			}
			if !strings.Contains(string(raw), `"package_id":3`) || !strings.Contains(string(raw), `"commission_outbox_count":0`) {
				t.Fatal("proof lost purchase terms")
			}
			if fixture.committed || !fixture.rolledBack || len(fixture.steps) != 0 {
				t.Fatal("audit mutated database or skipped checks")
			}
		})
	}
}

func TestCustodyProofRejectsNonProductionBeforeDatabaseAccess(t *testing.T) {
	if _, err := CustodyRecoveryProof(&app.Deps{Cfg: &config.Config{AppEnv: "staging", BlockchainServiceToken: strings.Repeat("k", 32)}}, CustodyRecoveryEvidence{}); err == nil {
		t.Fatal("staging proof accepted")
	}
}
