package handlers

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"signet-backend/internal/app"
	"time"
)

// LegacyWalletProof is read-only and includes only confirmed production commissions.
// Matching transaction hashes and amounts on both systems establish the wallet owner.
func LegacyWalletProof(d *app.Deps) ([]byte, error) {
	if d.Cfg.AppEnv != "production" || len(d.Cfg.BlockchainServiceToken) < 32 {
		return nil, fmt.Errorf("production environment and integration credential required")
	}
	var rows []struct {
		Legacy string `db:"legacy" json:"legacy"`
		UUID   string `db:"uuid" json:"uuid"`
		Outbox string `db:"outbox" json:"outbox"`
		Kind   string `db:"kind" json:"kind"`
		USD    string `db:"usd" json:"usd"`
		Signet string `db:"signet" json:"signet"`
		Tx     string `db:"tx" json:"tx"`
	}
	err := d.DB.Select(&rows, `SELECT CAST(u.id AS CHAR) AS legacy,u.uuid,CAST(c.id AS CHAR) AS outbox,c.commission_kind AS kind,CAST(c.commission_usd AS CHAR) AS usd,CAST(c.signet_amount AS CHAR) AS signet,c.transfer_tx_hash AS tx FROM users u JOIN blockchain_commission_outbox c ON c.beneficiary_user_id=u.id WHERE u.status='active' AND c.status='confirmed' AND c.signet_amount>0 AND c.transfer_tx_hash IS NOT NULL ORDER BY u.id,c.id LIMIT 10001`)
	if err != nil {
		return nil, err
	}
	if len(rows) > 10000 {
		return nil, fmt.Errorf("proof exceeds 10000 commission limit; explicit review required")
	}
	var commissions interface{} = rows
	if rows == nil {
		commissions = []interface{}{}
	}
	payload, err := json.Marshal(map[string]interface{}{"version": "legacy-wallet-commission-v1", "environment": "production", "issued_at": time.Now().Unix(), "expires_at": time.Now().Add(30 * time.Minute).Unix(), "commissions": commissions})
	if err != nil {
		return nil, err
	}
	encoded := base64.StdEncoding.EncodeToString(payload)
	mac := hmac.New(sha256.New, []byte(d.Cfg.BlockchainServiceToken))
	mac.Write([]byte(encoded))
	return json.Marshal(map[string]string{"payload": encoded, "signature": hex.EncodeToString(mac.Sum(nil))})
}
