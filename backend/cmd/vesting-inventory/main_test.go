package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestHistoricalEvidenceDoesNotTreatWebhookOrAccrualAsPayment(t *testing.T) {
	limits := inventoryEvidenceLimits()
	if !strings.Contains(limits["recorded_mining_token"], "not proof") || !strings.Contains(limits["recorded_webhook_sent_at"], "not proof") {
		t.Fatal("ambiguous historical evidence")
	}
	encoded, err := json.Marshal(miningEvidence{ID: 7, UserID: 42, Accrued: "123.5", WebhookStatus: "sent", WebhookSent: "2026-10-11 12:00:00"})
	if err != nil {
		t.Fatal(err)
	}
	var row map[string]interface{}
	if err := json.Unmarshal(encoded, &row); err != nil {
		t.Fatal(err)
	}
	if row["recorded_mining_token"] != "123.5" || row["recorded_webhook_status"] != "sent" {
		t.Fatal("source evidence changed")
	}
	for _, forbidden := range []string{"previously_paid", "available_unpaid", "secret_key", "webhook_response"} {
		if _, found := row[forbidden]; found {
			t.Fatalf("export inferred/exposed %s", forbidden)
		}
	}
}
