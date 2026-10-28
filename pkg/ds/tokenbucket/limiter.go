// Package tokenbucket implements a thread-safe token bucket rate limiter with a mockable clock.
//
// The token bucket algorithm allows bursty traffic up to a configured burst capacity
// while constraining sustained throughput to a smooth replenishment rate.
//
// Time Complexity:
//   - Allow / AllowN: O(1) - single arithmetic update of accumulated tokens.
//   - RetryAfter:     O(1) - simple linear formula: (needed - tokens) / refillRate.
//
// Space Complexity:
//   - O(1) - constant memory footprint per bucket.
package tokenbucket

import (
	"math"
	"sync"
	"time"
)

// Clock abstracts time measurement, enabling deterministic, instant unit testing
// without relying on sleeping.
type Clock interface {
	Now() time.Time
}

// realClock implements Clock using the system clock.
type realClock struct{}

func (realClock) Now() time.Time {
	return time.Now()
}

// Bucket represents a thread-safe token bucket rate limiter.
type Bucket struct {
	mu         sync.Mutex
	capacity   float64 // maximum burst capacity
	refillRate float64 // tokens added per second
	tokens     float64 // current available token count
	lastRefill time.Time
	clock      Clock
}

// New creates a new token bucket initialized to full capacity using the system clock.
// capacity is the burst size; refillRate is tokens per second.
func New(capacity float64, refillRate float64) *Bucket {
	return NewWithClock(capacity, refillRate, realClock{})
}

// NewWithClock creates a token bucket with an injected Clock for testing.
func NewWithClock(capacity float64, refillRate float64, clock Clock) *Bucket {
	now := clock.Now()
	return &Bucket{
		capacity:   capacity,
		refillRate: refillRate,
		tokens:     capacity, // start full
		lastRefill: now,
		clock:      clock,
	}
}

// Allow is shorthand for AllowN(1.0).
func (b *Bucket) Allow() bool {
	return b.AllowN(1.0)
}

// AllowN attempts to consume n tokens from the bucket.
// Returns true if sufficient tokens were available, or false if rejected.
func (b *Bucket) AllowN(n float64) bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.refill()

	if b.tokens >= n {
		b.tokens -= n
		return true
	}
	return false
}

// Tokens returns the current number of tokens available in the bucket.
func (b *Bucket) Tokens() float64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.refill()
	return b.tokens
}

// RetryAfter calculates how long a caller must wait before n tokens will be available.
// If n tokens are already available, it returns 0.
func (b *Bucket) RetryAfter(n float64) time.Duration {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.refill()

	if b.tokens >= n {
		return 0
	}

	needed := n - b.tokens
	seconds := needed / b.refillRate
	return time.Duration(math.Ceil(seconds * float64(time.Second)))
}

// refill updates the token count based on elapsed time since last refill.
// Caller must hold b.mu.
func (b *Bucket) refill() {
	now := b.clock.Now()
	elapsed := now.Sub(b.lastRefill).Seconds()
	if elapsed <= 0 {
		return
	}

	b.tokens = math.Min(b.capacity, b.tokens+(elapsed*b.refillRate))
	b.lastRefill = now
}
