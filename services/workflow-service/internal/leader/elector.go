// Package leader implements distributed leader election and stuck job crash recovery.
package leader

import (
	"context"
	"sync"
	"time"

	"github.com/fleetpulse/fleetpulse/services/workflow-service/internal/domain"
	"github.com/fleetpulse/fleetpulse/services/workflow-service/internal/repo"
)

// LeaseStore defines key-value lease operations.
type LeaseStore interface {
	SetNX(key, value string, ttl time.Duration) (bool, error)
	Get(key string) (string, error)
	Del(key string) error
}

// MemoryLeaseStore is an in-memory thread-safe lease store for local runs and unit tests.
type MemoryLeaseStore struct {
	mu     sync.Mutex
	leases map[string]leaseEntry
}

type leaseEntry struct {
	holder    string
	expiresAt time.Time
}

func NewMemoryLeaseStore() *MemoryLeaseStore {
	return &MemoryLeaseStore{
		leases: make(map[string]leaseEntry),
	}
}

func (m *MemoryLeaseStore) SetNX(key, value string, ttl time.Duration) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	if entry, exists := m.leases[key]; exists && entry.expiresAt.After(now) {
		if entry.holder == value {
			// Renew existing lease
			m.leases[key] = leaseEntry{holder: value, expiresAt: now.Add(ttl)}
			return true, nil
		}
		return false, nil // Held by someone else
	}

	m.leases[key] = leaseEntry{holder: value, expiresAt: now.Add(ttl)}
	return true, nil
}

func (m *MemoryLeaseStore) Get(key string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	entry, exists := m.leases[key]
	if !exists || time.Now().After(entry.expiresAt) {
		return "", nil
	}
	return entry.holder, nil
}

func (m *MemoryLeaseStore) Del(key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.leases, key)
	return nil
}

// Elector manages leader election state and scheduled sweeps.
type Elector struct {
	store      LeaseStore
	leaseKey   string
	instanceID string
	ttl        time.Duration
}

// NewElector constructs a leader election manager.
func NewElector(store LeaseStore, leaseKey, instanceID string, ttl time.Duration) *Elector {
	if ttl <= 0 {
		ttl = 10 * time.Second
	}
	if leaseKey == "" {
		leaseKey = "fleetpulse:leader:workflow"
	}
	return &Elector{
		store:      store,
		leaseKey:   leaseKey,
		instanceID: instanceID,
		ttl:        ttl,
	}
}

// TryAcquire attempts to acquire or renew leadership.
func (e *Elector) TryAcquire(ctx context.Context) (bool, error) {
	return e.store.SetNX(e.leaseKey, e.instanceID, e.ttl)
}

// StepDown releases leadership voluntarily (e.g. on SIGTERM).
func (e *Elector) StepDown(ctx context.Context) error {
	holder, err := e.store.Get(e.leaseKey)
	if err == nil && holder == e.instanceID {
		return e.store.Del(e.leaseKey)
	}
	return nil
}

// RecoverStuckJobs detects jobs abandoned in 'running' state past the timeout and re-queues them.
func RecoverStuckJobs(r *repo.Repository, timeout time.Duration) int {
	recovered := 0
	runningJobs := r.List("", domain.StateRunning, 100)
	now := time.Now()

	for _, job := range runningJobs {
		if !job.StartedAt.IsZero() && now.Sub(job.StartedAt) > timeout {
			_ = r.UpdateState(job.ID, domain.StateQueued, "recovered by crash reaper: previous runner timed out")
			recovered++
		}
	}
	return recovered
}
