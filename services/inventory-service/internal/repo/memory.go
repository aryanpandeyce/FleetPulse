// Package repo implements data access for Inventory Service.
package repo

import (
	"context"
	"sync"
	"time"

	"github.com/fleetpulse/fleetpulse/services/inventory-service/internal/domain"
)

// MemoryRepo provides a thread-safe in-memory implementation of domain.Repository.
type MemoryRepo struct {
	mu          sync.RWMutex
	devices     map[string]*domain.Device
	claims      map[string]*domain.DeviceClaim
	profiles    map[string]*domain.ServerProfile
	assignments map[string]string // deviceID -> profileID
	compliances []*domain.ComplianceResult
}

// NewMemoryRepo initializes a new in-memory inventory repository.
func NewMemoryRepo() *MemoryRepo {
	return &MemoryRepo{
		devices:     make(map[string]*domain.Device),
		claims:      make(map[string]*domain.DeviceClaim),
		profiles:    make(map[string]*domain.ServerProfile),
		assignments: make(map[string]string),
		compliances: make([]*domain.ComplianceResult, 0),
	}
}

// GetDeviceByID queries a device by ID scoped strictly to the organization.
func (m *MemoryRepo) GetDeviceByID(ctx context.Context, orgID, id string) (*domain.Device, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	d, ok := m.devices[id]
	if !ok || d.OrgID != orgID {
		return nil, domain.ErrDeviceNotFound
	}
	return d, nil
}

// GetDeviceBySerial queries a device by serial number.
func (m *MemoryRepo) GetDeviceBySerial(ctx context.Context, serial string) (*domain.Device, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, d := range m.devices {
		if d.Serial == serial {
			return d, nil
		}
	}
	return nil, domain.ErrDeviceNotFound
}

// ListDevices returns devices matching filter with keyset pagination simulation.
func (m *MemoryRepo) ListDevices(ctx context.Context, filter domain.DeviceFilter) ([]domain.Device, bool, int64, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var matches []domain.Device
	for _, d := range m.devices {
		if d.OrgID != filter.OrgID {
			continue
		}
		if filter.Model != "" && d.Model != filter.Model {
			continue
		}
		if filter.HealthStatus != "" && d.HealthStatus != filter.HealthStatus {
			continue
		}
		if filter.FirmwareVersion != "" && d.FirmwareVersion != filter.FirmwareVersion {
			continue
		}
		if filter.ConnectionState != "" && d.ConnectionState != filter.ConnectionState {
			continue
		}
		matches = append(matches, *d)
	}

	total := int64(len(matches))
	limit := filter.Limit
	if limit <= 0 {
		limit = 20
	}

	hasMore := len(matches) > limit
	if hasMore {
		matches = matches[:limit]
	}

	return matches, hasMore, total, nil
}

// UpsertDevice stores or updates a device record.
func (m *MemoryRepo) UpsertDevice(ctx context.Context, d *domain.Device) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.devices[d.ID] = d
	return nil
}

// CreateClaimCode persists a new claim code.
func (m *MemoryRepo) CreateClaimCode(ctx context.Context, claim *domain.DeviceClaim) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.claims[claim.ClaimCode] = claim
	return nil
}

// GetClaimCode retrieves an active claim code.
func (m *MemoryRepo) GetClaimCode(ctx context.Context, code string) (*domain.DeviceClaim, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	c, ok := m.claims[code]
	if !ok {
		return nil, domain.ErrClaimCodeInvalid
	}
	return c, nil
}

// MarkClaimUsed transitions a claim code to used status.
func (m *MemoryRepo) MarkClaimUsed(ctx context.Context, code, serial string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	c, ok := m.claims[code]
	if !ok {
		return domain.ErrClaimCodeInvalid
	}
	now := time.Now()
	c.Status = "claimed"
	c.Serial = serial
	c.ClaimedAt = &now
	return nil
}

// GetProfile retrieves a server profile by ID.
func (m *MemoryRepo) GetProfile(ctx context.Context, orgID, profileID string) (*domain.ServerProfile, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	p, ok := m.profiles[profileID]
	if !ok || p.OrgID != orgID {
		return nil, domain.ErrProfileNotFound
	}
	return p, nil
}

// GetDeviceProfile retrieves the profile assigned to a device.
func (m *MemoryRepo) GetDeviceProfile(ctx context.Context, orgID, deviceID string) (*domain.ServerProfile, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	profID, ok := m.assignments[deviceID]
	if !ok {
		return nil, nil
	}
	return m.GetProfile(ctx, orgID, profID)
}

// AssignProfile associates a profile with a device.
func (m *MemoryRepo) AssignProfile(ctx context.Context, orgID, deviceID, profileID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.assignments[deviceID] = profileID
	return nil
}

// RecordCompliance saves a compliance audit record.
func (m *MemoryRepo) RecordCompliance(ctx context.Context, cr *domain.ComplianceResult) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.compliances = append(m.compliances, cr)
	return nil
}
