package blockchain

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
)

// Reopen only after committing the source replacement placement and new intent.
// The same durable identities must be retried after any uncertain response.
func (g *Gateway) ReopenRegistration(ctx context.Context, userID, expiredID, placementID, purchaseID uint64) error {
	if userID == 0 || expiredID == 0 || placementID == 0 || purchaseID == 0 || expiredID == placementID {
		return ErrUnavailable
	}
	user, old, next, purchase := strconv.FormatUint(userID, 10), strconv.FormatUint(expiredID, 10), strconv.FormatUint(placementID, 10), strconv.FormatUint(purchaseID, 10)
	var result struct {
		User      string `json:"external_user_id"`
		Expired   string `json:"expired_placement_log_id"`
		Placement string `json:"new_placement_log_id"`
		Purchase  string `json:"external_purchase_id"`
		Reopened  *bool  `json:"reopened"`
	}
	err := g.request(ctx, http.MethodPost, "/api/v1/integration/registration-expiry/reopen", fmt.Sprintf("signet-app:registration-reopen:%d:%d:%d:%d", userID, expiredID, placementID, purchaseID),
		map[string]string{"source": "signet-app", "external_user_id": user, "expired_placement_log_id": old, "new_placement_log_id": next, "external_purchase_id": purchase}, &result)
	if err != nil {
		return err
	}
	if result.Reopened == nil || !*result.Reopened || result.User != user || result.Expired != old || result.Placement != next || result.Purchase != purchase {
		return ErrUnavailable
	}
	return nil
}
