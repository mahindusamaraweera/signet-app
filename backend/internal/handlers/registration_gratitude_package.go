package handlers

import (
	"time"

	"github.com/jmoiron/sqlx"

	"signet-backend/internal/tree"
)

// gratitudeBlockedPackageID is the package a registration may not buy when
// its placement makes it a gratitude sale (the sponsor's 2nd, 5th, 10th, ...
// sale, or any other placement that routes the commission up as gratitude).
const gratitudeBlockedPackageID uint64 = 2

const gratitudeBlockedPackageMessage = "This package is not available for this registration because it is a gratitude sale. Please choose another package."

// registrationIsGratitude reports whether the placement just resolved in tx
// for newUserID created a gratitude entitlement. tree.SuperParentFind writes
// that entitlement as a super_parent_logs row (deduplicated over 5 minutes),
// and activation pays the gratitude commission from that same row, so it is
// the authoritative signal rather than re-deriving the sale count here.
func registrationIsGratitude(tx *sqlx.Tx, newUserID uint64) (bool, error) {
	var n int
	err := tx.Get(&n, `SELECT COUNT(*) FROM super_parent_logs WHERE gratitude_user = ? AND created_at >= ?`,
		newUserID, time.Now().Add(-5*time.Minute))
	return n > 0, err
}

// previewRegistrationIsGratitude runs the real placement for newUserID inside
// a transaction that is always rolled back, so the package form can mark the
// blocked package before the user submits. The gratitude decision does not
// depend on which package is chosen. The submit handler re-checks inside its
// own transaction, which is what actually enforces the rule.
func previewRegistrationIsGratitude(db *sqlx.DB, newUserID, referredBy uint64) (bool, error) {
	tx, err := db.Beginx()
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	var created []uint64
	if _, err := tree.ParentFind(tx, referredBy, gratitudeBlockedPackageID, newUserID, 1, &created); err != nil {
		return false, err
	}
	return registrationIsGratitude(tx, newUserID)
}

// markGratitudeBlockedPackage flags the blocked package as unavailable
// (it stays in the list so the user can see why it can't be picked).
func markGratitudeBlockedPackage(choices []purchasePackageChoice) {
	for i := range choices {
		if choices[i].ID == gratitudeBlockedPackageID {
			choices[i].Unavailable = true
			choices[i].UnavailableReason = gratitudeBlockedPackageMessage
		}
	}
}
