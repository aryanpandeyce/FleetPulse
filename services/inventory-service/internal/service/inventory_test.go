package service

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/fleetpulse/fleetpulse/services/inventory-service/internal/domain"
)

// mockRepo provides thread-safe in-memory mock implementation of domain.Repository.
type mockRepo struct {
	mu           sync.RWMutex
	devices      map[string]*domain.Device
	claims       map[string]*domain.DeviceClaim
	profiles     map[string]*domain.ServerProfile
	assignments  map[string]string // device_id -> profile_id
	compliances  []*domain.ComplianceResult
	getByIdCalls int
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
	m.mu.Lock()
	m.getByIdCalls++
	m.mu.Unlock()

	m.mu.RLock()
	defer m.mu.RUnlock()
	d, ok := m.devices[id]
	if !ok || d.OrgID != orgID {
		return nil, domain.ErrDeviceNotFound
	}
	return d, nil
}

func (m *mockRepo) GetDeviceBySerial(ctx context.Context, serial string) (*domain.Device, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, d := range m.devices {
		if d.Serial == serial {
			return d, nil
		}
	}
	return nil, domain.ErrDeviceNotFound
}

func (m *mockRepo) ListDevices(ctx context.Context, filter domain.DeviceFilter) ([]domain.Device, bool, int64, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var res []domain.Device
	for _, d := range m.devices {
		if d.OrgID == filter.OrgID {
			res = append(res, *d)
		}
	}
	hasMore := len(res) > filter.Limit
	if hasMore {
		res = res[:filter.Limit]
	}
	return res, hasMore, int64(len(m.devices)), nil
}

func (m *mockRepo) UpsertDevice(ctx context.Context, d *domain.Device) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.devices[d.ID] = d
	return nil
}

func (m *mockRepo) CreateClaimCode(ctx context.Context, claim *domain.DeviceClaim) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.claims[claim.ClaimCode] = claim
	return nil
}

func (m *mockRepo) GetClaimCode(ctx context.Context, code string) (*domain.DeviceClaim, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	c, ok := m.claims[code]
	if !ok {
		return nil, domain.ErrClaimCodeInvalid
	}
	return c, nil
}

func (m *mockRepo) MarkClaimUsed(ctx context.Context, code, serial string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	c, ok := m.claims[code]
	if !ok || c.Status != "pending" {
		return domain.ErrClaimCodeInvalid
	}
	c.Status = "claimed"
	c.Serial = serial
	return nil
}

func (m *mockRepo) GetProfile(ctx context.Context, orgID, profileID string) (*domain.ServerProfile, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	p, ok := m.profiles[profileID]
	if !ok || p.OrgID != orgID {
		return nil, domain.ErrProfileNotFound
	}
	return p, nil
}

func (m *mockRepo) GetDeviceProfile(ctx context.Context, orgID, deviceID string) (*domain.ServerProfile, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	profileID, ok := m.assignments[deviceID]
	if !ok {
		return nil, nil
	}
	return m.GetProfile(ctx, orgID, profileID)
}

func (m *mockRepo) AssignProfile(ctx context.Context, orgID, deviceID, profileID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.assignments[deviceID] = profileID
	return nil
}

func (m *mockRepo) RecordCompliance(ctx context.Context, cr *domain.ComplianceResult) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.compliances = append(m.compliances, cr)
	return nil
}

// mockPublisher records emitted NATS events
type mockPublisher struct {
	mu     sync.Mutex
	events map[string][]string
}

func newMockPublisher() *mockPublisher {
	return &mockPublisher{events: make(map[string][]string)}
}

func (p *mockPublisher) Publish(subject string, data []byte) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.events[subject] = append(p.events[subject], string(data))
	return nil
}

func TestClaimFlowAndSingleUse(t *testing.T) {
	repo := newMockRepo()
	events := newMockPublisher()
	svc := New(repo, events, 100)

	ctx := context.Background()
	orgID := "org-alpha-123"

	// 1. Generate claim code
	claim, err := svc.CreateClaimCode(ctx, orgID, 10*time.Minute)
	if err != nil {
		t.Fatalf("failed to create claim code: %v", err)
	}

	// 2. Submit claim code from simulator
	device, err := svc.ClaimDevice(ctx, claim.ClaimCode, "UCS-SN-998811", "UCS-C220-M7", "4.2(3b)")
	if err != nil {
		t.Fatalf("device claim failed: %v", err)
	}

	if device.OrgID != orgID || device.Serial != "UCS-SN-998811" {
		t.Fatalf("device properties incorrect: %+v", device)
	}

	// 3. Attempt to re-use same claim code (must be rejected - single use)
	_, err = svc.ClaimDevice(ctx, claim.ClaimCode, "UCS-SN-ANOTHER", "UCS-C220-M7", "4.2(3b)")
	if err != domain.ErrClaimCodeInvalid {
		t.Fatalf("expected ErrClaimCodeInvalid on re-using single-use code, got %v", err)
	}

	// 4. Verify NATS event emitted
	if len(events.events["inventory.device.claimed"]) != 1 {
		t.Fatalf("expected 1 'inventory.device.claimed' event, got %d", len(events.events["inventory.device.claimed"]))
	}
}

func TestClaimCodeExpiration(t *testing.T) {
	repo := newMockRepo()
	svc := New(repo, nil, 100)
	ctx := context.Background()

	// Create claim that expired in past
	claim, _ := svc.CreateClaimCode(ctx, "org-1", 1*time.Millisecond)
	time.Sleep(10 * time.Millisecond)

	_, err := svc.ClaimDevice(ctx, claim.ClaimCode, "SN-001", "UCS-C240-M7", "5.0")
	if err != domain.ErrClaimCodeExpired {
		t.Fatalf("expected ErrClaimCodeExpired, got %v", err)
	}
}

func TestLRUCacheAcceleration(t *testing.T) {
	repo := newMockRepo()
	svc := New(repo, nil, 100)
	ctx := context.Background()
	orgID := "org-cache-test"

	dev := &domain.Device{
		ID:        "dev-123",
		OrgID:     orgID,
		Serial:    "SN-HOT-CACHE",
		Model:     "UCS-X210c",
		CreatedAt: time.Now(),
	}
	_ = repo.UpsertDevice(ctx, dev)

	// First lookup -> queries repository
	d1, err := svc.GetDevice(ctx, orgID, "dev-123")
	if err != nil || d1.Serial != "SN-HOT-CACHE" {
		t.Fatalf("first lookup failed: %v", err)
	}
	if repo.getByIdCalls != 1 {
		t.Fatalf("expected 1 repo call, got %d", repo.getByIdCalls)
	}

	// Second lookup -> must be served directly from LRU cache without hitting repository!
	d2, err := svc.GetDevice(ctx, orgID, "dev-123")
	if err != nil || d2.Serial != "SN-HOT-CACHE" {
		t.Fatalf("second lookup failed: %v", err)
	}
	if repo.getByIdCalls != 1 {
		t.Fatalf("expected LRU cache hit to prevent second DB query! repo calls: %d", repo.getByIdCalls)
	}

	// Lookup by serial
	d3, err := svc.GetDevice(ctx, orgID, "SN-HOT-CACHE")
	if err != nil || d3.ID != "dev-123" {
		t.Fatalf("lookup by serial failed: %v", err)
	}

	// Non-existent device
	_, err = svc.GetDevice(ctx, orgID, "non-existent")
	if err != domain.ErrDeviceNotFound {
		t.Fatalf("expected ErrDeviceNotFound, got %v", err)
	}
}

func TestComplianceEngine(t *testing.T) {
	repo := newMockRepo()
	events := newMockPublisher()
	svc := New(repo, events, 100)
	ctx := context.Background()
	orgID := "org-compliance"

	// Profile requires firmware 5.1(1a)
	profile := &domain.ServerProfile{
		ID:             "profile-golden",
		OrgID:          orgID,
		Name:           "Production-Golden-Firmware",
		TargetFirmware: "5.1(1a)",
		PowerPolicy:    "performance",
	}
	repo.profiles[profile.ID] = profile

	// Device 1: Compliant (firmware matches)
	devCompliant := &domain.Device{
		ID:              "dev-compliant",
		OrgID:           orgID,
		Serial:          "SN-OK",
		FirmwareVersion: "5.1(1a)",
		PowerState:      "performance",
		CreatedAt:       time.Now(),
	}
	_ = repo.UpsertDevice(ctx, devCompliant)
	_ = repo.AssignProfile(ctx, orgID, devCompliant.ID, profile.ID)

	res1, err := svc.EvaluateCompliance(ctx, orgID, devCompliant.ID)
	if err != nil {
		t.Fatalf("compliance eval 1 failed: %v", err)
	}
	if res1.Status != "compliant" || len(res1.DriftDetails) != 0 {
		t.Fatalf("expected compliant status, got %s with %d drifts", res1.Status, len(res1.DriftDetails))
	}

	// Device 2: Drifted (firmware is 4.2(3b))
	devDrifted := &domain.Device{
		ID:              "dev-drifted",
		OrgID:           orgID,
		Serial:          "SN-DRIFT",
		FirmwareVersion: "4.2(3b)",
		PowerState:      "performance",
		CreatedAt:       time.Now(),
	}
	_ = repo.UpsertDevice(ctx, devDrifted)
	_ = repo.AssignProfile(ctx, orgID, devDrifted.ID, profile.ID)

	res2, err := svc.EvaluateCompliance(ctx, orgID, devDrifted.ID)
	if err != nil {
		t.Fatalf("compliance eval 2 failed: %v", err)
	}
	if res2.Status != "drifted" {
		t.Fatalf("expected drifted status, got %s", res2.Status)
	}
	if len(res2.DriftDetails) != 1 || res2.DriftDetails[0].Component != "firmware_version" {
		t.Fatalf("expected firmware drift detail, got: %+v", res2.DriftDetails)
	}

	// Device 3: Without profile
	devNoProfile := &domain.Device{
		ID:        "dev-no-profile",
		OrgID:     orgID,
		Serial:    "SN-NONE",
		CreatedAt: time.Now(),
	}
	_ = repo.UpsertDevice(ctx, devNoProfile)
	_, err = svc.EvaluateCompliance(ctx, orgID, devNoProfile.ID)
	if err != domain.ErrProfileNotFound {
		t.Fatalf("expected ErrProfileNotFound, got %v", err)
	}

	// Verify NATS event emitted for drifted device
	if len(events.events["inventory.drift.detected"]) != 1 {
		t.Fatalf("expected 1 'inventory.drift.detected' event, got %d", len(events.events["inventory.drift.detected"]))
	}
}

func TestListDevicesPagination(t *testing.T) {
	repo := newMockRepo()
	svc := New(repo, nil, 100)
	ctx := context.Background()
	orgID := "org-paged"

	for i := 0; i < 5; i++ {
		dev := &domain.Device{
			ID:        fmt.Sprintf("dev-paged-%d", i),
			OrgID:     orgID,
			Serial:    fmt.Sprintf("SN-PAGE-%d", i),
			CreatedAt: time.Now().Add(time.Duration(i) * time.Minute),
		}
		_ = repo.UpsertDevice(ctx, dev)
	}

	filter := domain.DeviceFilter{
		OrgID: orgID,
		Limit: 2,
	}

	devices, nextCursor, hasMore, total, err := svc.ListDevices(ctx, filter, "")
	if err != nil {
		t.Fatalf("list devices failed: %v", err)
	}
	if len(devices) != 2 || !hasMore || nextCursor == "" || total != 5 {
		t.Fatalf("unexpected pagination results: count=%d, hasMore=%v, nextCursor=%s, total=%d",
			len(devices), hasMore, nextCursor, total)
	}

	// Fetch next page with cursor
	devicesPage2, _, _, _, err := svc.ListDevices(ctx, filter, nextCursor)
	if err != nil {
		t.Fatalf("page 2 failed: %v", err)
	}
	if len(devicesPage2) != 2 {
		t.Fatalf("expected 2 devices on page 2, got %d", len(devicesPage2))
	}
}

func TestKeysetCursorEncoding(t *testing.T) {
	now := time.Date(2026, 10, 28, 12, 0, 0, 123456789, time.UTC)
	id := "b2184e56-1111-4000-8000-123456789abc"

	encoded := encodeCursor(now, id)
	if encoded == "" {
		t.Fatalf("expected non-empty cursor token")
	}

	decodedTime, decodedID, err := decodeCursor(encoded)
	if err != nil {
		t.Fatalf("cursor decode failed: %v", err)
	}

	if decodedID != id {
		t.Fatalf("expected id %s, got %s", id, decodedID)
	}
	if !decodedTime.Equal(now) {
		t.Fatalf("expected time %v, got %v", now, decodedTime)
	}

	// Invalid cursors
	_, _, err = decodeCursor("not-base-64!@#")
	if err == nil {
		t.Fatalf("expected error on invalid base64")
	}
	_, _, err = decodeCursor("bm9jb2xvbg==") // "nocolon"
	if err == nil {
		t.Fatalf("expected error on missing colon")
	}
	_, _, err = decodeCursor("bm90YW51bWJlcjppZA==") // "notanumber:id"
	if err == nil {
		t.Fatalf("expected error on non-numeric timestamp")
	}
}
