// Package historicalmining freezes only explicitly selected, operator-reviewed batches.
// It archives incorrect paper accrual; it never transfers or credits tokens.
package historicalmining

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
)

const Batch = "historical-mining-435-439-448-v1"
const Domain = "historical-mining-cutover-v1|"

type Profile struct {
	UserID        uint64 `json:"user_id"`
	UUID          string `json:"uuid"`
	Email         string `json:"email"`
	PackageRecord uint64 `json:"source_package_id"`
	PackageID     string `json:"package_id"`
	Principal     string `json:"package_usdt"`
	Amount        string `json:"total_signet"`
	MiningID      uint64 `json:"mining_id"`
	OldTotal      string `json:"old_total"`
	Activated     string `json:"activated_at_colombo"`
}

const OptionOneBatch = "historical-option-one-nine-v1"

func OptionOneProfiles() []Profile {
	return []Profile{
		{377, "019f2876-4760-7000-8000-000000000179", "bestlifesanjeewa@gmail.com", 397, "11", "500", "250000", 176, "1000000", "2026-07-04 01:55:25"},
		{378, "019f287f-fbc0-7000-8000-00000000017a", "piyalsoysa@gmail.com", 398, "11", "500", "250000", 174, "1000000", "2026-07-04 02:09:08"},
		{379, "019f288f-68f8-7000-8000-00000000017b", "wasanthapeiris79@gmail.com", 399, "11", "500", "250000", 175, "1000000", "2026-07-04 02:49:15"},
		{383, "019f5ef7-c908-7000-8000-00000000017f", "sudeshpushpakumara22@gmail.com", 409, "11", "500", "250000", 178, "1000000", "2026-07-14 16:16:43"},
		{386, "019f6fcc-1ea0-7000-8000-000000000182", "prasannakumara215@gmail.com", 412, "3", "1000", "750000", 181, "3000000", "2026-07-18 01:50:29"},
		{391, "019fb762-cf70-7000-8000-000000000187", "nissanka.bandara04@gmail.com", 420, "3", "1000", "500000", 185, "3000000", "2026-07-31 23:27:36"},
		{392, "019fb831-bc18-7000-8000-000000000188", "bmuwanwella@gmail.com", 421, "5", "5000", "6250000", 186, "25000000", "2026-07-31 23:49:27"},
		{395, "01a00f44-7db0-7000-8000-00000000018b", "077chamlimobile@gmail.com", 424, "3", "1000", "750000", 192, "3000000", "2026-08-17 22:43:12"},
		{419, "01a07e7c-3380-7000-8000-0000000001a3", "fernandojayantha377@gmail.com", 451, "3", "1000", "750000", 213, "3000000", "2026-09-08 12:23:11"},
	}
}

const OctoberElevenBatch = "historical-october-eleven-v1"

func OctoberElevenProfiles() []Profile {
	return []Profile{
		{436, "01a10b6d-7c18-7000-8000-0000000001b4", "udayanganirmw665@gmail.com", 469, "3", "1000", "500000", 235, "2000000", "2026-10-06 01:33:31"},
		{441, "01a114e3-44a8-7000-8000-0000000001b9", "dmscdissanayake1960@gmail.com", 475, "3", "1000", "500000", 236, "2000000", "2026-10-07 17:42:52"},
		{442, "01a115fe-1700-7000-8000-0000000001ba", "ishara197507@gmail.com", 476, "11", "500", "150000", 243, "600000", "2026-10-07 23:54:38"},
		{443, "01a11b24-4420-7000-8000-0000000001bb", "jiwanthapremarathna@gmail.com", 477, "3", "1000", "500000", 237, "2000000", "2026-10-09 11:33:57"},
		{444, "01a11f97-7cc0-7000-8000-0000000001bc", "chamindananayakkaraforkaliganme@gmail.com", 478, "4", "2500", "1500000", 0, "0", "2026-10-09 20:33:10"},
		{446, "01a1210b-b5d0-7000-8000-0000000001be", "gayanika85c@gmail.com", 480, "3", "1000", "500000", 0, "0", "2026-10-10 02:08:07"},
		{449, "01a1244c-5240-7000-8000-0000000001c1", "buddikaanura04@gmail.com", 485, "11", "500", "150000", 246, "600000", "2026-10-11 20:50:30"},
		{450, "01a1249f-c1a0-7000-8000-0000000001c2", "manjuladulakshi4@gmail.com", 486, "3", "1000", "500000", 247, "2000000", "2026-10-10 20:37:19"},
		{455, "01a12627-7ae0-7000-8000-0000000001c7", "Hashankavihara@gmail.com", 490, "3", "1000", "500000", 244, "2000000", "2026-10-11 12:31:50"},
		{456, "01a126a6-8a38-7000-8000-0000000001c8", "bhmadubashi81@gmail.com", 491, "3", "1000", "500000", 245, "2000000", "2026-10-11 12:30:38"},
		{457, "01a12700-3740-7000-8000-0000000001c9", "ajith_we@yahoo.com", 492, "3", "1000", "500000", 248, "2000000", "2026-10-11 05:27:17"},
	}
}

func Profiles() []Profile {
	return []Profile{
		{435, "01a10605-3130-7000-8000-0000000001b3", "kushanmadushanka000@gmail.com", 468, "3", "1000", "500000", 230, "2000000", "2026-10-04 19:51:00"},
		{439, "01a10f88-4fd0-7000-8000-0000000001b7", "fnpinsiri6@gmail.com", 473, "5", "5000", "3500000", 233, "16000000", "2026-10-06 16:56:14"},
		{448, "01a12159-6488-7000-8000-0000000001c0", "bpedirisinghe1985@gmail.com", 482, "3", "1000", "500000", 239, "2000000", "2026-10-10 03:30:07"},
	}
}

type Mining struct {
	ID              uint64 `db:"id" json:"id"`
	UserID          uint64 `db:"user_id" json:"user_id"`
	Total           string `db:"total_token" json:"total_token"`
	Accrued         string `db:"mining_token" json:"mining_token"`
	Daily           string `db:"daily_mining" json:"daily_mining"`
	Status          string `db:"status" json:"status"`
	WebhookStatus   string `db:"webhook_status" json:"webhook_status"`
	WebhookSent     string `db:"webhook_sent_at" json:"webhook_sent_at"`
	WebhookAttempts string `db:"webhook_attempts" json:"webhook_attempts"`
	NextRetry       string `db:"next_retry_at" json:"next_retry_at"`
	Created         string `db:"created_at" json:"created_at"`
	Updated         string `db:"updated_at" json:"updated_at"`
}
type Snapshot struct {
	Profile      Profile `json:"profile"`
	Before       Mining  `json:"before"`
	CutoverAt    string  `json:"cutover_at"`
	AfterTotal   string  `json:"after_total"`
	AfterAccrued string  `json:"after_accrued"`
	AfterDaily   string  `json:"after_daily"`
	AfterStatus  string  `json:"after_status"`
	AfterWebhook string  `json:"after_webhook"`
}

func digest(raw []byte) string { sum := sha256.Sum256(raw); return hex.EncodeToString(sum[:]) }

func loadOwner(ctx context.Context, tx *sqlx.Tx, p Profile) error {
	var u struct {
		Email  string `db:"email"`
		UUID   string `db:"uuid"`
		Status string `db:"status"`
	}
	if err := tx.GetContext(ctx, &u, "SELECT email,uuid,status FROM users WHERE id=? FOR UPDATE", p.UserID); err != nil {
		return err
	}
	if u.Email != p.Email || u.UUID != p.UUID || u.Status != "active" {
		return fmt.Errorf("user %d identity/status changed", p.UserID)
	}
	var up struct {
		UserID    uint64 `db:"user_id"`
		Package   string `db:"package"`
		Status    string `db:"status"`
		Sale      string `db:"sale"`
		Activated string `db:"activated"`
	}
	if err := tx.GetContext(ctx, &up, "SELECT user_id,package,status,sale,CAST(activated_at AS CHAR) AS activated FROM user_packages WHERE id=? FOR UPDATE", p.PackageRecord); err != nil {
		return err
	}
	// Profiles preserve operator-reviewed Colombo wall times for the signed proof.
	// MySQLDSN pins this connection to UTC, including TIMESTAMP-to-CHAR reads.
	expectedActivation, err := time.ParseInLocation("2006-01-02 15:04:05", p.Activated, time.FixedZone("Asia/Colombo", 19800))
	if err != nil {
		return fmt.Errorf("user %d invalid reviewed activation timestamp", p.UserID)
	}
	if up.UserID != p.UserID || up.Package != p.PackageID || up.Status != "active" || up.Sale != "first" || up.Activated != expectedActivation.UTC().Format("2006-01-02 15:04:05") {
		return fmt.Errorf("user %d package evidence changed", p.UserID)
	}
	return nil
}
func loadMining(ctx context.Context, tx *sqlx.Tx, p Profile) (Mining, error) {
	var rows []Mining
	err := tx.SelectContext(ctx, &rows, `SELECT id,user_id,CAST(total_token AS CHAR) AS total_token,CAST(mining_token AS CHAR) AS mining_token,CAST(daily_mining AS CHAR) AS daily_mining,status,webhook_status,COALESCE(CAST(webhook_sent_at AS CHAR),'') AS webhook_sent_at,CAST(webhook_attempts AS CHAR) AS webhook_attempts,COALESCE(CAST(next_retry_at AS CHAR),'') AS next_retry_at,COALESCE(CAST(created_at AS CHAR),'') AS created_at,COALESCE(CAST(updated_at AS CHAR),'') AS updated_at FROM user_minings WHERE user_id=? FOR UPDATE`, p.UserID)
	if err != nil {
		return Mining{}, err
	}
	if p.MiningID == 0 {
		if len(rows) != 0 {
			return Mining{}, fmt.Errorf("user %d expected no mining records", p.UserID)
		}
		return absentMining(p), nil
	}
	if len(rows) != 1 || rows[0].ID != p.MiningID {
		return Mining{}, fmt.Errorf("user %d mining identity changed", p.UserID)
	}
	return rows[0], nil
}
func absentMining(p Profile) Mining {
	return Mining{UserID: p.UserID, Total: "0", Accrued: "0", Daily: "0", Status: "absent", WebhookStatus: "absent", WebhookAttempts: "0"}
}
func snapshot(p Profile, m Mining, at time.Time) Snapshot {
	return Snapshot{p, m, at.UTC().Format(time.RFC3339), p.Amount, "0", "0", "inactive", "historical_frozen"}
}
func validateFrozen(p Profile, m Mining, s Snapshot) error {
	if p.MiningID == 0 {
		if s.Profile != p || s.Before != absentMining(p) || m != absentMining(p) || s.AfterTotal != p.Amount || s.AfterAccrued != "0" || s.AfterDaily != "0" || s.AfterStatus != "inactive" || s.AfterWebhook != "historical_frozen" {
			return fmt.Errorf("user %d absent mining evidence changed", p.UserID)
		}
		_, err := time.Parse(time.RFC3339, s.CutoverAt)
		return err
	}

	if s.Profile != p || s.Before.ID != p.MiningID || s.Before.UserID != p.UserID || s.Before.Total != p.OldTotal || s.AfterTotal != p.Amount || s.AfterAccrued != "0" || s.AfterDaily != "0" || s.AfterStatus != "inactive" || s.AfterWebhook != "historical_frozen" || m.Total != p.Amount || m.Accrued != "0.00000000" || m.Daily != "0" || m.Status != "inactive" || m.WebhookStatus != "historical_frozen" || m.WebhookSent != s.Before.WebhookSent || m.WebhookAttempts != s.Before.WebhookAttempts || m.NextRetry != "" {
		return fmt.Errorf("user %d frozen mining evidence changed", p.UserID)
	}
	if _, err := time.Parse(time.RFC3339, s.CutoverAt); err != nil {
		return err
	}
	return nil
}

// Run applies the complete batch atomically. exportOnly verifies an existing
// freeze without rewriting it. Caller must stop all source API/scheduler
// instances before apply so no pre-cutover webhook can remain in flight.
func Run(ctx context.Context, db *sqlx.DB, apply, exportOnly bool, at time.Time) ([]Snapshot, error) {
	return RunBatch(ctx, db, apply, exportOnly, at, Batch)
}

func profilesFor(batch string) ([]Profile, error) {
	switch batch {
	case Batch:
		return Profiles(), nil
	case OctoberElevenBatch:
		return OctoberElevenProfiles(), nil
	case OptionOneBatch:
		return OptionOneProfiles(), nil
	default:
		return nil, fmt.Errorf("unknown reviewed batch")
	}
}

func RunBatch(ctx context.Context, db *sqlx.DB, apply, exportOnly bool, at time.Time, batch string) ([]Snapshot, error) {
	profiles, err := profilesFor(batch)
	if err != nil {
		return nil, err
	}
	tx, err := db.BeginTxx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	snapshots := []Snapshot{}
	newRows := []Snapshot{}
	for _, p := range profiles {
		if err := loadOwner(ctx, tx, p); err != nil {
			return nil, err
		}
		m, err := loadMining(ctx, tx, p)
		if err != nil {
			return nil, err
		}
		var audit struct {
			Raw   string `db:"snapshot_json"`
			Hash  string `db:"snapshot_sha256"`
			Batch string `db:"batch_id"`
		}
		err = tx.GetContext(ctx, &audit, "SELECT snapshot_json,snapshot_sha256,batch_id FROM historical_mining_cutovers WHERE user_id=? FOR UPDATE", p.UserID)
		if err == nil {
			var s Snapshot
			if audit.Batch != batch || digest([]byte(audit.Raw)) != audit.Hash || json.Unmarshal([]byte(audit.Raw), &s) != nil {
				return nil, fmt.Errorf("cutover audit mismatch")
			}
			if err := validateFrozen(p, m, s); err != nil {
				return nil, err
			}
			snapshots = append(snapshots, s)
			continue
		}
		if err != sql.ErrNoRows {
			return nil, err
		}
		if exportOnly {
			return nil, fmt.Errorf("user %d mining has not been frozen", p.UserID)
		}
		if p.MiningID != 0 && (m.Total != p.OldTotal || (m.Status != "active" && m.Status != "inactive")) {
			return nil, fmt.Errorf("user %d legacy total/status requires review", p.UserID)
		}
		s := snapshot(p, m, at)
		snapshots = append(snapshots, s)
		newRows = append(newRows, s)
	}
	if !apply {
		return snapshots, nil
	}
	for _, s := range newRows {
		raw, err := json.Marshal(s)
		if err != nil {
			return nil, err
		}
		var miningID interface{}
		if s.Profile.MiningID != 0 {
			miningID = s.Profile.MiningID
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO historical_mining_cutovers (user_id,mining_id,batch_id,corrected_total,snapshot_json,snapshot_sha256,cutover_at) VALUES (?,?,?,?,?,?,?)`, s.Profile.UserID, miningID, batch, s.AfterTotal, string(raw), digest(raw), s.CutoverAt); err != nil {
			return nil, err
		}
		if s.Profile.MiningID == 0 {
			continue
		}
		result, err := tx.ExecContext(ctx, `UPDATE user_minings SET total_token=?,mining_token=0,daily_mining=0,status='inactive',webhook_status='historical_frozen',next_retry_at=NULL,updated_at=UTC_TIMESTAMP() WHERE id=? AND user_id=?`, s.AfterTotal, s.Profile.MiningID, s.Profile.UserID)
		if err != nil {
			return nil, err
		}
		n, err := result.RowsAffected()
		if err != nil || n != 1 {
			return nil, fmt.Errorf("mining freeze update failed")
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return snapshots, nil
}
func Proof(rows []Snapshot, key string, now time.Time) ([]byte, error) {
	return ProofBatch(rows, key, now, Batch)
}

func ProofBatch(rows []Snapshot, key string, now time.Time, batch string) ([]byte, error) {
	profiles, err := profilesFor(batch)
	if err != nil {
		return nil, err
	}
	if len(key) < 32 || len(rows) != len(profiles) {
		return nil, fmt.Errorf("production source signing configuration required")
	}
	for i, p := range profiles {
		m := rows[i].Before
		if p.MiningID != 0 {
			m.Total = p.Amount
			m.Accrued = "0.00000000"
			m.Daily = "0"
			m.Status = "inactive"
			m.WebhookStatus = "historical_frozen"
			m.NextRetry = ""
		}
		if err := validateFrozen(p, m, rows[i]); err != nil {
			return nil, err
		}
	}
	raw, err := json.Marshal(map[string]interface{}{"version": batch, "environment": "production", "issued_at": now.Unix(), "expires_at": now.Add(30 * time.Minute).Unix(), "snapshots": rows})
	if err != nil {
		return nil, err
	}
	encoded := base64.StdEncoding.EncodeToString(raw)
	mac := hmac.New(sha256.New, []byte(key))
	mac.Write([]byte(Domain + encoded))
	return json.Marshal(map[string]string{"payload": encoded, "signature": hex.EncodeToString(mac.Sum(nil))})
}
