// Package grpcx provides gRPC server and client interceptors enforcing tenant isolation,
// context metadata propagation, and structured request logging.
package grpcx

import (
	"context"
	"log/slog"
	"time"

	"github.com/fleetpulse/fleetpulse/pkg/auth"
	"github.com/fleetpulse/fleetpulse/pkg/logging"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

const (
	HeaderOrgID     = "x-org-id"
	HeaderRequestID = "x-request-id"
	HeaderUserID    = "x-user-id"
	HeaderRole      = "x-role"
)

// UnaryServerTenantInterceptor extracts tenant identity, user, and request ID from incoming gRPC metadata.
func UnaryServerTenantInterceptor() grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {
		md, ok := metadata.FromIncomingContext(ctx)
		if !ok {
			return nil, status.Error(codes.Unauthenticated, "missing gRPC metadata")
		}

		orgIDs := md.Get(HeaderOrgID)
		if len(orgIDs) == 0 || orgIDs[0] == "" {
			return nil, status.Error(codes.Unauthenticated, "missing tenant header (x-org-id)")
		}
		orgID := orgIDs[0]

		reqID := ""
		if reqIDs := md.Get(HeaderRequestID); len(reqIDs) > 0 {
			reqID = reqIDs[0]
		}

		userID := ""
		if userIDs := md.Get(HeaderUserID); len(userIDs) > 0 {
			userID = userIDs[0]
		}

		role := auth.RoleViewer
		if roles := md.Get(HeaderRole); len(roles) > 0 && roles[0] != "" {
			role = roles[0]
		}

		// Inject into context
		ctx = auth.WithAuthContext(ctx, userID, orgID, role)
		ctx = logging.WithOrgID(ctx, orgID)
		if reqID != "" {
			ctx = logging.WithRequestID(ctx, reqID)
		}
		if userID != "" {
			ctx = logging.WithUserID(ctx, userID)
		}

		return handler(ctx, req)
	}
}

// UnaryClientTenantInterceptor automatically injects context org_id and request_id into outbound gRPC metadata.
func UnaryClientTenantInterceptor() grpc.UnaryClientInterceptor {
	return func(
		ctx context.Context,
		method string,
		req, reply interface{},
		cc *grpc.ClientConn,
		invoker grpc.UnaryInvoker,
		opts ...grpc.CallOption,
	) error {
		pairs := make([]string, 0, 8)

		if orgID := logging.OrgIDFromContext(ctx); orgID != "" {
			pairs = append(pairs, HeaderOrgID, orgID)
		}
		if reqID := logging.RequestIDFromContext(ctx); reqID != "" {
			pairs = append(pairs, HeaderRequestID, reqID)
		}
		if userID := logging.UserIDFromContext(ctx); userID != "" {
			pairs = append(pairs, HeaderUserID, userID)
		}
		if role := auth.RoleFromContext(ctx); role != "" {
			pairs = append(pairs, HeaderRole, role)
		}

		if len(pairs) > 0 {
			ctx = metadata.AppendToOutgoingContext(ctx, pairs...)
		}

		return invoker(ctx, method, req, reply, cc, opts...)
	}
}

// UnaryServerLoggingInterceptor logs incoming RPC executions with duration and tenant context.
func UnaryServerLoggingInterceptor(logger *slog.Logger) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {
		start := time.Now()
		resp, err := handler(ctx, req)
		duration := time.Since(start)

		statusCode := codes.OK
		if err != nil {
			statusCode = status.Code(err)
		}

		logger.InfoContext(ctx, "grpc_call",
			slog.String("method", info.FullMethod),
			slog.String("code", statusCode.String()),
			slog.Duration("duration", duration),
		)

		return resp, err
	}
}
