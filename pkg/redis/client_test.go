package redis

import (
	"context"
	"errors"
	"net"
	"strings"
	"testing"
	"time"
)

// mockRedisServer provides a minimal RESP responder for testing.
func startMockRedis(t *testing.T) (net.Listener, func()) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start mock redis: %v", err)
	}

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				buf := make([]byte, 1024)
				for {
					n, err := c.Read(buf)
					if err != nil {
						return
					}
					msg := string(buf[:n])
					if strings.Contains(msg, "PING") {
						_, _ = c.Write([]byte("+PONG\r\n"))
					} else if strings.Contains(msg, "LOAD") {
						_, _ = c.Write([]byte("$40\r\n726c07153a56e09c8f3074d2843bb369b775463f\r\n"))
					} else if strings.Contains(msg, "EVALSHA") {
						_, _ = c.Write([]byte("*2\r\n:1\r\n:0\r\n"))
					} else if strings.Contains(msg, "ERRTEST") {
						_, _ = c.Write([]byte("-ERR unknown error\r\n"))
					} else if strings.Contains(msg, "NILTEST") {
						_, _ = c.Write([]byte("$-1\r\n"))
					} else if strings.Contains(msg, "NILARRTEST") {
						_, _ = c.Write([]byte("*-1\r\n"))
					} else if strings.Contains(msg, "INTTEST") {
						_, _ = c.Write([]byte(":42\r\n"))
					} else if strings.Contains(msg, "BADTEST") {
						_, _ = c.Write([]byte("?unknown\r\n"))
					} else {
						_, _ = c.Write([]byte("+OK\r\n"))
					}
				}
			}(conn)
		}
	}()

	return ln, func() { _ = ln.Close() }
}

func TestRedisClientPing(t *testing.T) {
	ln, cleanup := startMockRedis(t)
	defer cleanup()

	client := NewClient("redis://" + ln.Addr().String())
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := client.Ping(ctx); err != nil {
		t.Fatalf("ping failed: %v", err)
	}
}

func TestRedisClientScriptLoadAndEvalSha(t *testing.T) {
	ln, cleanup := startMockRedis(t)
	defer cleanup()

	client := NewClient(ln.Addr().String())
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	sha, err := client.ScriptLoad(ctx, "return 1")
	if err != nil {
		t.Fatalf("script load failed: %v", err)
	}
	if sha != "726c07153a56e09c8f3074d2843bb369b775463f" {
		t.Fatalf("unexpected sha: %s", sha)
	}

	res, err := client.EvalSha(ctx, sha, []string{"key1"}, 100, 20)
	if err != nil {
		t.Fatalf("evalsha failed: %v", err)
	}

	arr, ok := res.([]interface{})
	if !ok || len(arr) != 2 || arr[0].(int64) != 1 {
		t.Fatalf("unexpected evalsha result: %+v", res)
	}
}

func TestRedisClientErrorCases(t *testing.T) {
	ln, cleanup := startMockRedis(t)
	defer cleanup()

	client := NewClient(ln.Addr().String())
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Error response
	_, err := client.Do(ctx, "ERRTEST")
	if err == nil || !strings.Contains(err.Error(), "unknown error") {
		t.Fatalf("expected error from ERRTEST, got %v", err)
	}

	// Nil bulk string
	_, err = client.Do(ctx, "NILTEST")
	if !errors.Is(err, ErrNilReply) {
		t.Fatalf("expected ErrNilReply, got %v", err)
	}

	// Nil array
	_, err = client.Do(ctx, "NILARRTEST")
	if !errors.Is(err, ErrNilReply) {
		t.Fatalf("expected ErrNilReply, got %v", err)
	}

	// Integer parsing
	val, err := client.Do(ctx, "INTTEST")
	if err != nil || val.(int64) != 42 {
		t.Fatalf("expected integer 42, got %v, err: %v", val, err)
	}

	// Bad prefix
	_, err = client.Do(ctx, "BADTEST")
	if err == nil || !strings.Contains(err.Error(), "unknown RESP prefix") {
		t.Fatalf("expected unknown prefix error, got %v", err)
	}

	// Close client
	if err := client.Close(); err != nil {
		t.Fatalf("failed to close client: %v", err)
	}

	// Call after close
	_, err = client.Do(ctx, "PING")
	if !errors.Is(err, ErrClosed) {
		t.Fatalf("expected ErrClosed, got %v", err)
	}
}

func TestRedisClientConnectionFailure(t *testing.T) {
	// Connect to non-existent port
	client := NewClient("127.0.0.1:54321")
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	err := client.Ping(ctx)
	if err == nil {
		t.Fatal("expected connection failure, got nil")
	}
}
