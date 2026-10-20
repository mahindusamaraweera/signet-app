package jobs

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"github.com/jmoiron/sqlx"
	"net/http"
	"net/http/httptest"
	"signet-backend/internal/blockchain"
	"strings"
	"testing"
)

type depositTestSender struct{ calls int }

type depositSendFunc func(context.Context, string, string, string) error

func (f depositSendFunc) Send(ctx context.Context, to, subject, body string) error {
	return f(ctx, to, subject, body)
}

func TestDepositMailDeliveryFollowsClaimAndRecordsOutcome(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "accepted", true: "review"}[fail], func(t *testing.T) {
			f := &expiryFixture{steps: []expiryStep{
				{query: "SELECT u.id", columns: []string{"id", "before_id"}},
				{query: "SELECT n.remote_deposit_id", columns: []string{"remote_deposit_id", "email", "amount", "tx_hash"}, values: []driver.Value{int64(1), "user@example.test", "10", "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}},
			}, execs: []string{"SET status='sending'", "SET status=?"}}
			conn := sql.OpenDB(f)
			defer conn.Close()
			calls := 0
			sender := depositSendFunc(func(_ context.Context, to, subject, body string) error {
				calls++
				if len(f.execs) != 1 {
					t.Fatal("send before claim")
				}
				if to != "user@example.test" || subject != "Deposit confirmed" || body == "" {
					t.Fatal("wrong message")
				}
				if fail {
					return errors.New("ambiguous mail transport")
				}
				return nil
			})
			err := RunDepositNotifications(sqlx.NewDb(conn, "fixture"), nil, sender)
			if (err != nil) != fail || calls != 1 || len(f.execs) != 0 || len(f.steps) != 0 {
				t.Fatalf("outcome not audited: %v", err)
			}
		})
	}
}

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

func TestDepositSourceFailureIdentifiesUserAndCursorWithoutBodyLeak(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/integration/deposits" {
			t.Error("wrong endpoint")
		}
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"message":"secret-token must not appear in logs"}`))
	}))
	defer server.Close()
	gateway, err := blockchain.New(server.URL, "test-token")
	if err != nil {
		t.Fatal(err)
	}
	f := &expiryFixture{steps: []expiryStep{
		{query: "SELECT u.id", columns: []string{"id", "before_id"}, values: []driver.Value{int64(42), int64(123)}},
		{query: "SELECT n.remote_deposit_id", columns: []string{"remote_deposit_id", "email", "amount", "tx_hash"}},
	}, execs: []string{"INSERT INTO deposit_notification_scans"}}
	conn := sql.OpenDB(f)
	defer conn.Close()
	sender := &depositTestSender{}
	err = RunDepositNotifications(sqlx.NewDb(conn, "fixture"), gateway, sender)
	if err == nil || !strings.Contains(err.Error(), "user_id=42 before_id=123 stage=token_deposits http=404") || strings.Contains(err.Error(), "secret-token") {
		t.Fatalf("unsafe or incomplete diagnostics: %v", err)
	}
	if sender.calls != 0 || len(f.execs) != 0 || len(f.steps) != 0 {
		t.Fatal("failed fetch sent mail or changed cursor")
	}
}
