package wallet

import (
	"database/sql"
	"errors"
	"strings"
	"testing"
)

type testCapReader struct {
	cap, balance, price float64
	percent             int64
	fail                bool
	queries             []string
}

func (r *testCapReader) Get(dest interface{}, query string, args ...interface{}) error {
	r.queries = append(r.queries, query)
	if r.fail {
		return errors.New("read failed")
	}
	switch value := dest.(type) {
	case *sql.NullFloat64:
		value.Valid = true
		switch {
		case strings.Contains(query, "SUM(p.price * 4)"):
			value.Float64 = r.cap
		case strings.Contains(query, "SELECT balance"):
			value.Float64 = r.balance
		case strings.Contains(query, "SELECT price"):
			value.Float64 = r.price
		default:
			return errors.New("unexpected amount query")
		}
	case *sql.NullInt64:
		value.Valid = true
		value.Int64 = r.percent
	default:
		return errors.New("unexpected destination")
	}
	return nil
}

func TestCapReaderPreservesLegacyQueriesAndBoundary(t *testing.T) {
	for _, tc := range []struct {
		balance float64
		allowed int
	}{{350, 1}, {350.01, 0}, {349.99, 1}} {
		r := &testCapReader{cap: 400, balance: tc.balance, price: 100, percent: 50}
		result, err := checkWalletFrom(r, 42, 7)
		if err != nil || result != tc.allowed {
			t.Fatalf("balance %v: result %d error %v", tc.balance, result, err)
		}
		if len(r.queries) != 4 || !strings.Contains(r.queries[2], "ORDER BY up.created_at DESC LIMIT 1") {
			t.Fatal("legacy active-package commission selection changed")
		}
	}
}

func TestCapReadFailureDoesNotAuthorizeCredit(t *testing.T) {
	result, err := checkWalletFrom(&testCapReader{fail: true}, 42, 7)
	if err == nil || result != 0 {
		t.Fatal("failed cap read authorized a credit")
	}
}
