package logging

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
)

func TestLoggingContextPropagation(t *testing.T) {
	var buf bytes.Buffer
	logger := NewWithWriter("test-service", slog.LevelInfo, &buf)

	ctx := context.Background()
	ctx = WithRequestID(ctx, "req-12345")
	ctx = WithOrgID(ctx, "org-abcde")
	ctx = WithUserID(ctx, "usr-99999")

	logger.InfoContext(ctx, "processing payload", slog.String("action", "test"))

	output := buf.String()
	if !strings.Contains(output, `"service":"test-service"`) {
		t.Fatalf("expected service tag in json output: %s", output)
	}
	if !strings.Contains(output, `"request_id":"req-12345"`) {
		t.Fatalf("expected request_id in json output: %s", output)
	}
	if !strings.Contains(output, `"org_id":"org-abcde"`) {
		t.Fatalf("expected org_id in json output: %s", output)
	}
	if !strings.Contains(output, `"user_id":"usr-99999"`) {
		t.Fatalf("expected user_id in json output: %s", output)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(output), &parsed); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
	if parsed["msg"] != "processing payload" {
		t.Fatalf("expected message 'processing payload', got %v", parsed["msg"])
	}
}

func TestContextHelpers(t *testing.T) {
	ctx := context.Background()
	if RequestIDFromContext(ctx) != "" {
		t.Fatalf("expected empty request id from clean context")
	}
	if OrgIDFromContext(ctx) != "" {
		t.Fatalf("expected empty org id from clean context")
	}
	if UserIDFromContext(ctx) != "" {
		t.Fatalf("expected empty user id from clean context")
	}

	ctx = WithRequestID(ctx, "r-1")
	ctx = WithOrgID(ctx, "o-1")
	ctx = WithUserID(ctx, "u-1")

	if RequestIDFromContext(ctx) != "r-1" || OrgIDFromContext(ctx) != "o-1" || UserIDFromContext(ctx) != "u-1" {
		t.Fatalf("context extraction failed: r=%s, o=%s, u=%s",
			RequestIDFromContext(ctx), OrgIDFromContext(ctx), UserIDFromContext(ctx))
	}

	logger := New("custom", slog.LevelDebug)
	ctx = WithLogger(ctx, logger)
	if FromContext(ctx) != logger {
		t.Fatalf("expected stored logger from context")
	}
	if FromContext(nil) == nil {
		t.Fatalf("expected fallback to default logger on nil context")
	}
}
