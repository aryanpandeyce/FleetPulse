package ratelimit

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/fleetpulse/fleetpulse/pkg/redis"
)

func TestLimiter_InMemory(t *testing.T) {
	limiter := New(nil, 3, 1) // 3 tokens, 1 token/sec
	ctx := context.Background()

	// 3 requests allowed
	for i := 0; i < 3; i++ {
		allowed, retryAfter := limiter.Allow(ctx, "tenant-test")
		if !allowed {
			t.Fatalf("expected request %d to be allowed", i+1)
		}
		if retryAfter != 0 {
			t.Fatalf("expected retryAfter 0, got %d", retryAfter)
		}
	}

	// 4th request rejected
	allowed, retryAfter := limiter.Allow(ctx, "tenant-test")
	if allowed {
		t.Fatalf("expected request 4 to be rejected")
	}
	if retryAfter <= 0 {
		t.Fatalf("expected positive retryAfter, got %d", retryAfter)
	}

	// Distinct keys have independent buckets
	allowedOther, _ := limiter.Allow(ctx, "tenant-other")
	if !allowedOther {
		t.Fatalf("expected other tenant to be allowed")
	}
}

func TestLimiter_Middleware(t *testing.T) {
	limiter := New(nil, 2, 1)

	handler := limiter.Middleware()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))

	// Request 1
	req1 := httptest.NewRequest("GET", "/test", nil)
	req1.Header.Set("X-Forwarded-For", "192.168.1.10")
	rec1 := httptest.NewRecorder()
	handler.ServeHTTP(rec1, req1)
	if rec1.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec1.Code)
	}

	// Request 2
	req2 := httptest.NewRequest("GET", "/test", nil)
	req2.Header.Set("X-Forwarded-For", "192.168.1.10")
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec2.Code)
	}

	// Request 3 - rate limited
	req3 := httptest.NewRequest("GET", "/test", nil)
	req3.Header.Set("X-Forwarded-For", "192.168.1.10")
	rec3 := httptest.NewRecorder()
	handler.ServeHTTP(rec3, req3)
	if rec3.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429 Too Many Requests, got %d", rec3.Code)
	}
	if rec3.Header().Get("Retry-After") == "" {
		t.Fatalf("missing Retry-After header")
	}

	// RemoteAddr fallback
	reqFallback := httptest.NewRequest("GET", "/test", nil)
	reqFallback.RemoteAddr = "10.0.0.5:1234"
	recFallback := httptest.NewRecorder()
	handler.ServeHTTP(recFallback, reqFallback)
	if recFallback.Code != http.StatusOK {
		t.Fatalf("expected 200 for new RemoteAddr, got %d", recFallback.Code)
	}
}

func TestLimiter_Redis(t *testing.T) {
	// Start mock RESP server
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer ln.Close()

	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func(conn net.Conn) {
				defer conn.Close()
				buf := make([]byte, 1024)
				for {
					n, err := conn.Read(buf)
					if err != nil {
						return
					}
					msg := string(buf[:n])
					if strings.Contains(msg, "LOAD") {
						_, _ = conn.Write([]byte("$40\r\n726c07153a56e09c8f3074d2843bb369b775463f\r\n"))
					} else if strings.Contains(msg, "EVALSHA") {
						_, _ = conn.Write([]byte("*2\r\n:1\r\n:0\r\n"))
					} else {
						_, _ = conn.Write([]byte("+OK\r\n"))
					}
				}
			}(c)
		}
	}()

	rdb := redis.NewClient(ln.Addr().String())
	defer rdb.Close()

	limiter := New(rdb, 10, 5)
	allowed, retry := limiter.Allow(context.Background(), "redis-tenant")
	if !allowed || retry != 0 {
		t.Fatalf("expected Redis limiter to allow request, got allowed=%v, retry=%d", allowed, retry)
	}
}

