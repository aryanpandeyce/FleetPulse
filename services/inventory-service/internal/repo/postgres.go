// Package repo implements data access for Inventory Service using PostgreSQL with pgx/v5.
package repo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/fleetpulse/fleetpulse/services/inventory-service/internal/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresRepo implements domain.Repository with pgx connection pooling.
type PostgresRepo struct {
	pool *pgxpool.Pool
}

// NewPostgresRepo creates a new PostgreSQL repository instance.
func NewPostgresRepo(pool *pgxpool.Pool) *PostgresRepo {
	return &PostgresRepo{pool: pool}
}

// GetDeviceByID queries a device by ID scoped strictly to the organization.
func (r *PostgresRepo) GetDeviceByID(ctx context.Context, orgID, id string) (*domain.Device, error) {
	query := `
		SELECT id, org_id, serial, model, firmware_version, power_state, health_status, connection_state,
		       tags, version, claimed_at, last_seen_at, created_at, updated_at
		FROM devices
		WHERE org_id = $1 AND id = $2;
	`
	var d domain.Device
	var tagsBytes []byte
	err := r.pool.QueryRow(ctx, query, orgID, id).Scan(
		&d.ID, &d.OrgID, &d.Serial, &d.Model, &d.FirmwareVersion, &d.PowerState, &d.HealthStatus,
		&d.ConnectionState, &tagsBytes, &d.Version, &d.ClaimedAt, &d.LastSeenAt, &d.CreatedAt, &d.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrDeviceNotFound
		}
		return nil, fmt.Errorf("failed to query device by id: %w", err)
	}
	_ = json.Unmarshal(tagsBytes, &d.Tags)
	return &d, nil
}

// GetDeviceBySerial queries a device by its unique serial number.
func (r *PostgresRepo) GetDeviceBySerial(ctx context.Context, serial string) (*domain.Device, error) {
	query := `
		SELECT id, org_id, serial, model, firmware_version, power_state, health_status, connection_state,
		       tags, version, claimed_at, last_seen_at, created_at, updated_at
		FROM devices
		WHERE serial = $1;
	`
	var d domain.Device
	var tagsBytes []byte
	err := r.pool.QueryRow(ctx, query, serial).Scan(
		&d.ID, &d.OrgID, &d.Serial, &d.Model, &d.FirmwareVersion, &d.PowerState, &d.HealthStatus,
		&d.ConnectionState, &tagsBytes, &d.Version, &d.ClaimedAt, &d.LastSeenAt, &d.CreatedAt, &d.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrDeviceNotFound
		}
		return nil, fmt.Errorf("failed to query device by serial: %w", err)
	}
	_ = json.Unmarshal(tagsBytes, &d.Tags)
	return &d, nil
}

// ListDevices executes a keyset-paginated query over the devices table with composite B-Tree index traversal.
func (r *PostgresRepo) ListDevices(ctx context.Context, f domain.DeviceFilter) ([]domain.Device, bool, int64, error) {
	// Query total count for tenant
	var totalCount int64
	countErr := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM devices WHERE org_id = $1`, f.OrgID).Scan(&totalCount)
	if countErr != nil {
		totalCount = 0
	}

	query := `
		SELECT id, org_id, serial, model, firmware_version, power_state, health_status, connection_state,
		       tags, version, claimed_at, last_seen_at, created_at, updated_at
		FROM devices
		WHERE org_id = $1
	`
	args := []interface{}{f.OrgID}
	argIdx := 2

	if !f.CursorCreatedAt.IsZero() && f.CursorID != "" {
		query += fmt.Sprintf(" AND (created_at, id) < ($%d, $%d)", argIdx, argIdx+1)
		args = append(args, f.CursorCreatedAt, f.CursorID)
		argIdx += 2
	}
	if f.Model != "" {
		query += fmt.Sprintf(" AND model = $%d", argIdx)
		args = append(args, f.Model)
		argIdx++
	}
	if f.HealthStatus != "" {
		query += fmt.Sprintf(" AND health_status = $%d", argIdx)
		args = append(args, f.HealthStatus)
		argIdx++
	}
	if f.FirmwareVersion != "" {
		query += fmt.Sprintf(" AND firmware_version = $%d", argIdx)
		args = append(args, f.FirmwareVersion)
		argIdx++
	}
	if f.ConnectionState != "" {
		query += fmt.Sprintf(" AND connection_state = $%d", argIdx)
		args = append(args, f.ConnectionState)
		argIdx++
	}

	// Fetch Limit + 1 to determine if there is a next page
	query += fmt.Sprintf(" ORDER BY created_at DESC, id DESC LIMIT $%d", argIdx)
	args = append(args, f.Limit+1)

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, false, 0, fmt.Errorf("failed to list devices: %w", err)
	}
	defer rows.Close()

	var devices []domain.Device
	for rows.Next() {
		var d domain.Device
		var tagsBytes []byte
		if err := rows.Scan(
			&d.ID, &d.OrgID, &d.Serial, &d.Model, &d.FirmwareVersion, &d.PowerState, &d.HealthStatus,
			&d.ConnectionState, &tagsBytes, &d.Version, &d.ClaimedAt, &d.LastSeenAt, &d.CreatedAt, &d.UpdatedAt,
		); err != nil {
			return nil, false, 0, err
		}
		_ = json.Unmarshal(tagsBytes, &d.Tags)
		devices = append(devices, d)
	}

	hasMore := false
	if len(devices) > f.Limit {
		hasMore = true
		devices = devices[:f.Limit]
	}

	return devices, hasMore, totalCount, nil
}

// UpsertDevice inserts or updates a device record with optimistic concurrency protection.
func (r *PostgresRepo) UpsertDevice(ctx context.Context, d *domain.Device) error {
	tagsBytes, _ := json.Marshal(d.Tags)
	query := `
		INSERT INTO devices (
			id, org_id, serial, model, firmware_version, power_state, health_status,
			connection_state, tags, version, claimed_at, last_seen_at, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
		ON CONFLICT (serial) DO UPDATE SET
			firmware_version = EXCLUDED.firmware_version,
			power_state = EXCLUDED.power_state,
			health_status = EXCLUDED.health_status,
			connection_state = EXCLUDED.connection_state,
			tags = EXCLUDED.tags,
			version = devices.version + 1,
			last_seen_at = EXCLUDED.last_seen_at,
			updated_at = NOW()
		WHERE devices.org_id = EXCLUDED.org_id
		RETURNING version;
	`
	return r.pool.QueryRow(ctx, query,
		d.ID, d.OrgID, d.Serial, d.Model, d.FirmwareVersion, d.PowerState, d.HealthStatus,
		d.ConnectionState, tagsBytes, d.Version, d.ClaimedAt, d.LastSeenAt, d.CreatedAt, d.UpdatedAt,
	).Scan(&d.Version)
}

// CreateClaimCode persists a new onboarding token with expiration.
func (r *PostgresRepo) CreateClaimCode(ctx context.Context, claim *domain.DeviceClaim) error {
	query := `
		INSERT INTO device_claims (id, org_id, claim_code, status, expires_at, created_at)
		VALUES ($1, $2, $3, $4, $5, $6);
	`
	_, err := r.pool.Exec(ctx, query,
		claim.ID, claim.OrgID, claim.ClaimCode, claim.Status, claim.ExpiresAt, claim.CreatedAt,
	)
	return err
}

// GetClaimCode retrieves claim by code.
func (r *PostgresRepo) GetClaimCode(ctx context.Context, code string) (*domain.DeviceClaim, error) {
	query := `
		SELECT id, org_id, claim_code, status, expires_at, created_at
		FROM device_claims
		WHERE claim_code = $1;
	`
	var c domain.DeviceClaim
	err := r.pool.QueryRow(ctx, query, code).Scan(
		&c.ID, &c.OrgID, &c.ClaimCode, &c.Status, &c.ExpiresAt, &c.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrClaimCodeInvalid
		}
		return nil, err
	}
	return &c, nil
}

// MarkClaimUsed transitions a claim to 'claimed' status atomically.
func (r *PostgresRepo) MarkClaimUsed(ctx context.Context, code, serial string) error {
	query := `
		UPDATE device_claims
		SET status = 'claimed', serial = $1, claimed_at = NOW()
		WHERE claim_code = $2 AND status = 'pending';
	`
	cmd, err := r.pool.Exec(ctx, query, serial, code)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return domain.ErrClaimCodeInvalid
	}
	return nil
}

// GetProfile retrieves a server profile by ID.
func (r *PostgresRepo) GetProfile(ctx context.Context, orgID, profileID string) (*domain.ServerProfile, error) {
	query := `
		SELECT id, org_id, name, description, target_firmware, power_policy, boot_order, created_at, updated_at
		FROM server_profiles
		WHERE org_id = $1 AND id = $2;
	`
	var p domain.ServerProfile
	var bootBytes []byte
	err := r.pool.QueryRow(ctx, query, orgID, profileID).Scan(
		&p.ID, &p.OrgID, &p.Name, &p.Description, &p.TargetFirmware, &p.PowerPolicy, &bootBytes, &p.CreatedAt, &p.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrProfileNotFound
		}
		return nil, err
	}
	_ = json.Unmarshal(bootBytes, &p.BootOrder)
	return &p, nil
}

// GetDeviceProfile returns the server profile assigned to a given device.
func (r *PostgresRepo) GetDeviceProfile(ctx context.Context, orgID, deviceID string) (*domain.ServerProfile, error) {
	query := `
		SELECT p.id, p.org_id, p.name, p.description, p.target_firmware, p.power_policy, p.boot_order, p.created_at, p.updated_at
		FROM server_profiles p
		INNER JOIN device_profile_assignments a ON a.profile_id = p.id
		WHERE p.org_id = $1 AND a.device_id = $2;
	`
	var p domain.ServerProfile
	var bootBytes []byte
	err := r.pool.QueryRow(ctx, query, orgID, deviceID).Scan(
		&p.ID, &p.OrgID, &p.Name, &p.Description, &p.TargetFirmware, &p.PowerPolicy, &bootBytes, &p.CreatedAt, &p.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil // No profile assigned yet
		}
		return nil, err
	}
	_ = json.Unmarshal(bootBytes, &p.BootOrder)
	return &p, nil
}

// AssignProfile associates a device with a target server profile.
func (r *PostgresRepo) AssignProfile(ctx context.Context, orgID, deviceID, profileID string) error {
	query := `
		INSERT INTO device_profile_assignments (device_id, profile_id)
		VALUES ($1, $2)
		ON CONFLICT (device_id) DO UPDATE SET profile_id = EXCLUDED.profile_id, assigned_at = NOW();
	`
	_, err := r.pool.Exec(ctx, query, deviceID, profileID)
	return err
}

// RecordCompliance stores an audit record of device compliance against a profile.
func (r *PostgresRepo) RecordCompliance(ctx context.Context, cr *domain.ComplianceResult) error {
	driftBytes, _ := json.Marshal(cr.DriftDetails)
	query := `
		INSERT INTO compliance_results (id, org_id, device_id, profile_id, status, drift_details, evaluated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7);
	`
	_, err := r.pool.Exec(ctx, query, cr.ID, cr.OrgID, cr.DeviceID, cr.ProfileID, cr.Status, driftBytes, cr.EvaluatedAt)
	return err
}
