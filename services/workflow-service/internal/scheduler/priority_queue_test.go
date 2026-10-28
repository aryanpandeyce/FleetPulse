package scheduler

import (
	"testing"
	"time"

	"github.com/fleetpulse/fleetpulse/services/workflow-service/internal/domain"
)

func TestPriorityScheduler_Ordering(t *testing.T) {
	sched := NewPriorityScheduler()
	now := time.Now()

	jobLow := &domain.Job{ID: "j-low", Priority: domain.PriorityLow, CreatedAt: now.Add(1 * time.Minute)}
	jobNormalOld := &domain.Job{ID: "j-norm-old", Priority: domain.PriorityNormal, CreatedAt: now}
	jobNormalNew := &domain.Job{ID: "j-norm-new", Priority: domain.PriorityNormal, CreatedAt: now.Add(2 * time.Minute)}
	jobHigh := &domain.Job{ID: "j-high", Priority: domain.PriorityHigh, CreatedAt: now.Add(3 * time.Minute)}

	// Enqueue in arbitrary order
	sched.Enqueue(jobLow)
	sched.Enqueue(jobNormalNew)
	sched.Enqueue(jobHigh)
	sched.Enqueue(jobNormalOld)

	if sched.Len() != 4 {
		t.Fatalf("expected 4 jobs, got %d", sched.Len())
	}

	// 1st Dequeued must be jobHigh (Priority 1)
	p1, ok := sched.Dequeue()
	if !ok || p1.ID != "j-high" {
		t.Fatalf("expected j-high first, got %+v", p1)
	}

	// 2nd Dequeued must be jobNormalOld (Priority 2, earlier timestamp)
	p2, ok := sched.Dequeue()
	if !ok || p2.ID != "j-norm-old" {
		t.Fatalf("expected j-norm-old second, got %+v", p2)
	}

	// 3rd Dequeued must be jobNormalNew (Priority 2, later timestamp)
	p3, ok := sched.Dequeue()
	if !ok || p3.ID != "j-norm-new" {
		t.Fatalf("expected j-norm-new third, got %+v", p3)
	}

	// 4th Dequeued must be jobLow (Priority 3)
	p4, ok := sched.Dequeue()
	if !ok || p4.ID != "j-low" {
		t.Fatalf("expected j-low fourth, got %+v", p4)
	}

	// Empty queue
	_, ok = sched.Dequeue()
	if ok {
		t.Fatalf("expected false on empty dequeue")
	}
}
