package blockchain

import (
	"context"
	"encoding/json"
	"fmt"
)

func (g *Gateway) Vesting(ctx context.Context, userID uint64) (json.RawMessage, error) {
	var response json.RawMessage
	err := g.request(ctx, "POST", "/api/v1/integration/vesting", fmt.Sprintf("signet-app:vesting:%d", userID), map[string]interface{}{"source": "signet-app", "external_user_id": fmt.Sprint(userID)}, &response)
	return response, err
}

type VestingAdjustment struct {
	ScheduleID       uint64 `json:"schedule_id"`
	AdjustmentID     string `json:"adjustment_id"`
	Direction        string `json:"direction"`
	Days             uint64 `json:"days"`
	Reason           string `json:"reason"`
	ExpectedRevision uint64 `json:"expected_revision"`
}

type VestingSwitch struct {
	Mode              string `json:"mode"`
	ScheduleID        uint64 `json:"schedule_id"`
	RequestID         string `json:"request_id"`
	ConfirmationToken string `json:"confirmation_token"`
	Consent           bool   `json:"consent"`
}

func (g *Gateway) PreviewVestingSwitch(ctx context.Context, userID, scheduleID uint64, mode string) (json.RawMessage, error) {
	var response json.RawMessage
	err := g.request(ctx, "POST", "/api/v1/integration/vesting/switch/preview", "", map[string]interface{}{
		"source": "signet-app", "external_user_id": fmt.Sprint(userID), "schedule_id": scheduleID, "mode": mode,
	}, &response)
	return response, err
}

func (g *Gateway) CommitVestingSwitch(ctx context.Context, userID uint64, body VestingSwitch) (json.RawMessage, error) {
	var response json.RawMessage
	err := g.request(ctx, "POST", "/api/v1/integration/vesting/switch/commit", "signet-app:vesting-switch:"+body.RequestID, map[string]interface{}{
		"source": "signet-app", "external_user_id": fmt.Sprint(userID), "schedule_id": body.ScheduleID,
		"request_id": body.RequestID, "confirmation_token": body.ConfirmationToken, "consent": body.Consent,
	}, &response)
	return response, err
}

func (g *Gateway) AdjustVesting(ctx context.Context, userID, adminID uint64, body VestingAdjustment) (json.RawMessage, error) {
	var response json.RawMessage
	err := g.request(ctx, "POST", "/api/v1/integration/vesting/adjust", "signet-app:vesting-adjust:"+body.AdjustmentID, map[string]interface{}{
		"source": "signet-app", "external_user_id": fmt.Sprint(userID), "admin_id": fmt.Sprint(adminID),
		"schedule_id": body.ScheduleID, "adjustment_id": body.AdjustmentID, "direction": body.Direction, "days": body.Days, "reason": body.Reason, "expected_revision": body.ExpectedRevision,
	}, &response)
	return response, err
}
