package handlers

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestPurchaseChecksSlowPurchaseDoesNotBlockOtherUsers(t *testing.T) {
	release := make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	defer unblock()
	fast := make(chan struct{})
	done := make(chan struct{})
	go func() {
		runPurchaseChecks([]uint64{1, 2}, func(id uint64) error {
			if id == 1 {
				<-release
			} else {
				close(fast)
			}
			return nil
		}, func(uint64, error) { t.Error("unexpected error") })
		close(done)
	}()
	select {
	case <-fast:
	case <-time.After(2 * time.Second):
		t.Fatal("later user waited for slow purchase")
	}
	select {
	case <-done:
		t.Fatal("batch completed without waiting for slow user")
	default:
	}
	unblock()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("batch did not finish after slow request completed")
	}
}

func TestPurchaseChecksAreBoundedAndPreserveEveryRetry(t *testing.T) {
	ids := []uint64{1, 2, 3, 4, 5, 6, 7, 8, 9}
	var active, max atomic.Int32
	var mu sync.Mutex
	checked := map[uint64]int{}
	failures := map[uint64]int{}
	runPurchaseChecks(ids, func(id uint64) error {
		n := active.Add(1)
		for {
			old := max.Load()
			if n <= old || max.CompareAndSwap(old, n) {
				break
			}
		}
		defer active.Add(-1)
		mu.Lock()
		checked[id]++
		mu.Unlock()
		if id == 3 || id == 7 {
			return errors.New("custody temporarily unavailable")
		}
		return nil
	}, func(id uint64, err error) { mu.Lock(); defer mu.Unlock(); failures[id]++ })
	if max.Load() > 4 {
		t.Fatal("unbounded activation requests")
	}
	for _, id := range ids {
		if checked[id] != 1 {
			t.Fatalf("purchase %d checked %d times", id, checked[id])
		}
	}
	if len(failures) != 2 || failures[3] != 1 || failures[7] != 1 {
		t.Fatal("failed purchase identities were lost")
	}
}
