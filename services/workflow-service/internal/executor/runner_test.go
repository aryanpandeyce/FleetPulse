package executor

import (
	"context"
	"testing"
	"time"

	"github.com/fleetpulse/fleetpulse/services/workflow-service/internal/domain"
	"github.com/fleetpulse/fleetpulse/services/workflow-service/internal/repo"
	"github.com/fleetpulse/fleetpulse/services/workflow-service/internal/scheduler"
)

func TestRunnerExecutionFlow(t *testing.T) {
	jobRepo := repo.NewRepository()
	jobSched := scheduler.NewPriorityScheduler()

	runner := NewRunner(Config{
		Scheduler:    jobSched,
		Repo:         jobRepo,
		Concurrency:  1,
		SimulatorURL: "http://127.0.0.1:9999", // Mock / non-running URL handled gracefully
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runner.Start(ctx)
	defer runner.Stop()

	// 1. Firmware Upgrade Job
	fwJob := &domain.Job{
		ID:          "job-fw",
		OrgID:       "org-acme",
		Type:        domain.TypeFirmwareUpgrade,
		Priority:    domain.PriorityHigh,
		State:       domain.StateQueued,
		Targets:     []string{"UCS-001"},
		Parameters:  map[string]string{"version": "4.3(1a)"},
		MaxAttempts: 1,
		CreatedAt:   time.Now(),
	}
	_ = jobRepo.Save(fwJob)
	jobSched.Enqueue(fwJob)

	// Wait for execution to finish
	var completed *domain.Job
	for i := 0; i < 20; i++ {
		time.Sleep(50 * time.Millisecond)
		j, _ := jobRepo.GetByID("job-fw")
		if j.IsTerminal() {
			completed = j
			break
		}
	}

	if completed == nil || completed.State != domain.StateSucceeded {
		t.Fatalf("expected job to succeed, got %+v", completed)
	}
	if len(completed.StepHistory) == 0 {
		t.Fatalf("expected step history records, got 0")
	}

	// 2. Cancelled job should not be executed
	cancelJob := &domain.Job{
		ID:        "job-cancelled",
		OrgID:     "org-acme",
		Type:      domain.TypePowerCycle,
		Priority:  domain.PriorityNormal,
		State:     domain.StateCancelled,
		Targets:   []string{"UCS-002"},
		CreatedAt: time.Now(),
	}
	_ = jobRepo.Save(cancelJob)
	jobSched.Enqueue(cancelJob)

	time.Sleep(100 * time.Millisecond)
	j, _ := jobRepo.GetByID("job-cancelled")
	if j.State != domain.StateCancelled {
		t.Fatalf("cancelled job state modified: %s", j.State)
	}

	// 3. Drift Remediation and PowerCycle Jobs
	driftJob := &domain.Job{
		ID:          "job-drift",
		OrgID:       "org-acme",
		Type:        domain.TypeDriftRemediation,
		Priority:    domain.PriorityNormal,
		State:       domain.StateQueued,
		Targets:     []string{"dev-drift-1"},
		MaxAttempts: 1,
		CreatedAt:   time.Now(),
	}
	_ = jobRepo.Save(driftJob)
	jobSched.Enqueue(driftJob)

	powerJob := &domain.Job{
		ID:          "job-power",
		OrgID:       "org-acme",
		Type:        domain.TypePowerCycle,
		Priority:    domain.PriorityNormal,
		State:       domain.StateQueued,
		Targets:     []string{"dev-power-1"},
		MaxAttempts: 1,
		CreatedAt:   time.Now(),
	}
	_ = jobRepo.Save(powerJob)
	jobSched.Enqueue(powerJob)

	// 4. Invalid Job Type triggering failure path
	invalidJob := &domain.Job{
		ID:          "job-invalid",
		OrgID:       "org-acme",
		Type:        "unsupported_bogus_type",
		Priority:    domain.PriorityNormal,
		State:       domain.StateQueued,
		Targets:     []string{"dev-1"},
		Attempts:    2,
		MaxAttempts: 2, // Will fail on 2nd attempt
		CreatedAt:   time.Now(),
	}
	_ = jobRepo.Save(invalidJob)
	jobSched.Enqueue(invalidJob)

	// Wait for processing
	time.Sleep(200 * time.Millisecond)
	jDrift, _ := jobRepo.GetByID("job-drift")
	if jDrift.State != domain.StateSucceeded {
		t.Fatalf("expected drift job to succeed, got %s", jDrift.State)
	}
	jPower, _ := jobRepo.GetByID("job-power")
	if jPower.State != domain.StateSucceeded {
		t.Fatalf("expected power job to succeed, got %s", jPower.State)
	}
	jInvalid, _ := jobRepo.GetByID("job-invalid")
	if jInvalid.State != domain.StateFailed {
		t.Fatalf("expected invalid job to fail, got %s", jInvalid.State)
	}
}

