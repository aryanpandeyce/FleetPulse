// Package logging provides structured JSON logging using Go standard library's log/slog.
// It integrates context-propagated tracing attributes (request_id, org_id, user_id)
// and enforces standardized field conventions across all FleetPulse microservices.
package logging

import (
	"context"
	"io"
	"log/slog"
	"os"
)

type contextKey string

const (
	loggerKey    contextKey = "fp_logger"
	requestIDKey contextKey = "fp_request_id"
	orgIDKey     contextKey = "fp_org_id"
	userIDKey    contextKey = "fp_user_id"
)

// ContextHandler wraps an slog.Handler to automatically inject context keys into every log entry.
type ContextHandler struct {
	slog.Handler
}

// Handle appends context values (request_id, org_id, user_id) if present in the context.
func (h *ContextHandler) Handle(ctx context.Context, r slog.Record) error {
	if ctx != nil {
		if reqID, ok := ctx.Value(requestIDKey).(string); ok && reqID != "" {
			r.AddAttrs(slog.String("request_id", reqID))
		}
		if orgID, ok := ctx.Value(orgIDKey).(string); ok && orgID != "" {
			r.AddAttrs(slog.String("org_id", orgID))
		}
		if userID, ok := ctx.Value(userIDKey).(string); ok && userID != "" {
			r.AddAttrs(slog.String("user_id", userID))
		}
	}
	return h.Handler.Handle(ctx, r)
}

// WithAttrs returns a new ContextHandler with the given attributes appended to the inner handler.
func (h *ContextHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &ContextHandler{Handler: h.Handler.WithAttrs(attrs)}
}

// WithGroup returns a new ContextHandler with the inner handler grouped.
func (h *ContextHandler) WithGroup(name string) slog.Handler {
	return &ContextHandler{Handler: h.Handler.WithGroup(name)}
}

// New creates a structured JSON logger for a named service writing to stdout.
func New(serviceName string, level slog.Level) *slog.Logger {
	return NewWithWriter(serviceName, level, os.Stdout)
}

// NewWithWriter allows redirecting log output (useful for tests or custom outputs).
func NewWithWriter(serviceName string, level slog.Level, w io.Writer) *slog.Logger {
	baseHandler := slog.NewJSONHandler(w, &slog.HandlerOptions{
		Level: level,
	})
	ctxHandler := &ContextHandler{Handler: baseHandler}
	return slog.New(ctxHandler).With(slog.String("service", serviceName))
}

// WithRequestID returns a new context carrying the given request ID.
func WithRequestID(ctx context.Context, requestID string) context.Context {
	return context.WithValue(ctx, requestIDKey, requestID)
}

// RequestIDFromContext retrieves the request ID from context or returns an empty string.
func RequestIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if v, ok := ctx.Value(requestIDKey).(string); ok {
		return v
	}
	return ""
}

// WithOrgID returns a new context carrying the tenant/org ID.
func WithOrgID(ctx context.Context, orgID string) context.Context {
	return context.WithValue(ctx, orgIDKey, orgID)
}

// OrgIDFromContext retrieves the tenant/org ID from context.
func OrgIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if v, ok := ctx.Value(orgIDKey).(string); ok {
		return v
	}
	return ""
}

// WithUserID returns a new context carrying the user ID.
func WithUserID(ctx context.Context, userID string) context.Context {
	return context.WithValue(ctx, userIDKey, userID)
}

// UserIDFromContext retrieves the user ID from context.
func UserIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if v, ok := ctx.Value(userIDKey).(string); ok {
		return v
	}
	return ""
}

// WithLogger stores an slog.Logger instance in the context.
func WithLogger(ctx context.Context, logger *slog.Logger) context.Context {
	return context.WithValue(ctx, loggerKey, logger)
}

// FromContext extracts the slog.Logger from context, falling back to default logger if missing.
func FromContext(ctx context.Context) *slog.Logger {
	if ctx != nil {
		if l, ok := ctx.Value(loggerKey).(*slog.Logger); ok && l != nil {
			return l
		}
	}
	return slog.Default()
}
