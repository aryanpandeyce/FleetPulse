// Package scheduler implements the job priority queue backed by custom generic min-heap.
package scheduler

import (
	"sync"

	"github.com/fleetpulse/fleetpulse/pkg/ds/minheap"
	"github.com/fleetpulse/fleetpulse/services/workflow-service/internal/domain"
)

// PriorityScheduler manages job queuing ordered by priority and creation timestamp.
type PriorityScheduler struct {
	mu   sync.Mutex
	heap *minheap.Heap[*domain.Job]
}

// NewPriorityScheduler creates a scheduler using custom generic min-heap.
func NewPriorityScheduler() *PriorityScheduler {
	// Comparator: lower numerical priority first (1 < 2 < 3); tie-breaker is FIFO by CreatedAt
	less := func(a, b *domain.Job) bool {
		if a.Priority != b.Priority {
			return a.Priority < b.Priority
		}
		return a.CreatedAt.Before(b.CreatedAt)
	}

	return &PriorityScheduler{
		heap: minheap.New(less),
	}
}

// Enqueue adds a job to the priority queue.
func (s *PriorityScheduler) Enqueue(job *domain.Job) {
	s.heap.Push(job)
}

// Dequeue extracts the highest priority job, returning false if the queue is empty.
func (s *PriorityScheduler) Dequeue() (*domain.Job, bool) {
	return s.heap.Pop()
}

// Len returns the current count of queued jobs.
func (s *PriorityScheduler) Len() int {
	return s.heap.Len()
}
