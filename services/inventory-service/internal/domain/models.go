// Package domain contains pure business entities and repository contracts for inventory management.
package domain

import (
	"context"
	"errors"
	"time"
)

var (
	ErrDeviceNotFound          = errors.New("device not found")
	ErrClaimCodeInvalid        = errors.New("claim code is invalid or has already been used")
	ErrClaimCodeExpired        = errors.New("claim code has expired")
	ErrProfileNotFound         = errors.New("server profile not found")
	ErrOptimisticLockConflict  = errors.New("concurrent modification detected; version mismatch")
)

// Device represents a compute server registered within a tenant.
type Device struct {
	ID              string            `json:"id"`
	OrgID           string            `json:"org_id"`
	Serial          string            `json:"serial"`
	Model           string            `json:"model"`
	FirmwareVersion string            `json:"firmware_version"`
	PowerState      string            `json:"power_state"`
	HealthStatus    string            `json:"health_status"` // "ok", "warning", "critical"
	ConnectionState string            `json:"connection_state"` // "connected", "disconnected"
	Tags            map[string]string `json:"tags"`
	Version         int32             `json:"version"`
	ClaimedAt       time.Time         `json:"claimed_at"`
	LastSeenAt      time.Time         `json:"last_seen_at"`
	CreatedAt       time.Time         `json:"created_at"`
	UpdatedAt       time.Time         `json:"updated_at"`
}

// DeviceClaim represents a single-use onboarding code.
type DeviceClaim struct {
	ID        string     `json:"id"`
	OrgID     string     `json:"org_id"`
	ClaimCode string     `json:"claim_code"`
	Serial    string     `json:"serial,omitempty"`
	Status    string     `json:"status"` // "pending", "claimed", "expired"
	ExpiresAt time.Time  `json:"expires_at"`
	CreatedAt time.Time  `json:"created_at"`
	ClaimedAt *time.Time `json:"claimed_at,omitempty"`
}

// ServerProfile specifies the desired configuration policy.
type ServerProfile struct {
	ID             string    `json:"id"`
	OrgID          string    `json:"org_id"`
	Name           string    `json:"name"`
	Description    string    `json:"description"`
	TargetFirmware string    `json:"target_firmware"`
	PowerPolicy    string    `json:"power_policy"` // "performance", "balanced", "power_saver"
	BootOrder      []string  `json:"boot_order"`   // e.g. ["nvme", "pxe", "usb"]
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// DriftItem details a specific divergence between observed and desired configuration.
type DriftItem struct {
	Component     string `json:"component"`
	ActualValue   string `json:"actual_value"`
	ExpectedValue string `json:"expected_value"`
}

// ComplianceResult records an audit of device configuration against an assigned profile.
type ComplianceResult struct {
	ID           string      `json:"id"`
	OrgID        string      `json:"org_id"`
	DeviceID     string      `json:"device_id"`
	ProfileID    string      `json:"profile_id"`
	Status       string      `json:"status"` // "compliant", "drifted"
	DriftDetails []DriftItem `json:"drift_details"`
	EvaluatedAt  time.Time   `json:"evaluated_at"`
}

// DeviceFilter parameters for keyset pagination.
type DeviceFilter struct {
	OrgID           string
	CursorCreatedAt time.Time
	CursorID        string
	Limit           int
	Model           string
	HealthStatus    string
	FirmwareVersion string
	ConnectionState string
}

// Repository defines data access operations required by the Inventory service.
type Repository interface {
	GetDeviceByID(ctx context.Context, orgID, id string) (*Device, error)
	GetDeviceBySerial(ctx context.Context, serial string) (*Device, error)
	ListDevices(ctx context.Context, filter DeviceFilter) ([]Device, bool, int64, error)
	UpsertDevice(ctx context.Context, d *Device) error
	CreateClaimCode(ctx context.Context, claim *DeviceClaim) error
	GetClaimCode(ctx context.Context, code string) (*DeviceClaim, error)
	MarkClaimUsed(ctx context.Context, code, serial string) error
	GetProfile(ctx context.Context, orgID, profileID string) (*ServerProfile, error)
	GetDeviceProfile(ctx context.Context, orgID, deviceID string) (*ServerProfile, error)
	AssignProfile(ctx context.Context, orgID, deviceID, profileID string) error
	RecordCompliance(ctx context.Context, cr *ComplianceResult) error
}
