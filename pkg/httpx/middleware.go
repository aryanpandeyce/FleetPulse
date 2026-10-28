// Package httpx provides standard HTTP middlewares, response formatting utilities,
// and context decorators for FleetPulse HTTP services.
package httpx

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"
	"strings"
	"time"

	"github.com/fleetpulse/fleetpulse/pkg/auth"
	"github.com/fleetpulse/fleetpulse/pkg/logging"
	"github.com/google/uuid"
)

// ResponseEnvelope provides standard envelope structure for successful API responses.
type ResponseEnvelope struct {
	Data interface{} `json:"data,omitempty"`
	Meta interface{} `json:"meta,omitempty"`
}

// ErrorResponse provides RFC-7807 inspired standard error response structure.
type ErrorResponse struct {
	Error ErrorDetails `json:"error"`
}

// ErrorDetails contains machine-readable and human-readable error metadata.
type ErrorDetails struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"request_id,omitempty"`
}

// JSON serializes the given data as application/json and writes the status code.
func JSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

// Error writes a structured JSON error response.
func Error(w http.ResponseWriter, r *http.Request, status int, code, message string) {
	reqID := ""
	if r != nil {
		reqID = logging.RequestIDFromContext(r.Context())
	}
	JSON(w, status, ErrorResponse{
		Error: ErrorDetails{
			Code:      code,
			Message:   message,
			RequestID: reqID,
		},
	})
}

// RequestID middleware extracts or generates a unique X-Request-ID and embeds it into context.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqID := r.Header.Get("X-Request-ID")
		if reqID == "" {
			reqID = uuid.NewString()
		}
		w.Header().Set("X-Request-ID", reqID)
		ctx := logging.WithRequestID(r.Context(), reqID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// Recovery middleware catches unhandled panics, logs stack traces, and returns HTTP 500.
func Recovery(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				stack := string(debug.Stack())
				logger := logging.FromContext(r.Context())
				logger.ErrorContext(r.Context(), "panic recovered in HTTP handler",
					slog.Any("panic", rec),
					slog.String("stack", stack),
				)
				Error(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "an unexpected server error occurred")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// CORS middleware configures permissive cross-origin resource sharing headers for browser frontends.
func CORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Accept, Authorization, Content-Type, X-Request-ID")
		w.Header().Set("Access-Control-Expose-Headers", "X-Request-ID, Content-Length")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// Logger middleware logs incoming HTTP request details and duration upon completion.
func Logger(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			wrapped := &statusResponseWriter{ResponseWriter: w, statusCode: http.StatusOK}
			next.ServeHTTP(wrapped, r)
			duration := time.Since(start)

			logger.InfoContext(r.Context(), "http_request",
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.Int("status", wrapped.statusCode),
				slog.Duration("duration", duration),
				slog.String("ip", r.RemoteAddr),
			)
		})
	}
}

// Authenticate middleware validates Bearer token and injects claims into context.
func Authenticate(tm *auth.TokenManager) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
				Error(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "missing or malformed authorization token")
				return
			}

			tokenStr := strings.TrimPrefix(authHeader, "Bearer ")
			claims, err := tm.ValidateToken(tokenStr)
			if err != nil {
				Error(w, r, http.StatusUnauthorized, "UNAUTHORIZED", fmt.Sprintf("invalid token: %v", err))
				return
			}

			// Embed user, org, and role in context
			ctx := auth.WithAuthContext(r.Context(), claims.UserID, claims.OrgID, claims.Role)
			ctx = logging.WithOrgID(ctx, claims.OrgID)
			ctx = logging.WithUserID(ctx, claims.UserID)

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequireRole middleware enforces that the authenticated user meets the minimum role rank.
func RequireRole(requiredRole string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			userRole := auth.RoleFromContext(r.Context())
			if userRole == "" || !auth.HasRole(userRole, requiredRole) {
				Error(w, r, http.StatusForbidden, "FORBIDDEN", fmt.Sprintf("role %s required", requiredRole))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// statusResponseWriter captures HTTP status code for access logging.
type statusResponseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *statusResponseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}
