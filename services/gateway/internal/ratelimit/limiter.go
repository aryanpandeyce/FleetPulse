// Package ratelimit provides distributed token-bucket rate limiting backed by Redis
// Lua script atomicity with seamless in-memory fallback.
package ratelimit

import (
	"context"
	"math"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/fleetpulse/fleetpulse/pkg/ds/tokenbucket"
	"github.com/fleetpulse/fleetpulse/pkg/redis"
)

// Redis Lua script for atomic token-bucket rate limiting
// KEYS[1]: bucket key (e.g. ratelimit:org:123 or ratelimit:ip:1.2.3.4)
// ARGV[1]: capacity (burst limit)
// ARGV[2]: refill_rate (tokens/sec)
// ARGV[3]: now (unix timestamp float seconds)
// ARGV[4]: requested tokens (e.g. 1)
const luaTokenBucketScript = `
local key = KEYS[1]
local capacity = tonumber(ARGV[1])
local refill_rate = tonumber(ARGV[2])
local now = tonumber(ARGV[3])
local requested = tonumber(ARGV[4])

-- Get current state: tokens, last_refill
local data = redis.call('HMGET', key, 'tokens', 'last_refill')
local tokens = tonumber(data[1])
local last_refill = tonumber(data[2])

if not tokens then
    tokens = capacity
    last_refill = now
else
    local elapsed = now - last_refill
    if elapsed > 0 then
        tokens = math.min(capacity, tokens + (elapsed * refill_rate))
        last_refill = now
    end
end

if tokens >= requested then
    tokens = tokens - requested
    redis.call('HMSET', key, 'tokens', tokens, 'last_refill', last_refill)
    redis.call('EXPIRE', key, math.ceil(capacity / refill_rate) * 2)
    return {1, 0, tokens}
else
    local needed = requested - tokens
    local retry_after = math.ceil(needed / refill_rate)
    return {0, retry_after, tokens}
end
`

// Limiter manages rate limits using Redis with in-memory fallback.
type Limiter struct {
	rdb        *redis.Client
	scriptSHA  string
	mu         sync.RWMutex
	fallback   map[string]*tokenbucket.Bucket
	capacity   float64
	refillRate float64
}

// New creates a new Limiter. If rdb is nil, it uses in-memory mode exclusively.
func New(rdb *redis.Client, capacity, refillRate float64) *Limiter {
	l := &Limiter{
		rdb:        rdb,
		fallback:   make(map[string]*tokenbucket.Bucket),
		capacity:   capacity,
		refillRate: refillRate,
	}
	if rdb != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		sha, err := rdb.ScriptLoad(ctx, luaTokenBucketScript)
		if err == nil {
			l.scriptSHA = sha
		}
	}
	return l
}

// Allow evaluates if a request with key (org or IP) is permitted.
// Returns (allowed bool, retryAfterSeconds int).
func (l *Limiter) Allow(ctx context.Context, key string) (bool, int) {
	if l.rdb != nil && l.scriptSHA != "" {
		now := float64(time.Now().UnixNano()) / 1e9
		res, err := l.rdb.EvalSha(ctx, l.scriptSHA, []string{"ratelimit:" + key}, l.capacity, l.refillRate, now, 1)
		if err == nil {
			if vals, ok := res.([]interface{}); ok && len(vals) >= 2 {
				allowed := vals[0].(int64) == 1
				retryAfter := int(vals[1].(int64))
				return allowed, retryAfter
			}
		}
	}

	// In-memory fallback
	l.mu.Lock()
	bucket, exists := l.fallback[key]
	if !exists {
		bucket = tokenbucket.New(l.capacity, l.refillRate)
		l.fallback[key] = bucket
	}
	l.mu.Unlock()

	if bucket.Allow() {
		return true, 0
	}
	wait := bucket.RetryAfter(1.0)
	retrySecs := int(math.Ceil(wait.Seconds()))
	if retrySecs <= 0 {
		retrySecs = 1
	}
	return false, retrySecs
}

// Middleware creates HTTP middleware enforcing rate limits per org or per IP.
func (l *Limiter) Middleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Key by X-Forwarded-For or RemoteAddr
			key := r.Header.Get("X-Forwarded-For")
			if key == "" {
				key = r.RemoteAddr
			}

			allowed, retryAfter := l.Allow(r.Context(), key)
			if !allowed {
				w.Header().Set("Retry-After", strconv.Itoa(retryAfter))
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusTooManyRequests)
				_, _ = w.Write([]byte(`{"error":{"code":"RATE_LIMIT_EXCEEDED","message":"too many requests, please retry later"}}`))
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
