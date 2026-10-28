package tokenbucket

import (
	"sync"
	"testing"
	"time"
)

// FakeClock allows precise control over time in tests.
type FakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func NewFakeClock(t time.Time) *FakeClock {
	return &FakeClock{now: t}
}

func (f *FakeClock) Now() time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.now
}

func (f *FakeClock) Advance(d time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.now = f.now.Add(d)
}

func TestTokenBucketBurstAndExhaustion(t *testing.T) {
	clock := NewFakeClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	// 5 tokens capacity, 1 token per second refill
	tb := NewWithClock(5.0, 1.0, clock)

	// Consume 5 tokens (all allowed)
	for i := 0; i < 5; i++ {
		if !tb.Allow() {
			t.Fatalf("expected token %d to be allowed", i+1)
		}
	}

	// 6th token must be rejected
	if tb.Allow() {
		t.Fatalf("expected 6th request to be rejected")
	}

	retryAfter := tb.RetryAfter(1.0)
	if retryAfter < 1*time.Second {
		t.Fatalf("expected RetryAfter >= 1s, got %v", retryAfter)
	}
}

func TestTokenBucketRefill(t *testing.T) {
	clock := NewFakeClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	// 10 tokens capacity, 5 tokens per second refill
	tb := NewWithClock(10.0, 5.0, clock)

	// Exhaust all 10
	for i := 0; i < 10; i++ {
		if !tb.Allow() {
			t.Fatalf("token %d rejected", i)
		}
	}
	if tb.Allow() {
		t.Fatalf("expected empty bucket")
	}

	// Advance clock by 1 second -> should refill 5 tokens
	clock.Advance(1 * time.Second)

	for i := 0; i < 5; i++ {
		if !tb.Allow() {
			t.Fatalf("refilled token %d should be allowed", i+1)
		}
	}

	// 6th must fail
	if tb.Allow() {
		t.Fatalf("expected 6th request after refill to be rejected")
	}
}

func TestTokenBucketCapacityCapping(t *testing.T) {
	clock := NewFakeClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	// 5 capacity, 10 refill per second
	tb := NewWithClock(5.0, 10.0, clock)

	// Wait 100 seconds
	clock.Advance(100 * time.Second)

	// Available tokens must still be capped at 5
	tokens := tb.Tokens()
	if tokens > 5.0 {
		t.Fatalf("tokens exceeded max capacity: %f", tokens)
	}

	// Can only consume 5 tokens
	for i := 0; i < 5; i++ {
		if !tb.Allow() {
			t.Fatalf("expected token %d to be allowed", i+1)
		}
	}
	if tb.Allow() {
		t.Fatalf("expected 6th token to fail even after long idle time")
	}
}

func TestTokenBucketAllowN(t *testing.T) {
	clock := NewFakeClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	tb := NewWithClock(10.0, 2.0, clock)

	if !tb.AllowN(7.0) {
		t.Fatalf("expected 7 tokens to be allowed")
	}

	// 3 tokens left
	if tb.AllowN(4.0) {
		t.Fatalf("expected 4 tokens to be rejected when only 3 remain")
	}

	if !tb.AllowN(3.0) {
		t.Fatalf("expected remaining 3 tokens to be allowed")
	}
}

func TestTokenBucketConcurrency(t *testing.T) {
	tb := New(1000.0, 500.0)
	var wg sync.WaitGroup

	allowed := 0
	var mu sync.Mutex

	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				if tb.Allow() {
					mu.Lock()
					allowed++
					mu.Unlock()
				}
			}
		}()
	}

	wg.Wait()
	if allowed == 0 {
		t.Fatalf("expected some tokens to be consumed concurrently")
	}
}

func BenchmarkTokenBucketAllow(b *testing.B) {
	tb := New(float64(b.N), float64(b.N))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = tb.Allow()
	}
}
