package grpcx

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"

	"github.com/fleetpulse/fleetpulse/pkg/auth"
	"github.com/fleetpulse/fleetpulse/pkg/logging"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func TestUnaryServerTenantInterceptor(t *testing.T) {
	interceptor := UnaryServerTenantInterceptor()

	// 1. Missing metadata entirely
	_, err := interceptor(context.Background(), "req", &grpc.UnaryServerInfo{}, func(ctx context.Context, req interface{}) (interface{}, error) {
		return "ok", nil
	})
	if status.Code(err) != codes.Unauthenticated {
		t.Fatalf("expected Unauthenticated for missing metadata, got %v", status.Code(err))
	}

	// 2. Metadata present but missing x-org-id
	mdEmpty := metadata.Pairs(HeaderRequestID, "r-123")
	ctxWithEmptyMD := metadata.NewIncomingContext(context.Background(), mdEmpty)
	_, err = interceptor(ctxWithEmptyMD, "req", &grpc.UnaryServerInfo{}, func(ctx context.Context, req interface{}) (interface{}, error) {
		return "ok", nil
	})
	if status.Code(err) != codes.Unauthenticated {
		t.Fatalf("expected Unauthenticated for missing x-org-id, got %v", status.Code(err))
	}

	// 3. Valid metadata with org_id and request_id
	mdValid := metadata.Pairs(
		HeaderOrgID, "org-acme-corp",
		HeaderRequestID, "req-xyz-987",
		HeaderUserID, "usr-bob-111",
		HeaderRole, auth.RoleOperator,
	)
	ctxValid := metadata.NewIncomingContext(context.Background(), mdValid)

	var capturedOrg, capturedReq, capturedUser, capturedRole string
	resp, err := interceptor(ctxValid, "req", &grpc.UnaryServerInfo{}, func(ctx context.Context, req interface{}) (interface{}, error) {
		capturedOrg = logging.OrgIDFromContext(ctx)
		capturedReq = logging.RequestIDFromContext(ctx)
		capturedUser = logging.UserIDFromContext(ctx)
		capturedRole = auth.RoleFromContext(ctx)
		return "success", nil
	})

	if err != nil || resp != "success" {
		t.Fatalf("interceptor failed on valid metadata: %v", err)
	}
	if capturedOrg != "org-acme-corp" || capturedReq != "req-xyz-987" || capturedUser != "usr-bob-111" || capturedRole != auth.RoleOperator {
		t.Fatalf("context values not extracted properly: org=%s, req=%s, user=%s, role=%s",
			capturedOrg, capturedReq, capturedUser, capturedRole)
	}
}

func TestUnaryClientTenantInterceptor(t *testing.T) {
	clientInterceptor := UnaryClientTenantInterceptor()

	ctx := context.Background()
	ctx = logging.WithOrgID(ctx, "org-tenant-1")
	ctx = logging.WithRequestID(ctx, "req-trace-456")
	ctx = logging.WithUserID(ctx, "usr-alice-222")
	ctx = auth.WithAuthContext(ctx, "usr-alice-222", "org-tenant-1", auth.RoleAdmin)

	var capturedMD metadata.MD
	invoker := func(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, opts ...grpc.CallOption) error {
		capturedMD, _ = metadata.FromOutgoingContext(ctx)
		return nil
	}

	err := clientInterceptor(ctx, "/inventory.InventoryService/GetDevice", nil, nil, nil, invoker)
	if err != nil {
		t.Fatalf("client interceptor returned error: %v", err)
	}

	if orgs := capturedMD.Get(HeaderOrgID); len(orgs) == 0 || orgs[0] != "org-tenant-1" {
		t.Fatalf("expected x-org-id in outgoing metadata, got: %v", orgs)
	}
	if reqs := capturedMD.Get(HeaderRequestID); len(reqs) == 0 || reqs[0] != "req-trace-456" {
		t.Fatalf("expected x-request-id in outgoing metadata, got: %v", reqs)
	}
	if users := capturedMD.Get(HeaderUserID); len(users) == 0 || users[0] != "usr-alice-222" {
		t.Fatalf("expected x-user-id in outgoing metadata, got: %v", users)
	}
	if roles := capturedMD.Get(HeaderRole); len(roles) == 0 || roles[0] != auth.RoleAdmin {
		t.Fatalf("expected x-role in outgoing metadata, got: %v", roles)
	}
}

func TestUnaryServerLoggingInterceptor(t *testing.T) {
	var buf bytes.Buffer
	logger := logging.NewWithWriter("grpc-test", slog.LevelInfo, &buf)
	loggingInterceptor := UnaryServerLoggingInterceptor(logger)

	info := &grpc.UnaryServerInfo{FullMethod: "/telemetry.TelemetryService/GetFleetSummary"}
	_, err := loggingInterceptor(context.Background(), nil, info, func(ctx context.Context, req interface{}) (interface{}, error) {
		return "summary", nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, `"/telemetry.TelemetryService/GetFleetSummary"`) || !strings.Contains(output, `"code":"OK"`) {
		t.Fatalf("expected gRPC log record, got: %s", output)
	}
}
