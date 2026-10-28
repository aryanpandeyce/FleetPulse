// Package repo provides storage and idempotency tracking for workflow jobs.
package repo

import (
	"fmt"
	"sync"
	"time"

	"github.com/fleetpulse/fleetpulse/services/workflow-service/internal/domain"
)

// Repository manages persistence and lookup of jobs.
type Repository struct {
	mu          sync.RWMutex
	jobs        map[string]*domain.Job // id -> job
	idempotency map[string]*domain.Job // "orgID:key" -> job
	order       []string
}

// NewRepository initializes job repository.
func NewRepository() *Repository {
	return &Repository{
		jobs:        make(map[string]*domain.Job),
		idempotency: make(map[string]*domain.Job),
		order:       make([]string, 0),
	}
}

// Save stores a new job or updates an existing job record.
func (r *Repository) Save(job *domain.Job) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.jobs[job.ID] = job
	if job.IdempotencyKey != "" {
		key := fmt.Sprintf("%s:%s", job.OrgID, job.IdempotencyKey)
		r.idempotency[key] = job
	}

	found := false
	for _, id := range r.order {
		if id == job.ID {
			found = true
			break
		}
	}
	if !found {
		r.order = append([]string{job.ID}, r.order...) // Prepend newest
	}

	return nil
}

// GetByID retrieves a job by its unique identifier.
func (r *Repository) GetByID(id string) (*domain.Job, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	job, exists := r.jobs[id]
	if !exists {
		return nil, domain.ErrJobNotFound
	}
	return job, nil
}

// GetByIdempotency retrieves an existing job created with the same idempotency key.
func (r *Repository) GetByIdempotency(orgID, idempotencyKey string) (*domain.Job, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	key := fmt.Sprintf("%s:%s", orgID, idempotencyKey)
	job, exists := r.idempotency[key]
	return job, exists
}

// List returns jobs matching orgID and optional state filter.
func (r *Repository) List(orgID, state string, limit int) []*domain.Job {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if limit <= 0 || limit > 100 {
		limit = 50
	}

	out := make([]*domain.Job, 0)
	for _, id := range r.order {
		job := r.jobs[id]
		if orgID != "" && job.OrgID != orgID {
			continue
		}
		if state != "" && job.State != state {
			continue
		}
		out = append(out, job)
		if len(out) >= limit {
			break
		}
	}
	return out
}

// UpdateState changes the execution status of a job.
func (r *Repository) UpdateState(id, state, errMsg string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	job, exists := r.jobs[id]
	if !exists {
		return domain.ErrJobNotFound
	}

	job.State = state
	if errMsg != "" {
		job.ErrorMessage = errMsg
	}
	if state == domain.StateRunning && job.StartedAt.IsZero() {
		job.StartedAt = time.Now()
	}
	if job.IsTerminal() && job.CompletedAt.IsZero() {
		job.CompletedAt = time.Now()
	}

	return nil
}

// AddStep appends an execution step milestone to the job's history.
func (r *Repository) AddStep(id string, step domain.StepRecord) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	job, exists := r.jobs[id]
	if !exists {
		return domain.ErrJobNotFound
	}

	job.StepHistory = append(job.StepHistory, step)
	return nil
}
