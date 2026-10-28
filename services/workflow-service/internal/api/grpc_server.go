// Package api implements the gRPC server for WorkflowService.
package api

import (
	"context"
	"time"

	"github.com/fleetpulse/fleetpulse/api/proto/workflowv1"
	"github.com/fleetpulse/fleetpulse/pkg/logging"
	"github.com/fleetpulse/fleetpulse/services/workflow-service/internal/domain"
	"github.com/fleetpulse/fleetpulse/services/workflow-service/internal/repo"
	"github.com/fleetpulse/fleetpulse/services/workflow-service/internal/scheduler"
	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Server implements workflowv1.WorkflowServiceServer.
type Server struct {
	workflowv1.UnimplementedWorkflowServiceServer

	repo      *repo.Repository
	scheduler *scheduler.PriorityScheduler
}

// NewServer creates a new WorkflowService gRPC server.
func NewServer(repo *repo.Repository, sched *scheduler.PriorityScheduler) *Server {
	return &Server{
		repo:      repo,
		scheduler: sched,
	}
}

// CreateJob enqueues a new asynchronous workflow task with idempotency verification.
func (s *Server) CreateJob(ctx context.Context, in *workflowv1.CreateJobRequest) (*workflowv1.CreateJobResponse, error) {
	orgID := logging.OrgIDFromContext(ctx)
	if orgID == "" {
		return nil, status.Error(codes.Unauthenticated, "org_id metadata required")
	}

	if in.Type == "" || len(in.Targets) == 0 {
		return nil, status.Error(codes.InvalidArgument, "type and targets are required")
	}

	// 1. Idempotency Check: if identical key exists for tenant, return existing job
	if in.IdempotencyKey != "" {
		if existing, exists := s.repo.GetByIdempotency(orgID, in.IdempotencyKey); exists {
			return &workflowv1.CreateJobResponse{
				Job: toProtoJob(existing),
			}, nil
		}
	}

	priority := in.Priority
	if priority <= 0 {
		priority = domain.PriorityNormal
	}

	job := &domain.Job{
		ID:             "job-" + uuid.NewString()[:8],
		OrgID:          orgID,
		Type:           in.Type,
		Priority:       priority,
		State:          domain.StateQueued,
		Targets:        in.Targets,
		Parameters:     in.Parameters,
		IdempotencyKey: in.IdempotencyKey,
		Attempts:       0,
		MaxAttempts:    3,
		StepHistory:    make([]domain.StepRecord, 0),
		CreatedAt:      time.Now(),
	}

	// Persist and enqueue to priority min-heap
	if err := s.repo.Save(job); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	s.scheduler.Enqueue(job)

	return &workflowv1.CreateJobResponse{
		Job: toProtoJob(job),
	}, nil
}

// GetJob retrieves the current state and step history of a workflow.
func (s *Server) GetJob(ctx context.Context, in *workflowv1.GetJobRequest) (*workflowv1.GetJobResponse, error) {
	if in.Id == "" {
		return nil, status.Error(codes.InvalidArgument, "id is required")
	}

	orgID := logging.OrgIDFromContext(ctx)
	job, err := s.repo.GetByID(in.Id)
	if err != nil {
		return nil, status.Error(codes.NotFound, "job not found")
	}

	if orgID != "" && job.OrgID != orgID {
		return nil, status.Error(codes.NotFound, "job not found") // Tenant isolation
	}

	return &workflowv1.GetJobResponse{
		Job: toProtoJob(job),
	}, nil
}

// CancelJob terminates or revokes an uncompleted workflow.
func (s *Server) CancelJob(ctx context.Context, in *workflowv1.CancelJobRequest) (*workflowv1.CancelJobResponse, error) {
	if in.Id == "" {
		return nil, status.Error(codes.InvalidArgument, "id is required")
	}

	orgID := logging.OrgIDFromContext(ctx)
	job, err := s.repo.GetByID(in.Id)
	if err != nil {
		return nil, status.Error(codes.NotFound, "job not found")
	}

	if orgID != "" && job.OrgID != orgID {
		return nil, status.Error(codes.NotFound, "job not found")
	}

	if job.IsTerminal() {
		return &workflowv1.CancelJobResponse{
			Success: false,
			Message: "job is already in terminal state",
		}, nil
	}

	_ = s.repo.UpdateState(job.ID, domain.StateCancelled, "cancelled by operator")

	return &workflowv1.CancelJobResponse{
		Success: true,
		Message: "job successfully cancelled",
	}, nil
}

// ListJobs retrieves jobs matching criteria under tenant isolation.
func (s *Server) ListJobs(ctx context.Context, in *workflowv1.ListJobsRequest) (*workflowv1.ListJobsResponse, error) {
	orgID := logging.OrgIDFromContext(ctx)

	jobs := s.repo.List(orgID, in.State, int(in.Limit))
	protoJobs := make([]*workflowv1.Job, len(jobs))
	for i, j := range jobs {
		protoJobs[i] = toProtoJob(j)
	}

	return &workflowv1.ListJobsResponse{
		Jobs: protoJobs,
	}, nil
}

func toProtoJob(j *domain.Job) *workflowv1.Job {
	steps := make([]*workflowv1.StepRecord, len(j.StepHistory))
	for i, s := range j.StepHistory {
		steps[i] = &workflowv1.StepRecord{
			StepName:      s.StepName,
			Status:        s.Status,
			Target:        s.Target,
			Message:       s.Message,
			TimestampUnix: s.Timestamp.Unix(),
		}
	}

	var startedUnix, completedUnix int64
	if !j.StartedAt.IsZero() {
		startedUnix = j.StartedAt.Unix()
	}
	if !j.CompletedAt.IsZero() {
		completedUnix = j.CompletedAt.Unix()
	}

	return &workflowv1.Job{
		Id:              j.ID,
		OrgId:           j.OrgID,
		Type:            j.Type,
		Priority:        j.Priority,
		State:           j.State,
		Targets:         j.Targets,
		Parameters:      j.Parameters,
		IdempotencyKey:  j.IdempotencyKey,
		Attempts:        j.Attempts,
		MaxAttempts:     j.MaxAttempts,
		StepHistory:     steps,
		ErrorMessage:    j.ErrorMessage,
		CreatedAtUnix:   j.CreatedAt.Unix(),
		StartedAtUnix:   startedUnix,
		CompletedAtUnix: completedUnix,
	}
}
