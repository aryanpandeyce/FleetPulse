package api

import (
	"context"
	"testing"

	"github.com/fleetpulse/fleetpulse/api/proto/workflowv1"
	"github.com/fleetpulse/fleetpulse/pkg/logging"
	"github.com/fleetpulse/fleetpulse/services/workflow-service/internal/domain"
	"github.com/fleetpulse/fleetpulse/services/workflow-service/internal/repo"
	"github.com/fleetpulse/fleetpulse/services/workflow-service/internal/scheduler"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func setupTestWorkflowServer() (*Server, *repo.Repository, *scheduler.PriorityScheduler) {
	jobRepo := repo.NewRepository()
	jobSched := scheduler.NewPriorityScheduler()
	server := NewServer(jobRepo, jobSched)
	return server, jobRepo, jobSched
}

func TestWorkflowServer_CreateJobAndIdempotency(t *testing.T) {
	server, _, sched := setupTestWorkflowServer()
	ctx := logging.WithOrgID(context.Background(), "org-acme")

	// 1. Unauthenticated (no org_id)
	_, err := server.CreateJob(context.Background(), &workflowv1.CreateJobRequest{Type: "firmware_upgrade", Targets: []string{"UCS-1"}})
	if status.Code(err) != codes.Unauthenticated {
		t.Fatalf("expected Unauthenticated without org_id, got %v", err)
	}

	// 2. InvalidArgument (empty targets)
	_, err = server.CreateJob(ctx, &workflowv1.CreateJobRequest{Type: "firmware_upgrade", Targets: nil})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("expected InvalidArgument for empty targets, got %v", err)
	}

	// 3. Valid CreateJob
	req := &workflowv1.CreateJobRequest{
		Type:           domain.TypeFirmwareUpgrade,
		Priority:       1,
		Targets:        []string{"UCS-001", "UCS-002"},
		IdempotencyKey: "test-idem-key-100",
		Parameters:     map[string]string{"version": "4.3(1a)"},
	}
	resp1, err := server.CreateJob(ctx, req)
	if err != nil {
		t.Fatalf("failed to create job: %v", err)
	}
	if resp1.Job.Id == "" || resp1.Job.State != domain.StateQueued || resp1.Job.Priority != 1 {
		t.Fatalf("unexpected job response: %+v", resp1.Job)
	}
	if sched.Len() != 1 {
		t.Fatalf("expected 1 job in priority scheduler, got %d", sched.Len())
	}

	// 4. Idempotency Check: second call with identical key must return EXACT SAME job!
	resp2, err := server.CreateJob(ctx, req)
	if err != nil {
		t.Fatalf("failed on idempotent retry: %v", err)
	}
	if resp2.Job.Id != resp1.Job.Id {
		t.Fatalf("IDEMPOTENCY FAILURE: expected job ID %s, but got %s", resp1.Job.Id, resp2.Job.Id)
	}
	// Scheduler count should NOT have increased
	if sched.Len() != 1 {
		t.Fatalf("scheduler length changed on idempotent duplicate: %d", sched.Len())
	}
}

func TestWorkflowServer_GetAndCancelJob(t *testing.T) {
	server, _, _ := setupTestWorkflowServer()
	acmeCtx := logging.WithOrgID(context.Background(), "org-acme")
	globexCtx := logging.WithOrgID(context.Background(), "org-globex")

	// Create job under Acme
	createResp, err := server.CreateJob(acmeCtx, &workflowv1.CreateJobRequest{
		Type:    domain.TypePowerCycle,
		Targets: []string{"UCS-001"},
	})
	if err != nil {
		t.Fatalf("failed to create job: %v", err)
	}
	jobID := createResp.Job.Id

	// 1. Get Job under Acme
	getResp, err := server.GetJob(acmeCtx, &workflowv1.GetJobRequest{Id: jobID})
	if err != nil || getResp.Job.Id != jobID {
		t.Fatalf("failed to get job: %v", err)
	}

	// 2. Tenant Isolation: Globex user cannot see Acme job!
	_, err = server.GetJob(globexCtx, &workflowv1.GetJobRequest{Id: jobID})
	if status.Code(err) != codes.NotFound {
		t.Fatalf("TENANT LEAK: expected NotFound for other tenant, got %v", err)
	}

	// 3. Cancel Job
	cancelResp, err := server.CancelJob(acmeCtx, &workflowv1.CancelJobRequest{Id: jobID})
	if err != nil || !cancelResp.Success {
		t.Fatalf("failed to cancel job: %v, %+v", err, cancelResp)
	}

	// Verify status is now cancelled
	getRespAfter, _ := server.GetJob(acmeCtx, &workflowv1.GetJobRequest{Id: jobID})
	if getRespAfter.Job.State != domain.StateCancelled {
		t.Fatalf("expected job to be cancelled, got %s", getRespAfter.Job.State)
	}

	// 4. Cancel already cancelled job
	secondCancel, _ := server.CancelJob(acmeCtx, &workflowv1.CancelJobRequest{Id: jobID})
	if secondCancel.Success {
		t.Fatalf("expected cancel on terminal job to return success=false")
	}

	// 5. List Jobs
	listResp, err := server.ListJobs(acmeCtx, &workflowv1.ListJobsRequest{State: domain.StateCancelled})
	if err != nil || len(listResp.Jobs) != 1 {
		t.Fatalf("unexpected list response: %+v", listResp)
	}
}
