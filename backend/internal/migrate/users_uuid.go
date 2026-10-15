package migrate

import (
	"context"
	"database/sql"
	"fmt"
)

// A developer may have applied dev's 035 manually before versioned deployment.
// Recognize its complete effect without rerunning the backfill or baselining any
// other version. Caller holds the normal database-scoped migration lock.
func (m *Migrator) adoptExistingUsersUUID(ctx context.Context, mig Migration) (bool, error) {
	if mig.Version != 35 || mig.UpFile != "035_add_uuid_to_users.sql" {
		return false, nil
	}
	var kind, nullable string
	var length sql.NullInt64
	err := m.conn.QueryRowContext(ctx, `SELECT data_type,is_nullable,character_maximum_length FROM information_schema.columns WHERE table_schema=DATABASE() AND table_name='users' AND column_name='uuid'`).Scan(&kind, &nullable, &length)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("inspect existing users.uuid: %w", err)
	}
	if kind != "char" || nullable != "NO" || !length.Valid || length.Int64 != 36 {
		return false, fmt.Errorf("users.uuid exists but migration 035 is unrecorded and its CHAR(36) NOT NULL schema is incomplete; reconcile before deployment")
	}
	var unique, invalid int
	if err := m.conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM (SELECT index_name FROM information_schema.statistics WHERE table_schema=DATABASE() AND table_name='users' GROUP BY index_name HAVING MIN(non_unique)=0 AND COUNT(*)=1 AND MAX(column_name)='uuid' AND MAX(sub_part) IS NULL) uuid_indexes`).Scan(&unique); err != nil {
		return false, err
	}
	if unique < 1 {
		return false, fmt.Errorf("users.uuid has no full-column unique index; migration 035 cannot be adopted")
	}
	if err := m.conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM users WHERE uuid IS NULL OR uuid NOT REGEXP '^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$' OR CAST(uuid AS BINARY)<>CAST(LOWER(uuid) AS BINARY)`).Scan(&invalid); err != nil {
		return false, err
	}
	if invalid != 0 {
		return false, fmt.Errorf("users.uuid contains %d invalid values; migration 035 cannot be adopted", invalid)
	}
	if _, err := m.conn.ExecContext(ctx, "INSERT INTO "+Table+" (version,name,checksum,baseline,execution_ms) VALUES (?,?,?,1,0)", mig.Version, mig.UpFile, mig.Checksum); err != nil {
		return false, err
	}
	fmt.Fprintln(m.out, "035_add_uuid_to_users.sql  verified existing UUID schema/data; recorded only 035, no UUIDs changed")
	return true, nil
}
