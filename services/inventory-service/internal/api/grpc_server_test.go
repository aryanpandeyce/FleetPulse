package api

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/fleetpulse/fleetpulse/api/proto/inventoryv1"
	"github.com/fleetpulse/fleetpulse/pkg/logging"
	"github.com/fleetpulse/fleetpulse/services/inventory-service/internal/domain"
	"github.com/fleetpulse/fleetpulse/services/inventory-service/internal/service"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// mockRepo satisfies domain.Repository for gRPC server tests.
type mockRepo struct {
	devices     map[string]*domain.Device
	claims      map[string]*domain.DeviceClaim
	profiles    map[string]*domain.ServerProfile
	assignments map[string]string // deviceID -> profileID
	compliances []*domain.ComplianceResult
	failNext    bool
}

func newMockRepo() *mockRepo {
	return &mockRepo{
		devices:     make(map[string]*domain.Device),
		claims:      make(map[string]*domain.DeviceClaim),
		profiles:    make(map[string]*domain.ServerProfile),
		assignments: make(map[string]string),
	}
}

func (m *mockRepo) GetDeviceByID(ctx context.Context, orgID, id string) (*domain.Device, error) {
	if m.failNext {
		return nil, errors.New("db error")
	}
	d, ok := m.devices[id]
	if !ok || d.OrgID != orgID {
		return nil, domain.ErrDeviceNotFound
	}
	return d, nil
}

func (m *mockRepo) GetDeviceBySerial(ctx context.Context, serial string) (*domain.Device, error) {
	if m.failNext {
		return nil, errors.New("db error")
	}
	for _, d := range m.devices {
		if d.Serial == serial {
			return d, nil
		}
	}
	return nil, domain.ErrDeviceNotFound
}

func (m *mockRepo) ListDevices(ctx context.Context, filter domain.DeviceFilter) ([]domain.Device, bool, int64, error) {
	if m.failNext {
		return nil, false, 0, errors.New("db error")
	}
	out := make([]domain.Device, 0)
	for _, d := range m.devices {
		if d.OrgID == filter.OrgID {
			out = append(out, *d)
		}
	}
	return out, false, int64(len(out)), nil
}

func (m *mockRepo) UpsertDevice(ctx context.Context, device *domain.Device) error {
	m.devices[device.ID] = device
	return nil
}

func (m *mockRepo) CreateClaimCode(ctx context.Context, claim *domain.DeviceClaim) error {
	if m.failNext {
		return errors.New("db error")
	}
	m.claims[claim.ClaimCode] = claim
	return nil
}

func (m *mockRepo) GetClaimCode(ctx context.Context, claimCode string) (*domain.DeviceClaim, error) {
	c, ok := m.claims[claimCode]
	if !ok {
		return nil, domain.ErrClaimCodeInvalid
	}
	return c, nil
}

func (m *mockRepo) MarkClaimUsed(ctx context.Context, claimCode, serial string) error {
	c, ok := m.claims[claimCode]
	if !ok {
		return domain.ErrClaimCodeInvalid
	}
	c.Status = "claimed"
	c.Serial = serial
	return nil
}

func (m *mockRepo) GetProfile(ctx context.Context, orgID, profileID string) (*domain.ServerProfile, error) {
	p, ok := m.profiles[profileID]
	if !ok || p.OrgID != orgID {
		return nil, domain.ErrProfileNotFound
	}
	return p, nil
}

func (m *mockRepo) GetDeviceProfile(ctx context.Context, orgID, deviceID string) (*domain.ServerProfile, error) {
	profID, ok := m.assignments[deviceID]
	if !ok {
		return nil, nil
	}
	return m.GetProfile(ctx, orgID, profID)
}

func (m *mockRepo) AssignProfile(ctx context.Context, orgID, deviceID, profileID string) error {
	m.assignments[deviceID] = profileID
	return nil
}

func (m *mockRepo) RecordCompliance(ctx context.Context, cr *domain.ComplianceResult) error {
	m.compliances = append(m.compliances, cr)
	return nil
}

func TestInventoryGRPCServer(t *testing.T) {
	repo := newMockRepo()
	svc := service.New(repo, nil, 10)
	server := NewServer(svc)

	orgID := "org-acme"
	ctx := logging.WithOrgID(context.Background(), orgID)

	// Seed a device
	dev := &domain.Device{
		ID:              "dev-1",
		OrgID:           orgID,
		Serial:          "UCS-001",
		Model:           "UCS-C220-M7",
		FirmwareVersion: "4.2(3a)",
		PowerState:      "on",
		HealthStatus:    "ok",
		ConnectionState: "connected",
		Tags:            map[string]string{"rack": "R01"},
		ClaimedAt:       time.Now(),
		LastSeenAt:      time.Now(),
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}
	_ = repo.UpsertDevice(ctx, dev)

	// 1. GetDevice by ID
	resp, err := server.GetDevice(ctx, &inventoryv1.GetDeviceRequest{Id: "dev-1"})
	if err != nil || resp.Device.Serial != "UCS-001" {
		t.Fatalf("unexpected GetDevice result: %v, %+v", err, resp)
	}

	// 2. GetDevice by Serial
	respSerial, err := server.GetDevice(ctx, &inventoryv1.GetDeviceRequest{Serial: "UCS-001"})
	if err != nil || respSerial.Device.Id != "dev-1" {
		t.Fatalf("unexpected GetDevice by serial result: %v", err)
	}

	// 3. GetDevice Missing Context -> Unauthenticated
	_, err = server.GetDevice(context.Background(), &inventoryv1.GetDeviceRequest{Id: "dev-1"})
	if status.Code(err) != codes.Unauthenticated {
		t.Fatalf("expected Unauthenticated, got %v", err)
	}

	// 4. GetDevice Missing Params -> InvalidArgument
	_, err = server.GetDevice(ctx, &inventoryv1.GetDeviceRequest{})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("expected InvalidArgument, got %v", err)
	}

	// 5. GetDevice Not Found -> NotFound
	_, err = server.GetDevice(ctx, &inventoryv1.GetDeviceRequest{Id: "missing"})
	if status.Code(err) != codes.NotFound {
		t.Fatalf("expected NotFound, got %v", err)
	}

	// 6. ListDevices
	listResp, err := server.ListDevices(ctx, &inventoryv1.ListDevicesRequest{Limit: 10})
	if err != nil || len(listResp.Devices) != 1 {
		t.Fatalf("unexpected ListDevices: %v, %+v", err, listResp)
	}

	// 6b. ListDevices Missing Context -> Unauthenticated
	_, err = server.ListDevices(context.Background(), &inventoryv1.ListDevicesRequest{})
	if status.Code(err) != codes.Unauthenticated {
		t.Fatalf("expected Unauthenticated, got %v", err)
	}

	// 7. CreateClaimCode
	claimResp, err := server.CreateClaimCode(ctx, &inventoryv1.CreateClaimCodeRequest{TtlSeconds: 3600})
	if err != nil || claimResp.ClaimCode == "" {
		t.Fatalf("unexpected CreateClaimCode: %v, %+v", err, claimResp)
	}

	// 7b. CreateClaimCode Missing Context -> Unauthenticated
	_, err = server.CreateClaimCode(context.Background(), &inventoryv1.CreateClaimCodeRequest{})
	if status.Code(err) != codes.Unauthenticated {
		t.Fatalf("expected Unauthenticated, got %v", err)
	}

	// 8. ClaimDevice
	claimDevResp, err := server.ClaimDevice(ctx, &inventoryv1.ClaimDeviceRequest{
		ClaimCode:       claimResp.ClaimCode,
		Serial:          "UCS-CLAIMED-1",
		Model:           "UCS-C240-M7",
		InitialFirmware: "4.2(3a)",
	})
	if err != nil || claimDevResp.Device.Serial != "UCS-CLAIMED-1" {
		t.Fatalf("unexpected ClaimDevice: %v, %+v", err, claimDevResp)
	}

	// 8b. ClaimDevice missing args
	_, err = server.ClaimDevice(ctx, &inventoryv1.ClaimDeviceRequest{ClaimCode: ""})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("expected InvalidArgument, got %v", err)
	}

	// 8c. ClaimDevice invalid claim code
	_, err = server.ClaimDevice(ctx, &inventoryv1.ClaimDeviceRequest{
		ClaimCode: "NONEXISTENT",
		Serial:    "S1",
		Model:     "M1",
	})
	if status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("expected FailedPrecondition, got %v", err)
	}

	// 9. ServerProfile & Compliance
	profile := &domain.ServerProfile{
		ID:             "prof-1",
		OrgID:          orgID,
		Name:           "Golden",
		TargetFirmware: "4.2(3a)",
		PowerPolicy:    "performance",
		BootOrder:      []string{"nvme"},
	}
	repo.profiles["prof-1"] = profile
	repo.assignments["dev-1"] = "prof-1"

	profResp, err := server.GetServerProfile(ctx, &inventoryv1.GetServerProfileRequest{Id: "prof-1"})
	if err != nil || profResp.Profile.Name != "Golden" {
		t.Fatalf("unexpected GetServerProfile: %v, %+v", err, profResp)
	}

	// 9b. GetServerProfile missing ctx / id / not found
	_, err = server.GetServerProfile(context.Background(), &inventoryv1.GetServerProfileRequest{Id: "prof-1"})
	if status.Code(err) != codes.Unauthenticated {
		t.Fatalf("expected Unauthenticated, got %v", err)
	}
	_, err = server.GetServerProfile(ctx, &inventoryv1.GetServerProfileRequest{Id: ""})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("expected InvalidArgument, got %v", err)
	}
	_, err = server.GetServerProfile(ctx, &inventoryv1.GetServerProfileRequest{Id: "unknown"})
	if status.Code(err) != codes.NotFound {
		t.Fatalf("expected NotFound, got %v", err)
	}

	// 10. EvaluateCompliance
	compResp, err := server.EvaluateCompliance(ctx, &inventoryv1.EvaluateComplianceRequest{
		DeviceId: "dev-1",
	})
	if err != nil || compResp.Result.Status != "compliant" {
		t.Fatalf("unexpected EvaluateCompliance: %v, %+v", err, compResp)
	}

	// 10b. EvaluateCompliance errors
	_, err = server.EvaluateCompliance(context.Background(), &inventoryv1.EvaluateComplianceRequest{DeviceId: "dev-1"})
	if status.Code(err) != codes.Unauthenticated {
		t.Fatalf("expected Unauthenticated, got %v", err)
	}
	_, err = server.EvaluateCompliance(ctx, &inventoryv1.EvaluateComplianceRequest{DeviceId: ""})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("expected InvalidArgument, got %v", err)
	}
	_, err = server.EvaluateCompliance(ctx, &inventoryv1.EvaluateComplianceRequest{DeviceId: "unknown"})
	if status.Code(err) != codes.NotFound {
		t.Fatalf("expected NotFound, got %v", err)
	}
}
