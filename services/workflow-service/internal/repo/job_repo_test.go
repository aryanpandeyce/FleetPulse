package repo

import (
	"testing"
	"time"

	"github.com/fleetpulse/fleetpulse/services/workflow-service/internal/domain"
)

func TestJobRepository(t *testing.T) {
	r := NewRepository()

	job := &domain.Job{
		ID:             "job-1",
		OrgID:          "org-acme",
		Type:           domain.TypeFirmwareUpgrade,
		Priority:       domain.PriorityNormal,
		State:          domain.StateQueued,
		Targets:        []string{"UCS-001"},
		IdempotencyKey: "idem-key-1",
		CreatedAt:      time.Now(),
	}

	// 1. Save and GetByID
	if err := r.Save(job); err != nil {
		t.Fatalf("failed to save job: %v", err)
	}

	found, err := r.GetByID("job-1")
	if err != nil || found.ID != "job-1" {
		t.Fatalf("failed to retrieve job: %v", err)
	}

	// 2. Idempotency Key Lookup
	idemJob, exists := r.GetByIdempotency("org-acme", "idem-key-1")
	if !exists || idemJob.ID != "job-1" {
		t.Fatalf("failed to find job by idempotency key")
	}

	// Unknown idempotency key
	_, exists = r.GetByIdempotency("org-acme", "unknown-key")
	if exists {
		t.Fatalf("expected false for unknown key")
	}

	// 3. Update State
	if err := r.UpdateState("job-1", domain.StateRunning, ""); err != nil {
		t.Fatalf("failed to update state: %v", err)
	}
	if found.State != domain.StateRunning || found.StartedAt.IsZero() {
		t.Fatalf("expected running state and started timestamp: %+v", found)
	}

	// 4. Add Step
	step := domain.StepRecord{
		StepName:  "Validate",
		Status:    "success",
		Message:   "OK",
		Timestamp: time.Now(),
	}
	if err := r.AddStep("job-1", step); err != nil {
		t.Fatalf("failed to add step: %v", err)
	}
	if len(found.StepHistory) != 1 || found.StepHistory[0].StepName != "Validate" {
		t.Fatalf("step not added to history")
	}

	// 5. Complete Job
	if err := r.UpdateState("job-1", domain.StateSucceeded, ""); err != nil {
		t.Fatalf("failed to complete job: %v", err)
	}
	if !found.IsTerminal() || found.CompletedAt.IsZero() {
		t.Fatalf("expected terminal state and completed timestamp: %+v", found)
	}

	// 6. List with filters
	list := r.List("org-acme", domain.StateSucceeded, 10)
	if len(list) != 1 || list[0].ID != "job-1" {
		t.Fatalf("expected 1 job in list, got %+v", list)
	}

	// Non-existent job
	_, err = r.GetByID("job-missing")
	if err != domain.ErrJobNotFound {
		t.Fatalf("expected ErrJobNotFound, got %v", err)
	}
}
