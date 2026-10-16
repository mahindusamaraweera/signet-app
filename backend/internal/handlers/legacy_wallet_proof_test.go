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

func TestLegacyWalletProof(t *testing.T) {
	f := &commissionSQLFixture{steps: []commissionSQLStep{{contains: "c.status='confirmed'", columns: []string{"legacy", "uuid", "outbox", "kind", "usd", "signet", "tx"}, values: []driver.Value{"134", "0198c5f3-72f8-7000-8000-000000000086", "3", "direct_team", "400", "22588.157335967066369968", "0x" + strings.Repeat("b", 64)}}}}
	conn := sql.OpenDB(f)
	defer conn.Close()
	key := strings.Repeat("k", 64)
	data, err := LegacyWalletProof(&app.Deps{DB: sqlx.NewDb(conn, "fixture"), Cfg: &config.Config{AppEnv: "production", BlockchainServiceToken: key}})
	if err != nil {
		t.Fatal(err)
	}
	var envelope map[string]string
	if err = json.Unmarshal(data, &envelope); err != nil {
		t.Fatal(err)
	}
	mac := hmac.New(sha256.New, []byte(key))
	mac.Write([]byte(envelope["payload"]))
	if envelope["signature"] != hex.EncodeToString(mac.Sum(nil)) {
		t.Fatal("wrong signature")
	}
	decoded, err := base64.StdEncoding.DecodeString(envelope["payload"])
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(decoded), "22588.157335967066369968") {
		t.Fatal("decimal precision loss")
	}
	if len(f.steps) != 0 {
		t.Fatal("unexpected mutation or remaining query")
	}
}
func TestLegacyWalletProofRejectsNonproduction(t *testing.T) {
	if _, err := LegacyWalletProof(&app.Deps{Cfg: &config.Config{AppEnv: "local", BlockchainServiceToken: strings.Repeat("k", 64)}}); err == nil {
		t.Fatal("nonproduction must fail")
	}
}
