package blockchain

import (
	"context"
	"database/sql"
	"fmt"
)

func (g *Gateway) UseSQLUUIDIdentity(db *sql.DB) {
	g.UseUUIDIdentity(func(ctx context.Context, id uint64) (string, error) {
		var uuid string
		err := db.QueryRowContext(ctx, "SELECT uuid FROM users WHERE id=?", id).Scan(&uuid)
		return uuid, err
	}, func(ctx context.Context, uuid string) (uint64, error) {
		var id uint64
		err := db.QueryRowContext(ctx, "SELECT id FROM users WHERE uuid=?", uuid).Scan(&id)
		return id, err
	})
}
func (g *Gateway) localAssertionOwner(ctx context.Context, owner string) (uint64, error) {
	if g.identityResolver != nil {
		if !uuidIdentity.MatchString(owner) || g.identityReverse == nil {
			return 0, ErrRejected
		}
		id, err := g.identityReverse(ctx, owner)
		if err != nil || id == 0 {
			return 0, ErrRejected
		}
		return id, nil
	}
	var id uint64
	// Legacy test/transition clients have no resolver; production always sets one.
	if _, err := fmt.Sscan(owner, &id); err != nil || id == 0 || fmt.Sprint(id) != owner {
		return 0, ErrRejected
	}
	return id, nil
}
