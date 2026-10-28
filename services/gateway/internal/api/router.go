// Package api implements public REST routing, proxying, and tenant forwarding.
package api

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/fleetpulse/fleetpulse/api/proto/inventoryv1"
	"github.com/fleetpulse/fleetpulse/api/proto/telemetryv1"
	"github.com/fleetpulse/fleetpulse/api/proto/workflowv1"
	"github.com/fleetpulse/fleetpulse/pkg/auth"
	"github.com/fleetpulse/fleetpulse/pkg/httpx"
	"github.com/fleetpulse/fleetpulse/pkg/metrics"
	gwAuth "github.com/fleetpulse/fleetpulse/services/gateway/internal/auth"
	"github.com/fleetpulse/fleetpulse/services/gateway/internal/ratelimit"
	"github.com/go-chi/chi/v5"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Config configures Gateway API dependencies.
type Config struct {
	TokenManager    *auth.TokenManager
	UserStore       *gwAuth.UserStore
	RateLimiter     *ratelimit.Limiter
	Logger          *slog.Logger
	InventoryClient inventoryv1.InventoryServiceClient
	TelemetryClient telemetryv1.TelemetryServiceClient
	WorkflowClient  workflowv1.WorkflowServiceClient
}

// NewRouter constructs Chi HTTP router with complete middleware stack and route mapping.
func NewRouter(cfg Config) http.Handler {
	r := chi.NewRouter()

	// Global Middlewares
	r.Use(httpx.RequestID)
	r.Use(httpx.CORS)
	r.Use(httpx.Recovery)
	if cfg.Logger != nil {
		r.Use(httpx.Logger(cfg.Logger))
	}
	if cfg.RateLimiter != nil {
		r.Use(cfg.RateLimiter.Middleware())
	}

	// Health & Observability
	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		httpx.JSON(w, http.StatusOK, map[string]string{"status": "ok", "service": "gateway"})
	})
	r.Get("/readyz", func(w http.ResponseWriter, r *http.Request) {
		httpx.JSON(w, http.StatusOK, map[string]string{"status": "ready"})
	})
	r.Handle("/metrics", metrics.Handler())

	// Public Auth Endpoints
	r.Post("/api/v1/auth/login", handleLogin(cfg.UserStore, cfg.TokenManager))
	r.Post("/api/v1/auth/refresh", handleRefresh(cfg.TokenManager))

	// Protected Endpoints
	r.Group(func(protected chi.Router) {
		protected.Use(httpx.Authenticate(cfg.TokenManager))

		// Inventory
		protected.Get("/api/v1/devices", handleListDevices(cfg.InventoryClient))
		protected.Get("/api/v1/devices/{id}", handleGetDevice(cfg.InventoryClient))
		protected.With(httpx.RequireRole(auth.RoleAdmin)).Post("/api/v1/devices/claims", handleCreateClaim(cfg.InventoryClient))

		// Telemetry & Alarms
		protected.Get("/api/v1/devices/{id}/telemetry", handleGetDeviceTelemetry(cfg.TelemetryClient))
		protected.Get("/api/v1/alarms", handleListAlarms(cfg.TelemetryClient))
		protected.With(httpx.RequireRole(auth.RoleOperator)).Post("/api/v1/alarms/{id}/acknowledge", handleAcknowledgeAlarm(cfg.TelemetryClient))
		protected.Get("/api/v1/fleet/summary", handleGetFleetSummary(cfg.TelemetryClient))

		// Workflows
		protected.Get("/api/v1/workflows", handleListWorkflows(cfg.WorkflowClient))
		protected.Get("/api/v1/workflows/{id}", handleGetWorkflow(cfg.WorkflowClient))
		protected.With(httpx.RequireRole(auth.RoleOperator)).Post("/api/v1/workflows", handleCreateWorkflow(cfg.WorkflowClient))
		protected.With(httpx.RequireRole(auth.RoleOperator)).Post("/api/v1/workflows/{id}/cancel", handleCancelWorkflow(cfg.WorkflowClient))
	})

	return r
}

func handleLogin(store *gwAuth.UserStore, tm *auth.TokenManager) http.HandlerFunc {
	type LoginRequest struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		var req LoginRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpx.Error(w, r, http.StatusBadRequest, "INVALID_BODY", "malformed request body")
			return
		}

		user, err := store.Authenticate(r.Context(), req.Email, req.Password)
		if err != nil {
			httpx.Error(w, r, http.StatusUnauthorized, "INVALID_CREDENTIALS", "invalid email or password")
			return
		}

		accessToken, err := tm.GenerateAccessToken(user.ID, user.OrgID, user.Role)
		if err != nil {
			httpx.Error(w, r, http.StatusInternalServerError, "TOKEN_ERROR", "failed to issue access token")
			return
		}

		refreshToken, err := tm.GenerateRefreshToken(user.ID, user.OrgID)
		if err != nil {
			httpx.Error(w, r, http.StatusInternalServerError, "TOKEN_ERROR", "failed to issue refresh token")
			return
		}

		httpx.JSON(w, http.StatusOK, map[string]interface{}{
			"access_token":  accessToken,
			"refresh_token": refreshToken,
			"token_type":    "Bearer",
			"user": map[string]string{
				"id":     user.ID,
				"org_id": user.OrgID,
				"email":  user.Email,
				"role":   user.Role,
			},
		})
	}
}

func handleRefresh(tm *auth.TokenManager) http.HandlerFunc {
	type RefreshRequest struct {
		RefreshToken string `json:"refresh_token"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		var req RefreshRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.RefreshToken == "" {
			httpx.Error(w, r, http.StatusBadRequest, "INVALID_BODY", "refresh_token required")
			return
		}

		claims, err := tm.ValidateToken(req.RefreshToken)
		if err != nil || claims.Role != "refresh" {
			httpx.Error(w, r, http.StatusUnauthorized, "INVALID_TOKEN", "invalid or expired refresh token")
			return
		}

		newAccessToken, err := tm.GenerateAccessToken(claims.UserID, claims.OrgID, auth.RoleOperator) // default rank
		if err != nil {
			httpx.Error(w, r, http.StatusInternalServerError, "TOKEN_ERROR", "failed to refresh access token")
			return
		}

		httpx.JSON(w, http.StatusOK, map[string]string{
			"access_token": newAccessToken,
			"token_type":   "Bearer",
		})
	}
}

func handleListDevices(client inventoryv1.InventoryServiceClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if client == nil {
			httpx.Error(w, r, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "inventory service unavailable")
			return
		}

		limit := 20
		if lStr := r.URL.Query().Get("limit"); lStr != "" {
			if l, err := strconv.Atoi(lStr); err == nil && l > 0 {
				limit = l
			}
		}

		req := &inventoryv1.ListDevicesRequest{
			Cursor:          r.URL.Query().Get("cursor"),
			Limit:           int32(limit),
			Model:           r.URL.Query().Get("model"),
			HealthStatus:    r.URL.Query().Get("health"),
			FirmwareVersion: r.URL.Query().Get("firmware"),
			ConnectionState: r.URL.Query().Get("connection"),
		}

		resp, err := client.ListDevices(r.Context(), req)
		if err != nil {
			handleGRPCError(w, r, err)
			return
		}

		httpx.JSON(w, http.StatusOK, map[string]interface{}{
			"devices":     resp.Devices,
			"next_cursor": resp.NextCursor,
			"has_more":    resp.HasMore,
			"total":       resp.TotalCount,
		})
	}
}

func handleGetDevice(client inventoryv1.InventoryServiceClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if client == nil {
			httpx.Error(w, r, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "inventory service unavailable")
			return
		}
		id := chi.URLParam(r, "id")
		resp, err := client.GetDevice(r.Context(), &inventoryv1.GetDeviceRequest{Id: id})
		if err != nil {
			handleGRPCError(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, resp.Device)
	}
}

func handleCreateClaim(client inventoryv1.InventoryServiceClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if client == nil {
			httpx.Error(w, r, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "inventory service unavailable")
			return
		}
		resp, err := client.CreateClaimCode(r.Context(), &inventoryv1.CreateClaimCodeRequest{TtlSeconds: 900})
		if err != nil {
			handleGRPCError(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusCreated, map[string]interface{}{
			"claim_code": resp.ClaimCode,
			"expires_at": resp.ExpiresAtUnix,
		})
	}
}

func handleGetDeviceTelemetry(client telemetryv1.TelemetryServiceClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if client == nil {
			httpx.Error(w, r, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "telemetry service unavailable")
			return
		}
		id := chi.URLParam(r, "id")
		limit := 60
		if lStr := r.URL.Query().Get("limit"); lStr != "" {
			if l, err := strconv.Atoi(lStr); err == nil && l > 0 {
				limit = l
			}
		}
		resp, err := client.GetDeviceTelemetry(r.Context(), &telemetryv1.GetDeviceTelemetryRequest{
			DeviceId: id,
			Limit:    int32(limit),
		})
		if err != nil {
			handleGRPCError(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, resp.Samples)
	}
}

func handleListAlarms(client telemetryv1.TelemetryServiceClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if client == nil {
			httpx.Error(w, r, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "telemetry service unavailable")
			return
		}
		resp, err := client.ListAlarms(r.Context(), &telemetryv1.ListAlarmsRequest{
			DeviceId: r.URL.Query().Get("device_id"),
			Severity: r.URL.Query().Get("severity"),
			Status:   r.URL.Query().Get("status"),
			Limit:    50,
		})
		if err != nil {
			handleGRPCError(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, resp.Alarms)
	}
}

func handleAcknowledgeAlarm(client telemetryv1.TelemetryServiceClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if client == nil {
			httpx.Error(w, r, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "telemetry service unavailable")
			return
		}
		alarmID := chi.URLParam(r, "id")
		resp, err := client.AcknowledgeAlarm(r.Context(), &telemetryv1.AcknowledgeAlarmRequest{
			AlarmId: alarmID,
		})
		if err != nil {
			handleGRPCError(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, resp.Alarm)
	}
}

func handleGetFleetSummary(client telemetryv1.TelemetryServiceClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if client == nil {
			httpx.Error(w, r, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "telemetry service unavailable")
			return
		}
		resp, err := client.GetFleetSummary(r.Context(), &telemetryv1.GetFleetSummaryRequest{})
		if err != nil {
			handleGRPCError(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, map[string]interface{}{
			"health":        resp.Health,
			"top_anomalies": resp.TopAnomalies,
		})
	}
}

func handleListWorkflows(client workflowv1.WorkflowServiceClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if client == nil {
			httpx.Error(w, r, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "workflow service unavailable")
			return
		}
		resp, err := client.ListJobs(r.Context(), &workflowv1.ListJobsRequest{
			State: r.URL.Query().Get("state"),
			Limit: 50,
		})
		if err != nil {
			handleGRPCError(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, resp.Jobs)
	}
}

func handleGetWorkflow(client workflowv1.WorkflowServiceClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if client == nil {
			httpx.Error(w, r, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "workflow service unavailable")
			return
		}
		id := chi.URLParam(r, "id")
		resp, err := client.GetJob(r.Context(), &workflowv1.GetJobRequest{Id: id})
		if err != nil {
			handleGRPCError(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, resp.Job)
	}
}

func handleCreateWorkflow(client workflowv1.WorkflowServiceClient) http.HandlerFunc {
	type WorkflowCreatePayload struct {
		Type           string            `json:"type"`
		Priority       int               `json:"priority"`
		Targets        []string          `json:"targets"`
		IdempotencyKey string            `json:"idempotency_key"`
		Parameters     map[string]string `json:"parameters"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		if client == nil {
			httpx.Error(w, r, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "workflow service unavailable")
			return
		}
		var payload WorkflowCreatePayload
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			httpx.Error(w, r, http.StatusBadRequest, "INVALID_BODY", "malformed request payload")
			return
		}
		if payload.Type == "" || payload.IdempotencyKey == "" || len(payload.Targets) == 0 {
			httpx.Error(w, r, http.StatusBadRequest, "VALIDATION_FAILED", "type, idempotency_key, and targets are required")
			return
		}

		resp, err := client.CreateJob(r.Context(), &workflowv1.CreateJobRequest{
			Type:           payload.Type,
			Priority:       int32(payload.Priority),
			Targets:        payload.Targets,
			Parameters:     payload.Parameters,
			IdempotencyKey: payload.IdempotencyKey,
		})
		if err != nil {
			handleGRPCError(w, r, err)
			return
		}

		httpx.JSON(w, http.StatusCreated, resp.Job)
	}
}

func handleCancelWorkflow(client workflowv1.WorkflowServiceClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if client == nil {
			httpx.Error(w, r, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "workflow service unavailable")
			return
		}
		id := chi.URLParam(r, "id")
		resp, err := client.CancelJob(r.Context(), &workflowv1.CancelJobRequest{Id: id})
		if err != nil {
			handleGRPCError(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, map[string]interface{}{
			"success": resp.Success,
			"message": resp.Message,
		})
	}
}

func handleGRPCError(w http.ResponseWriter, r *http.Request, err error) {
	st, ok := status.FromError(err)
	if !ok {
		httpx.Error(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	switch st.Code() {
	case codes.NotFound:
		httpx.Error(w, r, http.StatusNotFound, "NOT_FOUND", st.Message())
	case codes.InvalidArgument:
		httpx.Error(w, r, http.StatusBadRequest, "BAD_REQUEST", st.Message())
	case codes.Unauthenticated:
		httpx.Error(w, r, http.StatusUnauthorized, "UNAUTHENTICATED", st.Message())
	case codes.PermissionDenied:
		httpx.Error(w, r, http.StatusForbidden, "FORBIDDEN", st.Message())
	default:
		httpx.Error(w, r, http.StatusInternalServerError, fmt.Sprintf("GRPC_%s", st.Code().String()), st.Message())
	}
}
