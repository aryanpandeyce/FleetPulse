// Package service implements application use cases for inventory and compliance management.
package service

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/fleetpulse/fleetpulse/pkg/ds/lru"
	"github.com/fleetpulse/fleetpulse/services/inventory-service/internal/domain"
	"github.com/google/uuid"
)

// EventPublisher abstracts NATS event emission.
type EventPublisher interface {
	Publish(subject string, data []byte) error
}

// Service manages devices, claims, and compliance policies.
type Service struct {
	repo   domain.Repository
	cache  *lru.Cache[string, *domain.Device]
	events EventPublisher
}

// New creates a new inventory Service instance with an integrated LRU cache.
func New(repo domain.Repository, events EventPublisher, cacheCap int) *Service {
	if cacheCap <= 0 {
		cacheCap = 1000
	}
	return &Service{
		repo:   repo,
		cache:  lru.New[string, *domain.Device](cacheCap, 10*time.Minute),
		events: events,
	}
}

// GetDevice looks up a device by ID or serial number, checking the LRU cache first.
func (s *Service) GetDevice(ctx context.Context, orgID, identifier string) (*domain.Device, error) {
	cacheKey := fmt.Sprintf("%s:%s", orgID, identifier)
	if cached, ok := s.cache.Get(cacheKey); ok {
		return cached, nil
	}

	var d *domain.Device
	var err error

	// Try lookup by ID first
	d, err = s.repo.GetDeviceByID(ctx, orgID, identifier)
	if err != nil || d == nil {
		// Fallback to lookup by Serial
		d, err = s.repo.GetDeviceBySerial(ctx, identifier)
		if err == nil && d != nil && d.OrgID != orgID {
			return nil, domain.ErrDeviceNotFound
		}
	}

	if err != nil {
		return nil, err
	}
	if d == nil {
		return nil, domain.ErrDeviceNotFound
	}

	// Cache both by ID and by Serial
	s.cache.Put(fmt.Sprintf("%s:%s", orgID, d.ID), d)
	s.cache.Put(fmt.Sprintf("%s:%s", orgID, d.Serial), d)

	return d, nil
}

// ListDevices handles keyset cursor pagination and filtering.
func (s *Service) ListDevices(ctx context.Context, filter domain.DeviceFilter, cursor string) ([]domain.Device, string, bool, int64, error) {
	if cursor != "" {
		cTime, cID, err := decodeCursor(cursor)
		if err == nil {
			filter.CursorCreatedAt = cTime
			filter.CursorID = cID
		}
	}

	if filter.Limit <= 0 || filter.Limit > 100 {
		filter.Limit = 20
	}

	devices, hasMore, totalCount, err := s.repo.ListDevices(ctx, filter)
	if err != nil {
		return nil, "", false, 0, fmt.Errorf("failed to list devices: %w", err)
	}

	var nextCursor string
	if hasMore && len(devices) > 0 {
		last := devices[len(devices)-1]
		nextCursor = encodeCursor(last.CreatedAt, last.ID)
	}

	return devices, nextCursor, hasMore, totalCount, nil
}

// CreateClaimCode generates a single-use expiring token for device onboarding.
func (s *Service) CreateClaimCode(ctx context.Context, orgID string, ttl time.Duration) (*domain.DeviceClaim, error) {
	if ttl <= 0 {
		ttl = 15 * time.Minute
	}

	tokenBytes := make([]byte, 16)
	if _, err := rand.Read(tokenBytes); err != nil {
		return nil, fmt.Errorf("failed to generate random token: %w", err)
	}
	claimCode := "FP-" + strings.ToUpper(hex.EncodeToString(tokenBytes)[:16])

	now := time.Now()
	claim := &domain.DeviceClaim{
		ID:        uuid.NewString(),
		OrgID:     orgID,
		ClaimCode: claimCode,
		Status:    "pending",
		ExpiresAt: now.Add(ttl),
		CreatedAt: now,
	}

	if err := s.repo.CreateClaimCode(ctx, claim); err != nil {
		return nil, fmt.Errorf("failed to persist claim code: %w", err)
	}

	return claim, nil
}

// ClaimDevice is invoked by a device simulator or connector presenting its serial and claim code.
func (s *Service) ClaimDevice(ctx context.Context, claimCode, serial, model, firmware string) (*domain.Device, error) {
	claim, err := s.repo.GetClaimCode(ctx, claimCode)
	if err != nil {
		return nil, domain.ErrClaimCodeInvalid
	}
	if claim == nil || claim.Status != "pending" {
		return nil, domain.ErrClaimCodeInvalid
	}
	if time.Now().After(claim.ExpiresAt) {
		return nil, domain.ErrClaimCodeExpired
	}

	// Invalidate claim code (single use)
	if err := s.repo.MarkClaimUsed(ctx, claimCode, serial); err != nil {
		return nil, fmt.Errorf("failed to mark claim as used: %w", err)
	}

	now := time.Now()
	device := &domain.Device{
		ID:              uuid.NewString(),
		OrgID:           claim.OrgID,
		Serial:          serial,
		Model:           model,
		FirmwareVersion: firmware,
		PowerState:      "on",
		HealthStatus:    "ok",
		ConnectionState: "connected",
		Tags:            map[string]string{"claimed_by": "connector"},
		Version:         1,
		ClaimedAt:       now,
		LastSeenAt:      now,
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	if err := s.repo.UpsertDevice(ctx, device); err != nil {
		return nil, fmt.Errorf("failed to persist claimed device: %w", err)
	}

	// Update cache
	s.cache.Put(fmt.Sprintf("%s:%s", device.OrgID, device.ID), device)
	s.cache.Put(fmt.Sprintf("%s:%s", device.OrgID, device.Serial), device)

	// Publish NATS event
	if s.events != nil {
		payload, _ := json.Marshal(map[string]string{
			"org_id":   device.OrgID,
			"device_id": device.ID,
			"serial":   device.Serial,
			"model":    device.Model,
		})
		_ = s.events.Publish("inventory.device.claimed", payload)
	}

	return device, nil
}

// EvaluateCompliance audits device actual state against assigned ServerProfile.
func (s *Service) EvaluateCompliance(ctx context.Context, orgID, deviceID string) (*domain.ComplianceResult, error) {
	device, err := s.GetDevice(ctx, orgID, deviceID)
	if err != nil {
		return nil, err
	}

	profile, err := s.repo.GetDeviceProfile(ctx, orgID, deviceID)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve device profile: %w", err)
	}
	if profile == nil {
		return nil, domain.ErrProfileNotFound
	}

	var drifts []domain.DriftItem

	// 1. Audit Firmware Version
	if profile.TargetFirmware != "" && device.FirmwareVersion != profile.TargetFirmware {
		drifts = append(drifts, domain.DriftItem{
			Component:     "firmware_version",
			ActualValue:   device.FirmwareVersion,
			ExpectedValue: profile.TargetFirmware,
		})
	}

	// 2. Audit Power Policy
	if profile.PowerPolicy != "" && device.PowerState != profile.PowerPolicy && device.PowerState != "on" {
		drifts = append(drifts, domain.DriftItem{
			Component:     "power_policy",
			ActualValue:   device.PowerState,
			ExpectedValue: profile.PowerPolicy,
		})
	}

	status := "compliant"
	if len(drifts) > 0 {
		status = "drifted"
	}

	result := &domain.ComplianceResult{
		ID:           uuid.NewString(),
		OrgID:        orgID,
		DeviceID:     deviceID,
		ProfileID:    profile.ID,
		Status:       status,
		DriftDetails: drifts,
		EvaluatedAt:  time.Now(),
	}

	if err := s.repo.RecordCompliance(ctx, result); err != nil {
		return nil, fmt.Errorf("failed to record compliance audit: %w", err)
	}

	// Emit drift event if drifted
	if status == "drifted" && s.events != nil {
		payload, _ := json.Marshal(map[string]interface{}{
			"org_id":        orgID,
			"device_id":     deviceID,
			"profile_id":    profile.ID,
			"drift_details": drifts,
		})
		_ = s.events.Publish("inventory.drift.detected", payload)
	}

	return result, nil
}

func encodeCursor(t time.Time, id string) string {
	raw := fmt.Sprintf("%d:%s", t.UnixNano(), id)
	return base64.URLEncoding.EncodeToString([]byte(raw))
}

func decodeCursor(token string) (time.Time, string, error) {
	bytes, err := base64.URLEncoding.DecodeString(token)
	if err != nil {
		return time.Time{}, "", err
	}
	parts := strings.SplitN(string(bytes), ":", 2)
	if len(parts) != 2 {
		return time.Time{}, "", fmt.Errorf("invalid cursor token format")
	}
	nanos, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return time.Time{}, "", err
	}
	return time.Unix(0, nanos), parts[1], nil
}
