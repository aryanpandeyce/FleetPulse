package repo

import (
	"context"
	"testing"
	"time"

	"github.com/fleetpulse/fleetpulse/services/inventory-service/internal/domain"
)

func TestMemoryRepoLifecycle(t *testing.T) {
	ctx := context.Background()
	repo := NewMemoryRepo()

	orgID := "org-1"
	dev := &domain.Device{
		ID:              "dev-1",
		OrgID:           orgID,
		Serial:          "UCS-001",
		Model:           "UCS-C220-M7",
		FirmwareVersion: "4.2(3a)",
		PowerState:      "on",
		HealthStatus:    "ok",
		ConnectionState: "connected",
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}

	// 1. Upsert Device
	if err := repo.UpsertDevice(ctx, dev); err != nil {
		t.Fatalf("failed to upsert device: %v", err)
	}

	// 2. GetDeviceByID
	got, err := repo.GetDeviceByID(ctx, orgID, "dev-1")
	if err != nil || got.Serial != "UCS-001" {
		t.Fatalf("unexpected GetDeviceByID: %v, %+v", err, got)
	}

	// 2b. GetDeviceByID not found / wrong org
	_, err = repo.GetDeviceByID(ctx, "wrong-org", "dev-1")
	if err != domain.ErrDeviceNotFound {
		t.Fatalf("expected ErrDeviceNotFound for wrong org, got %v", err)
	}

	// 3. GetDeviceBySerial
	gotSerial, err := repo.GetDeviceBySerial(ctx, "UCS-001")
	if err != nil || gotSerial.ID != "dev-1" {
		t.Fatalf("unexpected GetDeviceBySerial: %v, %+v", err, gotSerial)
	}
	_, err = repo.GetDeviceBySerial(ctx, "UNKNOWN")
	if err != domain.ErrDeviceNotFound {
		t.Fatalf("expected ErrDeviceNotFound for unknown serial")
	}

	// 4. ListDevices
	list, hasMore, total, err := repo.ListDevices(ctx, domain.DeviceFilter{
		OrgID: orgID,
		Limit: 10,
	})
	if err != nil || len(list) != 1 || total != 1 || hasMore {
		t.Fatalf("unexpected ListDevices: err=%v, count=%d, total=%d, hasMore=%v", err, len(list), total, hasMore)
	}

	// 5. Claim code lifecycle
	claim := &domain.DeviceClaim{
		ID:        "c-1",
		OrgID:     orgID,
		ClaimCode: "FP-12345",
		Status:    "pending",
		ExpiresAt: time.Now().Add(10 * time.Minute),
	}
	if err := repo.CreateClaimCode(ctx, claim); err != nil {
		t.Fatalf("failed to create claim code: %v", err)
	}
	gotClaim, err := repo.GetClaimCode(ctx, "FP-12345")
	if err != nil || gotClaim.ClaimCode != "FP-12345" {
		t.Fatalf("unexpected GetClaimCode: %v, %+v", err, gotClaim)
	}
	_, err = repo.GetClaimCode(ctx, "INVALID")
	if err != domain.ErrClaimCodeInvalid {
		t.Fatalf("expected ErrClaimCodeInvalid")
	}

	if err := repo.MarkClaimUsed(ctx, "FP-12345", "UCS-001"); err != nil {
		t.Fatalf("failed to mark claim used: %v", err)
	}
	if err := repo.MarkClaimUsed(ctx, "INVALID", "UCS-001"); err != domain.ErrClaimCodeInvalid {
		t.Fatalf("expected ErrClaimCodeInvalid for invalid claim")
	}

	// 6. Profile & Assignment
	prof := &domain.ServerProfile{
		ID:             "prof-1",
		OrgID:          orgID,
		Name:           "Golden",
		TargetFirmware: "4.2(3a)",
		PowerPolicy:    "performance",
	}
	repo.profiles["prof-1"] = prof

	pGot, err := repo.GetProfile(ctx, orgID, "prof-1")
	if err != nil || pGot.Name != "Golden" {
		t.Fatalf("unexpected GetProfile: %v, %+v", err, pGot)
	}
	_, err = repo.GetProfile(ctx, "wrong-org", "prof-1")
	if err != domain.ErrProfileNotFound {
		t.Fatalf("expected ErrProfileNotFound for wrong org")
	}

	// Assign profile
	if err := repo.AssignProfile(ctx, orgID, "dev-1", "prof-1"); err != nil {
		t.Fatalf("failed to assign profile: %v", err)
	}
	devProf, err := repo.GetDeviceProfile(ctx, orgID, "dev-1")
	if err != nil || devProf.ID != "prof-1" {
		t.Fatalf("unexpected GetDeviceProfile: %v, %+v", err, devProf)
	}
	unassignedProf, err := repo.GetDeviceProfile(ctx, orgID, "dev-unknown")
	if err != nil || unassignedProf != nil {
		t.Fatalf("expected nil for unassigned device profile")
	}

	// 7. RecordCompliance
	cr := &domain.ComplianceResult{
		ID:          "cr-1",
		OrgID:       orgID,
		DeviceID:    "dev-1",
		ProfileID:   "prof-1",
		Status:      "compliant",
		EvaluatedAt: time.Now(),
	}
	if err := repo.RecordCompliance(ctx, cr); err != nil {
		t.Fatalf("failed to record compliance: %v", err)
	}
}
