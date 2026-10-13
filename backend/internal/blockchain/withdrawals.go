package blockchain

import (
	"context"
	"encoding/json"
	"fmt"
)

func (g *Gateway) WithdrawalHistory(ctx context.Context, userID, beforeID uint64) (json.RawMessage, error) {
	var response json.RawMessage
	body := map[string]interface{}{"source": "signet-app", "external_user_id": fmt.Sprint(userID)}
	if beforeID > 0 {
		body["before_id"] = beforeID
	}
	err := g.request(ctx, "POST", "/api/v1/integration/withdrawals/history", fmt.Sprintf("signet-app:withdrawals:%d:%d", userID, beforeID), body, &response)
	return response, err
}
