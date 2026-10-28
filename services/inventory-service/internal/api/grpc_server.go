// Package api implements the gRPC server transport for InventoryService.
package api

import (
	"context"
	"time"

	"github.com/fleetpulse/fleetpulse/api/proto/inventoryv1"
	"github.com/fleetpulse/fleetpulse/pkg/logging"
	"github.com/fleetpulse/fleetpulse/services/inventory-service/internal/domain"
	"github.com/fleetpulse/fleetpulse/services/inventory-service/internal/service"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Server implements inventoryv1.InventoryServiceServer.
type Server struct {
	inventoryv1.UnimplementedInventoryServiceServer
	svc *service.Service
}

// NewServer creates a new inventory gRPC server.
func NewServer(svc *service.Service) *Server {
	return &Server{svc: svc}
}

// GetDevice handles retrieval of a device by ID or serial number.
func (s *Server) GetDevice(ctx context.Context, req *inventoryv1.GetDeviceRequest) (*inventoryv1.GetDeviceResponse, error) {
	orgID := logging.OrgIDFromContext(ctx)
	if orgID == "" {
		return nil, status.Error(codes.Unauthenticated, "missing tenant context")
	}

	target := req.Id
	if target == "" {
		target = req.Serial
	}
	if target == "" {
		return nil, status.Error(codes.InvalidArgument, "either id or serial must be provided")
	}

	device, err := s.svc.GetDevice(ctx, orgID, target)
	if err != nil {
		if err == domain.ErrDeviceNotFound {
			return nil, status.Error(codes.NotFound, "device not found")
		}
		return nil, status.Errorf(codes.Internal, "failed to get device: %v", err)
	}

	return &inventoryv1.GetDeviceResponse{
		Device: toProtoDevice(device),
	}, nil
}

// ListDevices handles paginated queries over devices.
func (s *Server) ListDevices(ctx context.Context, req *inventoryv1.ListDevicesRequest) (*inventoryv1.ListDevicesResponse, error) {
	orgID := logging.OrgIDFromContext(ctx)
	if orgID == "" {
		return nil, status.Error(codes.Unauthenticated, "missing tenant context")
	}

	filter := domain.DeviceFilter{
		OrgID:           orgID,
		Limit:           int(req.Limit),
		Model:           req.Model,
		HealthStatus:    req.HealthStatus,
		FirmwareVersion: req.FirmwareVersion,
		ConnectionState: req.ConnectionState,
	}

	devices, nextCursor, hasMore, totalCount, err := s.svc.ListDevices(ctx, filter, req.Cursor)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list devices: %v", err)
	}

	protoDevices := make([]*inventoryv1.Device, len(devices))
	for i, d := range devices {
		protoDevices[i] = toProtoDevice(&d)
	}

	return &inventoryv1.ListDevicesResponse{
		Devices:    protoDevices,
		NextCursor: nextCursor,
		HasMore:    hasMore,
		TotalCount: totalCount,
	}, nil
}

// CreateClaimCode issues a new single-use token.
func (s *Server) CreateClaimCode(ctx context.Context, req *inventoryv1.CreateClaimCodeRequest) (*inventoryv1.CreateClaimCodeResponse, error) {
	orgID := logging.OrgIDFromContext(ctx)
	if orgID == "" {
		return nil, status.Error(codes.Unauthenticated, "missing tenant context")
	}

	ttl := time.Duration(req.TtlSeconds) * time.Second
	claim, err := s.svc.CreateClaimCode(ctx, orgID, ttl)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to create claim code: %v", err)
	}

	return &inventoryv1.CreateClaimCodeResponse{
		ClaimCode:     claim.ClaimCode,
		ExpiresAtUnix: claim.ExpiresAt.Unix(),
	}, nil
}

// ClaimDevice registers a device using a claim code.
func (s *Server) ClaimDevice(ctx context.Context, req *inventoryv1.ClaimDeviceRequest) (*inventoryv1.ClaimDeviceResponse, error) {
	if req.ClaimCode == "" || req.Serial == "" || req.Model == "" {
		return nil, status.Error(codes.InvalidArgument, "claim_code, serial, and model are required")
	}

	device, err := s.svc.ClaimDevice(ctx, req.ClaimCode, req.Serial, req.Model, req.InitialFirmware)
	if err != nil {
		if err == domain.ErrClaimCodeInvalid || err == domain.ErrClaimCodeExpired {
			return nil, status.Errorf(codes.FailedPrecondition, "%v", err)
		}
		return nil, status.Errorf(codes.Internal, "claim failed: %v", err)
	}

	return &inventoryv1.ClaimDeviceResponse{
		Success: true,
		Device:  toProtoDevice(device),
		Message: "device successfully registered and bound to tenant",
	}, nil
}

// EvaluateCompliance audits device against server profile.
func (s *Server) EvaluateCompliance(ctx context.Context, req *inventoryv1.EvaluateComplianceRequest) (*inventoryv1.EvaluateComplianceResponse, error) {
	orgID := logging.OrgIDFromContext(ctx)
	if orgID == "" {
		return nil, status.Error(codes.Unauthenticated, "missing tenant context")
	}
	if req.DeviceId == "" {
		return nil, status.Error(codes.InvalidArgument, "device_id is required")
	}

	res, err := s.svc.EvaluateCompliance(ctx, orgID, req.DeviceId)
	if err != nil {
		if err == domain.ErrDeviceNotFound || err == domain.ErrProfileNotFound {
			return nil, status.Errorf(codes.NotFound, "%v", err)
		}
		return nil, status.Errorf(codes.Internal, "compliance check failed: %v", err)
	}

	protoDrifts := make([]*inventoryv1.DriftItem, len(res.DriftDetails))
	for i, d := range res.DriftDetails {
		protoDrifts[i] = &inventoryv1.DriftItem{
			Component:     d.Component,
			ActualValue:   d.ActualValue,
			ExpectedValue: d.ExpectedValue,
		}
	}

	return &inventoryv1.EvaluateComplianceResponse{
		Result: &inventoryv1.ComplianceResult{
			Id:               res.ID,
			DeviceId:         res.DeviceID,
			ProfileId:        res.ProfileID,
			Status:           res.Status,
			DriftDetails:     protoDrifts,
			EvaluatedAtUnix: res.EvaluatedAt.Unix(),
		},
	}, nil
}

func toProtoDevice(d *domain.Device) *inventoryv1.Device {
	if d == nil {
		return nil
	}
	return &inventoryv1.Device{
		Id:              d.ID,
		OrgId:           d.OrgID,
		Serial:          d.Serial,
		Model:           d.Model,
		FirmwareVersion: d.FirmwareVersion,
		PowerState:      d.PowerState,
		HealthStatus:    d.HealthStatus,
		ConnectionState: d.ConnectionState,
		Tags:            d.Tags,
		Version:         d.Version,
		ClaimedAtUnix:   d.ClaimedAt.Unix(),
		LastSeenAtUnix:  d.LastSeenAt.Unix(),
	}
}
