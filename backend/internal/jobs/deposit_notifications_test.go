package jobs

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"github.com/jmoiron/sqlx"
	"testing"
)

type depositTestSender struct{ calls int }

func (s *depositTestSender) Send(context.Context, string, string, string) error {
	s.calls++
	return nil
}

func TestDepositMailClaimFailureDoesNotSend(t *testing.T) {
	f := &expiryFixture{steps: []expiryStep{
		{query: "SELECT u.id", columns: []string{"id", "before_id"}},
		{query: "SELECT n.remote_deposit_id", columns: []string{"remote_deposit_id", "email", "amount", "tx_hash"}, values: []driver.Value{int64(1), "user@example.test", "10", "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}},
	}, failWrite: "SET status='sending'"}
	conn := sql.OpenDB(f)
	defer conn.Close()
	sender := &depositTestSender{}
	if err := RunDepositNotifications(sqlx.NewDb(conn, "fixture"), nil, sender); err == nil {
		t.Fatal("claim failure must be reported")
	}
	if sender.calls != 0 {
		t.Fatal("email sent before durable claim")
	}
}

func TestDepositMailRequiresSettledUSDT(t *testing.T) {
	date := "2026-10-11T00:00:00Z"
	d := confirmedMailDeposit{ID: 1, Asset: "USDT", Amount: "0.000000000000000001", Hash: "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Status: "credited", CreditedAt: &date}
	if !depositMailEligible(d) {
		t.Fatal("confirmed partial deposit must notify")
	}
	for _, status := range []string{"detected", "confirmed", "internal_transfer", "external_reconciliation", ""} {
		candidate := d
		candidate.Status = status
		if depositMailEligible(candidate) {
			t.Fatalf("not settled: %s", status)
		}
	}
	candidate := d
	candidate.CreditedAt = nil
	if depositMailEligible(candidate) {
		t.Fatal("missing credit timestamp")
	}
	candidate = d
	candidate.Asset = "SIGNET"
	if depositMailEligible(candidate) {
		t.Fatal("unexpected asset")
	}
	candidate = d
	candidate.Amount = "1\nInjected"
	if depositMailEligible(candidate) {
		t.Fatal("malformed amount")
	}
	candidate = d
	candidate.Amount = "0.000000000000000000"
	if depositMailEligible(candidate) {
		t.Fatal("zero is not a deposit")
	}
	candidate = d
	candidate.Hash = "javascript:alert(1)"
	if depositMailEligible(candidate) {
		t.Fatal("malformed hash")
	}
}
