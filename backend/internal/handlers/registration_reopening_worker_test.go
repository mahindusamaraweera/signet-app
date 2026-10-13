package handlers

import (
	"errors"
	"signet-backend/internal/blockchain"
	"testing"
)

func TestReopeningFailuresDistinguishUncertainRetryFromCustodyConflict(t *testing.T) {
	for _, tc := range []struct {
		err    error
		status string
	}{
		{blockchain.ErrUnavailable, "pending"},
		{errors.New("timeout with private response"), "pending"},
		{errors.Join(blockchain.ErrUnavailable, blockchain.ErrConflict), "review_required"},
		{blockchain.ErrRejected, "review_required"},
	} {
		if got := reopeningFailureState(tc.err); got != tc.status {
			t.Fatalf("wrong reopening recovery state %s", got)
		}
	}
}
