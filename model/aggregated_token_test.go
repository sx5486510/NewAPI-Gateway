package model

import (
	"sync"
	"sync/atomic"
	"testing"
)

func TestShouldFlushAggTokenAccessedAtThrottles(t *testing.T) {
	resetAggTokenAccessedAtForTest()

	now := int64(1_800_000_000)
	if !shouldFlushAggTokenAccessedAt(7, now) {
		t.Fatal("first call should flush")
	}
	if shouldFlushAggTokenAccessedAt(7, now+1) {
		t.Fatal("second call inside the window should be skipped")
	}
	if shouldFlushAggTokenAccessedAt(7, now+aggTokenAccessedAtIntervalSeconds-1) {
		t.Fatal("call just before the window closes should be skipped")
	}
	if !shouldFlushAggTokenAccessedAt(7, now+aggTokenAccessedAtIntervalSeconds) {
		t.Fatal("call after the window should flush again")
	}
}

func TestShouldFlushAggTokenAccessedAtIsPerToken(t *testing.T) {
	resetAggTokenAccessedAtForTest()

	now := int64(1_800_000_000)
	if !shouldFlushAggTokenAccessedAt(1, now) {
		t.Fatal("token 1 should flush")
	}
	if !shouldFlushAggTokenAccessedAt(2, now) {
		t.Fatal("token 2 has its own window and should flush")
	}
	if shouldFlushAggTokenAccessedAt(1, now+5) {
		t.Fatal("token 1 should still be throttled")
	}
}

func TestShouldFlushAggTokenAccessedAtRejectsInvalidId(t *testing.T) {
	resetAggTokenAccessedAtForTest()

	if shouldFlushAggTokenAccessedAt(0, 1_800_000_000) {
		t.Fatal("token id 0 should never flush")
	}
	if shouldFlushAggTokenAccessedAt(-3, 1_800_000_000) {
		t.Fatal("negative token id should never flush")
	}
}

// TestShouldFlushAggTokenAccessedAtBurstReservesOnce covers the race the throttle
// exists for: a burst of requests on one token must produce a single write.
func TestShouldFlushAggTokenAccessedAtBurstReservesOnce(t *testing.T) {
	resetAggTokenAccessedAtForTest()

	const burst = 64
	now := int64(1_800_000_000)
	var allowed int64
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < burst; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			if shouldFlushAggTokenAccessedAt(9, now) {
				atomic.AddInt64(&allowed, 1)
			}
		}()
	}
	close(start)
	wg.Wait()

	if allowed != 1 {
		t.Fatalf("expected exactly one flush permit for a %d-request burst, got %d", burst, allowed)
	}
}
