package totp

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"
)

type counterResult int64

func (r counterResult) LastInsertId() (int64, error) { return 0, nil }
func (r counterResult) RowsAffected() (int64, error) { return int64(r), nil }

type counterFake struct {
	calls int
	last  int64
	fail  bool
}

func (f *counterFake) ExecContext(_ context.Context, query string, args ...interface{}) (sql.Result, error) {
	f.calls++
	if f.fail {
		return nil, errors.New("storage unavailable")
	}
	if strings.HasPrefix(query, "INSERT") {
		return counterResult(1), nil
	}
	counter := args[0].(int64)
	if counter <= f.last {
		return counterResult(0), nil
	}
	f.last = counter
	return counterResult(1), nil
}

func TestWithdrawalCounterConsumption(t *testing.T) {
	ctx := context.Background()
	at := time.Unix(1234567890, 0)
	secret := "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ"
	value, _ := code(secret, at.Unix()/30)
	store := &counterFake{last: -1}
	if err := consumeWithdrawalCode(ctx, store, 42, secret, value, at); err != nil {
		t.Fatal(err)
	}
	if err := consumeWithdrawalCode(ctx, store, 42, secret, value, at); !errors.Is(err, ErrInvalidOrReusedCode) {
		t.Fatalf("replay: %v", err)
	}
	invalid := &counterFake{last: -1}
	if err := consumeWithdrawalCode(ctx, invalid, 42, secret, "bad", at); !errors.Is(err, ErrInvalidOrReusedCode) || invalid.calls != 0 {
		t.Fatal("invalid code touched storage")
	}
	if err := consumeWithdrawalCode(ctx, &counterFake{fail: true}, 42, secret, value, at); err == nil {
		t.Fatal("storage failure accepted")
	}
	if err := ConsumeWithdrawalCode(ctx, nil, 42, secret, value, at); err == nil {
		t.Fatal("missing transaction accepted")
	}
}
