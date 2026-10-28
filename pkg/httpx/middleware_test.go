package httpx

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/fleetpulse/fleetpulse/pkg/auth"
	"github.com/fleetpulse/fleetpulse/pkg/logging"
)

func TestRequestIDMiddleware(t *testing.T) {
	var extractedReqID string
	handler := RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		extractedReqID = logging.RequestIDFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	// Case 1: Client provides X-Request-ID
	req1 := httptest.NewRequest("GET", "/test", nil)
	req1.Header.Set("X-Request-ID", "client-trace-123")
	rec1 := httptest.NewRecorder()
	handler.ServeHTTP(rec1, req1)

	if extractedReqID != "client-trace-123" {
		t.Fatalf("expected client-trace-123, got %s", extractedReqID)
	}
	if rec1.Header().Get("X-Request-ID") != "client-trace-123" {
		t.Fatalf("expected X-Request-ID response header")
	}

	// Case 2: Client provides no header -> generated
	req2 := httptest.NewRequest("GET", "/test", nil)
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req2)

	if extractedReqID == "" || rec2.Header().Get("X-Request-ID") == "" {
		t.Fatalf("expected auto-generated request ID")
	}
}

func TestRecoveryMiddleware(t *testing.T) {
	var logBuf bytes.Buffer
	testLogger := logging.NewWithWriter("test", slog.LevelInfo, &logBuf)

	panicHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("deliberate test panic!")
	})

	ctx := logging.WithLogger(context.Background(), testLogger)
	handler := Recovery(panicHandler)

	req := httptest.NewRequest("GET", "/panic", nil).WithContext(ctx)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 Internal Server Error, got %d", rec.Code)
	}

	var resp ErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode error json: %v", err)
	}
	if resp.Error.Code != "INTERNAL_ERROR" {
		t.Fatalf("expected code INTERNAL_ERROR, got %s", resp.Error.Code)
	}
}

func TestCORSMiddleware(t *testing.T) {
	handler := CORS(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// OPTIONS preflight
	reqOptions := httptest.NewRequest("OPTIONS", "/devices", nil)
	recOptions := httptest.NewRecorder()
	handler.ServeHTTP(recOptions, reqOptions)

	if recOptions.Code != http.StatusNoContent {
		t.Fatalf("expected 204 No Content for OPTIONS, got %d", recOptions.Code)
	}
	if recOptions.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Fatalf("expected CORS origin * header")
	}

	// Normal GET
	reqGet := httptest.NewRequest("GET", "/devices", nil)
	recGet := httptest.NewRecorder()
	handler.ServeHTTP(recGet, reqGet)

	if recGet.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", recGet.Code)
	}
}

func TestAuthAndRoleMiddleware(t *testing.T) {
	tm := auth.NewTokenManager("test-secret-key-32-chars-at-least", "test", 10*time.Minute, 1*time.Hour)

	adminToken, _ := tm.GenerateAccessToken("u-admin", "org-1", auth.RoleAdmin)
	viewerToken, _ := tm.GenerateAccessToken("u-viewer", "org-1", auth.RoleViewer)

	adminEndpoint := Authenticate(tm)(RequireRole(auth.RoleAdmin)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("admin-ok"))
	})))

	// 1. Missing token -> 401
	req1 := httptest.NewRequest("GET", "/admin", nil)
	rec1 := httptest.NewRecorder()
	adminEndpoint.ServeHTTP(rec1, req1)
	if rec1.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for missing token, got %d", rec1.Code)
	}

	// 2. Viewer token on admin endpoint -> 403 Forbidden
	req2 := httptest.NewRequest("GET", "/admin", nil)
	req2.Header.Set("Authorization", "Bearer "+viewerToken)
	rec2 := httptest.NewRecorder()
	adminEndpoint.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for viewer accessing admin endpoint, got %d", rec2.Code)
	}

	// 3. Admin token on admin endpoint -> 200 OK
	req3 := httptest.NewRequest("GET", "/admin", nil)
	req3.Header.Set("Authorization", "Bearer "+adminToken)
	rec3 := httptest.NewRecorder()
	adminEndpoint.ServeHTTP(rec3, req3)
	if rec3.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for admin token, got %d", rec3.Code)
	}
}

func TestLoggerMiddleware(t *testing.T) {
	var buf bytes.Buffer
	testLogger := logging.NewWithWriter("test", slog.LevelInfo, &buf)

	endpoint := Logger(testLogger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"created":true}`))
	}))

	req := httptest.NewRequest("POST", "/api/v1/devices", nil)
	rec := httptest.NewRecorder()
	endpoint.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created, got %d", rec.Code)
	}

	output := buf.String()
	if !strings.Contains(output, `"status":201`) || !strings.Contains(output, `"/api/v1/devices"`) {
		t.Fatalf("expected access log record, got: %s", output)
	}
}
