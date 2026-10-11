package blockchain

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"
)

type RegistrationExpirySeal struct {
	PlacementLogID string `json:"placement_log_id"`
	UserID         string `json:"external_user_id"`
	SealedAt       string `json:"sealed_at"`
	ScannedBlock   uint64 `json:"scanned_block"`
	ObservedHead   uint64 `json:"observed_head"`
}

// A durable exact-identity reservation barrier, not a read-only balance check.
func (g *Gateway) SealRegistrationExpiry(ctx context.Context, userID, placementLogID uint64) (*RegistrationExpirySeal, error) {
	if userID == 0 || placementLogID == 0 {
		return nil, ErrUnavailable
	}
	var result struct {
		CanExpire *bool                   `json:"can_expire"`
		Seal      *RegistrationExpirySeal `json:"seal"`
	}
	err := g.request(ctx, http.MethodPost, "/api/v1/integration/registration-expiry/seal", fmt.Sprintf("signet-app:expiry-seal:%d:%d", userID, placementLogID),
		map[string]string{"source": "signet-app", "external_user_id": strconv.FormatUint(userID, 10), "placement_log_id": strconv.FormatUint(placementLogID, 10)}, &result)
	if err != nil {
		return nil, err
	}
	if result.CanExpire == nil {
		return nil, ErrUnavailable
	}
	if !*result.CanExpire {
		if result.Seal != nil {
			return nil, ErrUnavailable
		}
		return nil, nil
	}
	seal := result.Seal
	if seal == nil || seal.UserID != strconv.FormatUint(userID, 10) || seal.PlacementLogID != strconv.FormatUint(placementLogID, 10) || seal.ScannedBlock == 0 || seal.ObservedHead <= seal.ScannedBlock {
		return nil, ErrUnavailable
	}
	sealedAt, err := time.Parse(time.RFC3339, seal.SealedAt)
	if err != nil || sealedAt.Unix() <= 0 || sealedAt.After(time.Now().Add(time.Minute)) {
		return nil, ErrUnavailable
	}
	return seal, nil
}
