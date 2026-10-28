package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/fleetpulse/fleetpulse/api/proto/inventoryv1"
	"github.com/fleetpulse/fleetpulse/api/proto/telemetryv1"
	"github.com/fleetpulse/fleetpulse/api/proto/workflowv1"
	"github.com/fleetpulse/fleetpulse/pkg/auth"
	"github.com/fleetpulse/fleetpulse/pkg/logging"
	gwAuth "github.com/fleetpulse/fleetpulse/services/gateway/internal/auth"
	"github.com/fleetpulse/fleetpulse/services/gateway/internal/ratelimit"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// mockInventoryClient verifies tenant isolation and satisfies InventoryServiceClient interface.
type mockInventoryClient struct {
	lastContext context.Context
}

func (m *mockInventoryClient) GetDevice(ctx context.Context, in *inventoryv1.GetDeviceRequest, opts ...grpc.CallOption) (*inventoryv1.GetDeviceResponse, error) {
	m.lastContext = ctx
	orgID := logging.OrgIDFromContext(ctx)
	if in.Id == "missing-id" {
		return nil, status.Error(codes.NotFound, "device not found")
	}
	return &inventoryv1.GetDeviceResponse{
		Device: &inventoryv1.Device{
			Id:     in.Id,
			OrgId:  orgID,
			Serial: "UCS-MOCK-1",
		},
	}, nil
}

func (m *mockInventoryClient) ListDevices(ctx context.Context, in *inventoryv1.ListDevicesRequest, opts ...grpc.CallOption) (*inventoryv1.ListDevicesResponse, error) {
	m.lastContext = ctx
	orgID := logging.OrgIDFromContext(ctx)
	return &inventoryv1.ListDevicesResponse{
		Devices: []*inventoryv1.Device{
			{Id: "dev-1", OrgId: orgID, Serial: "UCS-1"},
		},
		TotalCount: 1,
	}, nil
}

func (m *mockInventoryClient) CreateClaimCode(ctx context.Context, in *inventoryv1.CreateClaimCodeRequest, opts ...grpc.CallOption) (*inventoryv1.CreateClaimCodeResponse, error) {
	m.lastContext = ctx
	return &inventoryv1.CreateClaimCodeResponse{
		ClaimCode:     "FP-TEST-CLAIM",
		ExpiresAtUnix: time.Now().Add(15 * time.Minute).Unix(),
	}, nil
}

func (m *mockInventoryClient) ClaimDevice(ctx context.Context, in *inventoryv1.ClaimDeviceRequest, opts ...grpc.CallOption) (*inventoryv1.ClaimDeviceResponse, error) {
	return nil, nil
}

func (m *mockInventoryClient) EvaluateCompliance(ctx context.Context, in *inventoryv1.EvaluateComplianceRequest, opts ...grpc.CallOption) (*inventoryv1.EvaluateComplianceResponse, error) {
	return nil, nil
}

func (m *mockInventoryClient) GetServerProfile(ctx context.Context, in *inventoryv1.GetServerProfileRequest, opts ...grpc.CallOption) (*inventoryv1.GetServerProfileResponse, error) {
	return nil, nil
}

// mockTelemetryClient satisfies TelemetryServiceClient interface.
type mockTelemetryClient struct{}

func (m *mockTelemetryClient) GetDeviceTelemetry(ctx context.Context, in *telemetryv1.GetDeviceTelemetryRequest, opts ...grpc.CallOption) (*telemetryv1.GetDeviceTelemetryResponse, error) {
	return &telemetryv1.GetDeviceTelemetryResponse{
		Samples: []*telemetryv1.TelemetrySample{
			{DeviceId: in.DeviceId, InletTempCelsius: 22.5, CpuTempCelsius: 48.0},
		},
	}, nil
}

func (m *mockTelemetryClient) ListAlarms(ctx context.Context, in *telemetryv1.ListAlarmsRequest, opts ...grpc.CallOption) (*telemetryv1.ListAlarmsResponse, error) {
	return &telemetryv1.ListAlarmsResponse{
		Alarms: []*telemetryv1.Alarm{
			{Id: "alm-1", RuleName: "HighTemp", Severity: "warning", Status: "raised"},
		},
	}, nil
}

func (m *mockTelemetryClient) AcknowledgeAlarm(ctx context.Context, in *telemetryv1.AcknowledgeAlarmRequest, opts ...grpc.CallOption) (*telemetryv1.AcknowledgeAlarmResponse, error) {
	return &telemetryv1.AcknowledgeAlarmResponse{
		Success: true,
		Alarm: &telemetryv1.Alarm{
			Id: in.AlarmId, Status: "acknowledged",
		},
	}, nil
}

func (m *mockTelemetryClient) GetFleetSummary(ctx context.Context, in *telemetryv1.GetFleetSummaryRequest, opts ...grpc.CallOption) (*telemetryv1.GetFleetSummaryResponse, error) {
	return &telemetryv1.GetFleetSummaryResponse{
		Health: &telemetryv1.FleetHealthCount{
			OkCount: 980, WarningCount: 15, CriticalCount: 5, TotalDevices: 1000,
		},
	}, nil
}

// mockWorkflowClient satisfies WorkflowServiceClient interface.
type mockWorkflowClient struct{}

func (m *mockWorkflowClient) CreateJob(ctx context.Context, in *workflowv1.CreateJobRequest, opts ...grpc.CallOption) (*workflowv1.CreateJobResponse, error) {
	return &workflowv1.CreateJobResponse{
		Job: &workflowv1.Job{
			Id:             "job-new-123",
			Type:           in.Type,
			State:          "queued",
			Priority:       in.Priority,
			IdempotencyKey: in.IdempotencyKey,
			Targets:        in.Targets,
		},
	}, nil
}

func (m *mockWorkflowClient) GetJob(ctx context.Context, in *workflowv1.GetJobRequest, opts ...grpc.CallOption) (*workflowv1.GetJobResponse, error) {
	return &workflowv1.GetJobResponse{
		Job: &workflowv1.Job{
			Id: in.Id, Type: "firmware_upgrade", State: "running",
		},
	}, nil
}

func (m *mockWorkflowClient) CancelJob(ctx context.Context, in *workflowv1.CancelJobRequest, opts ...grpc.CallOption) (*workflowv1.CancelJobResponse, error) {
	return &workflowv1.CancelJobResponse{
		Success: true, Message: "job cancelled",
	}, nil
}

func (m *mockWorkflowClient) ListJobs(ctx context.Context, in *workflowv1.ListJobsRequest, opts ...grpc.CallOption) (*workflowv1.ListJobsResponse, error) {
	return &workflowv1.ListJobsResponse{
		Jobs: []*workflowv1.Job{
			{Id: "job-1", Type: "firmware_upgrade", State: "succeeded"},
		},
	}, nil
}

func setupTestRouter(rateLimitBurst float64) (http.Handler, *auth.TokenManager, *mockInventoryClient) {
	tm := auth.NewTokenManager("gateway-test-secret-key-32-chars", "gateway-test", 10*time.Minute, 1*time.Hour)
	userStore := gwAuth.NewSeededUserStore()
	invClient := &mockInventoryClient{}
	telClient := &mockTelemetryClient{}
	wfClient := &mockWorkflowClient{}
	limiter := ratelimit.New(nil, rateLimitBurst, 1.0) // In-memory rate limiter

	router := NewRouter(Config{
		TokenManager:    tm,
		UserStore:       userStore,
		RateLimiter:     limiter,
		InventoryClient: invClient,
		TelemetryClient: telClient,
		WorkflowClient:  wfClient,
	})
	return router, tm, invClient
}

func TestGatewayAuthentication(t *testing.T) {
	router, tm, _ := setupTestRouter(100.0)

	// 1. Valid login for Acme Admin
	body := `{"email":"admin@acme.com","password":"AdminPass123!"}`
	req := httptest.NewRequest("POST", "/api/v1/auth/login", bytes.NewBufferString(body))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for valid login, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp["access_token"] == nil || resp["token_type"] != "Bearer" {
		t.Fatalf("missing access_token in login response: %+v", resp)
	}

	// Test Refresh Token
	refreshToken, _ := tm.GenerateRefreshToken("usr-acme-1", "org-acme")
	refreshBody := fmt.Sprintf(`{"refresh_token":"%s"}`, refreshToken)
	refReq := httptest.NewRequest("POST", "/api/v1/auth/refresh", bytes.NewBufferString(refreshBody))
	refRec := httptest.NewRecorder()
	router.ServeHTTP(refRec, refReq)

	if refRec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for valid refresh, got %d", refRec.Code)
	}

	// 2. Invalid password
	badBody := `{"email":"admin@acme.com","password":"WrongPassword!"}`
	badReq := httptest.NewRequest("POST", "/api/v1/auth/login", bytes.NewBufferString(badBody))
	badRec := httptest.NewRecorder()
	router.ServeHTTP(badRec, badReq)

	if badRec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized for bad password, got %d", badRec.Code)
	}
}

func TestTenantIsolation(t *testing.T) {
	router, tm, invClient := setupTestRouter(100.0)

	// Acme user token (tenant: c827b56a-1234-4b55-a22b-8a89201f92e1)
	acmeOrgID := "c827b56a-1234-4b55-a22b-8a89201f92e1"
	token, _ := tm.GenerateAccessToken("usr-acme-1", acmeOrgID, auth.RoleOperator)

	req := httptest.NewRequest("GET", "/api/v1/devices", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rec.Code)
	}

	// Assert that the downstream call carried strictly the Acme org_id from JWT context!
	forwardedOrg := logging.OrgIDFromContext(invClient.lastContext)
	if forwardedOrg != acmeOrgID {
		t.Fatalf("TENANT LEAK: expected downstream call to carry %s, but got %s", acmeOrgID, forwardedOrg)
	}

	// Test GET device by ID
	reqDev := httptest.NewRequest("GET", "/api/v1/devices/dev-1", nil)
	reqDev.Header.Set("Authorization", "Bearer "+token)
	recDev := httptest.NewRecorder()
	router.ServeHTTP(recDev, reqDev)
	if recDev.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for get device, got %d", recDev.Code)
	}

	// Test Not Found
	reqMissing := httptest.NewRequest("GET", "/api/v1/devices/missing-id", nil)
	reqMissing.Header.Set("Authorization", "Bearer "+token)
	recMissing := httptest.NewRecorder()
	router.ServeHTTP(recMissing, reqMissing)
	if recMissing.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for missing device, got %d", recMissing.Code)
	}
}

func TestRBACEnforcement(t *testing.T) {
	router, tm, _ := setupTestRouter(100.0)

	viewerToken, _ := tm.GenerateAccessToken("u-viewer", "org-1", auth.RoleViewer)
	adminToken, _ := tm.GenerateAccessToken("u-admin", "org-1", auth.RoleAdmin)

	// Viewer attempts to create claim code (Admin only) -> 403 Forbidden
	req1 := httptest.NewRequest("POST", "/api/v1/devices/claims", nil)
	req1.Header.Set("Authorization", "Bearer "+viewerToken)
	rec1 := httptest.NewRecorder()
	router.ServeHTTP(rec1, req1)

	if rec1.Code != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden for viewer on admin endpoint, got %d", rec1.Code)
	}

	// Admin creates claim code -> 201 Created
	req2 := httptest.NewRequest("POST", "/api/v1/devices/claims", nil)
	req2.Header.Set("Authorization", "Bearer "+adminToken)
	rec2 := httptest.NewRecorder()
	router.ServeHTTP(rec2, req2)

	if rec2.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created for admin on claim endpoint, got %d", rec2.Code)
	}
}

func TestTelemetryAndWorkflowEndpoints(t *testing.T) {
	router, tm, _ := setupTestRouter(100.0)
	token, _ := tm.GenerateAccessToken("u-op", "org-1", auth.RoleOperator)

	// 1. Telemetry Samples
	reqTel := httptest.NewRequest("GET", "/api/v1/devices/dev-1/telemetry", nil)
	reqTel.Header.Set("Authorization", "Bearer "+token)
	recTel := httptest.NewRecorder()
	router.ServeHTTP(recTel, reqTel)
	if recTel.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for telemetry, got %d", recTel.Code)
	}

	// 2. Alarms list & acknowledge
	reqAlarms := httptest.NewRequest("GET", "/api/v1/alarms", nil)
	reqAlarms.Header.Set("Authorization", "Bearer "+token)
	recAlarms := httptest.NewRecorder()
	router.ServeHTTP(recAlarms, reqAlarms)
	if recAlarms.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for alarms list, got %d", recAlarms.Code)
	}

	reqAck := httptest.NewRequest("POST", "/api/v1/alarms/alm-1/acknowledge", nil)
	reqAck.Header.Set("Authorization", "Bearer "+token)
	recAck := httptest.NewRecorder()
	router.ServeHTTP(recAck, reqAck)
	if recAck.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for alarm acknowledge, got %d", recAck.Code)
	}

	// 3. Fleet Summary
	reqSum := httptest.NewRequest("GET", "/api/v1/fleet/summary", nil)
	reqSum.Header.Set("Authorization", "Bearer "+token)
	recSum := httptest.NewRecorder()
	router.ServeHTTP(recSum, reqSum)
	if recSum.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for fleet summary, got %d", recSum.Code)
	}

	// 4. Workflows List, Create, Get, Cancel
	reqWfList := httptest.NewRequest("GET", "/api/v1/workflows", nil)
	reqWfList.Header.Set("Authorization", "Bearer "+token)
	recWfList := httptest.NewRecorder()
	router.ServeHTTP(recWfList, reqWfList)
	if recWfList.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for workflow list, got %d", recWfList.Code)
	}

	wfPayload := `{"type":"firmware_upgrade","priority":1,"targets":["dev-1"],"idempotency_key":"idem-test-1"}`
	reqWfCreate := httptest.NewRequest("POST", "/api/v1/workflows", bytes.NewBufferString(wfPayload))
	reqWfCreate.Header.Set("Authorization", "Bearer "+token)
	recWfCreate := httptest.NewRecorder()
	router.ServeHTTP(recWfCreate, reqWfCreate)
	if recWfCreate.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created for workflow create, got %d: %s", recWfCreate.Code, recWfCreate.Body.String())
	}

	reqWfGet := httptest.NewRequest("GET", "/api/v1/workflows/job-new-123", nil)
	reqWfGet.Header.Set("Authorization", "Bearer "+token)
	recWfGet := httptest.NewRecorder()
	router.ServeHTTP(recWfGet, reqWfGet)
	if recWfGet.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for workflow get, got %d", recWfGet.Code)
	}

	reqWfCancel := httptest.NewRequest("POST", "/api/v1/workflows/job-new-123/cancel", nil)
	reqWfCancel.Header.Set("Authorization", "Bearer "+token)
	recWfCancel := httptest.NewRecorder()
	router.ServeHTTP(recWfCancel, reqWfCancel)
	if recWfCancel.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for workflow cancel, got %d", recWfCancel.Code)
	}
}

func TestRateLimiterHTTP429(t *testing.T) {
	// Burst capacity of 3
	router, _, _ := setupTestRouter(3.0)

	// Send 3 requests (all allowed)
	for i := 0; i < 3; i++ {
		req := httptest.NewRequest("GET", "/healthz", nil)
		req.Header.Set("X-Forwarded-For", "192.168.1.50")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("request %d rejected with status %d", i+1, rec.Code)
		}
	}

	// 4th request must be rejected with 429 Too Many Requests
	reqExceed := httptest.NewRequest("GET", "/healthz", nil)
	reqExceed.Header.Set("X-Forwarded-For", "192.168.1.50")
	recExceed := httptest.NewRecorder()
	router.ServeHTTP(recExceed, reqExceed)

	if recExceed.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429 Too Many Requests, got %d", recExceed.Code)
	}
	retryAfter := recExceed.Header().Get("Retry-After")
	if retryAfter == "" {
		t.Fatalf("expected Retry-After header on 429 response")
	}
}

func TestHealthObservabilityAndErrors(t *testing.T) {
	router, tm, _ := setupTestRouter(100.0)
	token, _ := tm.GenerateAccessToken("u-op", "org-1", auth.RoleOperator)

	// 1. Healthz, Readyz, Metrics
	for _, path := range []string{"/healthz", "/readyz", "/metrics"} {
		req := httptest.NewRequest("GET", path, nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 for %s, got %d", path, rec.Code)
		}
	}

	// 2. Malformed JSON bodies
	badLoginReq := httptest.NewRequest("POST", "/api/v1/auth/login", bytes.NewBufferString("{invalid-json"))
	badLoginRec := httptest.NewRecorder()
	router.ServeHTTP(badLoginRec, badLoginReq)
	if badLoginRec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for bad login json, got %d", badLoginRec.Code)
	}

	badRefreshReq := httptest.NewRequest("POST", "/api/v1/auth/refresh", bytes.NewBufferString("{invalid-json"))
	badRefreshRec := httptest.NewRecorder()
	router.ServeHTTP(badRefreshRec, badRefreshReq)
	if badRefreshRec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for bad refresh json, got %d", badRefreshRec.Code)
	}

	badWfReq := httptest.NewRequest("POST", "/api/v1/workflows", bytes.NewBufferString("{invalid-json"))
	badWfReq.Header.Set("Authorization", "Bearer "+token)
	badWfRec := httptest.NewRecorder()
	router.ServeHTTP(badWfRec, badWfReq)
	if badWfRec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for bad workflow json, got %d", badWfRec.Code)
	}

	// 3. Query params coverage (filtering / pagination)
	reqFilter := httptest.NewRequest("GET", "/api/v1/devices?cursor=cur123&limit=25&model=UCS-C220-M7&health=OK", nil)
	reqFilter.Header.Set("Authorization", "Bearer "+token)
	recFilter := httptest.NewRecorder()
	router.ServeHTTP(recFilter, reqFilter)
	if recFilter.Code != http.StatusOK {
		t.Fatalf("expected 200 for filtered list devices, got %d", recFilter.Code)
	}

	reqAlarmsFilter := httptest.NewRequest("GET", "/api/v1/alarms?status=raised&severity=critical&device_id=dev-1", nil)
	reqAlarmsFilter.Header.Set("Authorization", "Bearer "+token)
	recAlarmsFilter := httptest.NewRecorder()
	router.ServeHTTP(recAlarmsFilter, reqAlarmsFilter)
	if recAlarmsFilter.Code != http.StatusOK {
		t.Fatalf("expected 200 for filtered list alarms, got %d", recAlarmsFilter.Code)
	}

	reqWfFilter := httptest.NewRequest("GET", "/api/v1/workflows?state=running&type=firmware_upgrade&target_id=dev-1", nil)
	reqWfFilter.Header.Set("Authorization", "Bearer "+token)
	recWfFilter := httptest.NewRecorder()
	router.ServeHTTP(recWfFilter, reqWfFilter)
	if recWfFilter.Code != http.StatusOK {
		t.Fatalf("expected 200 for filtered list workflows, got %d", recWfFilter.Code)
	}

	// 4. Missing required fields in workflow create
	emptyWfReq := httptest.NewRequest("POST", "/api/v1/workflows", bytes.NewBufferString(`{"type":""}`))
	emptyWfReq.Header.Set("Authorization", "Bearer "+token)
	emptyWfRec := httptest.NewRecorder()
	router.ServeHTTP(emptyWfRec, emptyWfReq)
	if emptyWfRec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for empty workflow fields, got %d", emptyWfRec.Code)
	}
}

func TestNilClientsAndServiceUnavailable(t *testing.T) {
	tm := auth.NewTokenManager("test-secret-at-least-32-bytes-long!", "gateway-test", 15*time.Minute, 7*24*time.Hour)
	token, _ := tm.GenerateAccessToken("u-op", "org-1", auth.RoleOperator)

	// Router with nil clients
	r := NewRouter(Config{
		TokenManager: tm,
		UserStore:    gwAuth.NewSeededUserStore(),
	})

	for _, path := range []string{"/api/v1/devices", "/api/v1/devices/dev-1", "/api/v1/devices/123/telemetry", "/api/v1/alarms", "/api/v1/fleet/summary", "/api/v1/workflows", "/api/v1/workflows/job-1"} {
		req := httptest.NewRequest("GET", path, nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		if rec.Code != http.StatusServiceUnavailable {
			t.Fatalf("expected 503 Service Unavailable for %s with nil client, got %d", path, rec.Code)
		}
	}

	// Post routes with nil clients
	adminToken, _ := tm.GenerateAccessToken("u-admin", "org-1", auth.RoleAdmin)
	claimReq := httptest.NewRequest("POST", "/api/v1/devices/claims", bytes.NewBufferString(`{"ttl_seconds":300}`))
	claimReq.Header.Set("Authorization", "Bearer "+adminToken)
	claimRec := httptest.NewRecorder()
	r.ServeHTTP(claimRec, claimReq)
	if claimRec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 for create claim with nil client, got %d", claimRec.Code)
	}

	ackReq := httptest.NewRequest("POST", "/api/v1/alarms/alm-1/acknowledge", nil)
	ackReq.Header.Set("Authorization", "Bearer "+token)
	ackRec := httptest.NewRecorder()
	r.ServeHTTP(ackRec, ackReq)
	if ackRec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 for ack alarm with nil client, got %d", ackRec.Code)
	}

	cancelReq := httptest.NewRequest("POST", "/api/v1/workflows/job-1/cancel", nil)
	cancelReq.Header.Set("Authorization", "Bearer "+token)
	cancelRec := httptest.NewRecorder()
	r.ServeHTTP(cancelRec, cancelReq)
	if cancelRec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 for cancel workflow with nil client, got %d", cancelRec.Code)
	}

	createWfReq := httptest.NewRequest("POST", "/api/v1/workflows", bytes.NewBufferString(`{"type":"firmware_upgrade","targets":["dev-1"]}`))
	createWfReq.Header.Set("Authorization", "Bearer "+token)
	createWfRec := httptest.NewRecorder()
	r.ServeHTTP(createWfRec, createWfReq)
	if createWfRec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 for create workflow with nil client, got %d", createWfRec.Code)
	}
}

func TestHandleGRPCErrorMappings(t *testing.T) {
	testCases := []struct {
		err          error
		expectedCode int
	}{
		{errors.New("plain non-grpc error"), http.StatusInternalServerError},
		{status.Error(codes.NotFound, "not found"), http.StatusNotFound},
		{status.Error(codes.InvalidArgument, "invalid argument"), http.StatusBadRequest},
		{status.Error(codes.Unauthenticated, "unauthenticated"), http.StatusUnauthorized},
		{status.Error(codes.PermissionDenied, "permission denied"), http.StatusForbidden},
		{status.Error(codes.Unavailable, "unavailable"), http.StatusInternalServerError},
	}

	for _, tc := range testCases {
		req := httptest.NewRequest("GET", "/test", nil)
		rec := httptest.NewRecorder()
		handleGRPCError(rec, req, tc.err)
		if rec.Code != tc.expectedCode {
			t.Fatalf("expected code %d for error %v, got %d", tc.expectedCode, tc.err, rec.Code)
		}
	}
}


