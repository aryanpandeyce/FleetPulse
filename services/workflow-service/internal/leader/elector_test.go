package leader

import (
	"context"
	"testing"
	"time"

	"github.com/fleetpulse/fleetpulse/services/workflow-service/internal/domain"
	"github.com/fleetpulse/fleetpulse/services/workflow-service/internal/repo"
)

func TestLeaderElectionAndFailover(t *testing.T) {
	store := NewMemoryLeaseStore()
	ctx := context.Background()

	node1 := NewElector(store, "test:lease", "node-1", 100*time.Millisecond)
	node2 := NewElector(store, "test:lease", "node-2", 100*time.Millisecond)

	// 1. Node 1 acquires leadership
	isLeader, err := node1.TryAcquire(ctx)
	if err != nil || !isLeader {
		t.Fatalf("expected node-1 to acquire leadership: %v", err)
	}

	// 2. Node 2 attempts to acquire -> rejected!
	isLeader, err = node2.TryAcquire(ctx)
	if err != nil || isLeader {
		t.Fatalf("expected node-2 to be rejected while node-1 holds lease")
	}

	// 3. Node 1 renews leadership
	isLeader, err = node1.TryAcquire(ctx)
	if err != nil || !isLeader {
		t.Fatalf("expected node-1 to renew its lease")
	}

	// 4. Failover scenario: Node 1 steps down gracefully (SIGTERM)
	if err := node1.StepDown(ctx); err != nil {
		t.Fatalf("failed to step down: %v", err)
	}

	// 5. Node 2 successfully acquires leadership on failover!
	isLeader, err = node2.TryAcquire(ctx)
	if err != nil || !isLeader {
		t.Fatalf("expected node-2 to acquire leadership on failover: %v", err)
	}
}

func TestRecoverStuckJobs(t *testing.T) {
	r := repo.NewRepository()
	now := time.Now()

	// Stuck job: started 10 minutes ago, still running
	stuckJob := &domain.Job{
		ID:        "job-stuck",
		OrgID:     "org-1",
		State:     domain.StateRunning,
		StartedAt: now.Add(-10 * time.Minute),
	}
	_ = r.Save(stuckJob)

	// Fresh job: started 30 seconds ago, still running
	freshJob := &domain.Job{
		ID:        "job-fresh",
		OrgID:     "org-1",
		State:     domain.StateRunning,
		StartedAt: now.Add(-30 * time.Second),
	}
	_ = r.Save(freshJob)

	// Recover jobs stuck > 5 minutes
	recovered := RecoverStuckJobs(r, 5*time.Minute)
	if recovered != 1 {
		t.Fatalf("expected 1 recovered job, got %d", recovered)
	}

	j1, _ := r.GetByID("job-stuck")
	if j1.State != domain.StateQueued {
		t.Fatalf("expected stuck job to be re-queued, got %s", j1.State)
	}

	j2, _ := r.GetByID("job-fresh")
	if j2.State != domain.StateRunning {
		t.Fatalf("fresh job should remain running, got %s", j2.State)
	}
}
