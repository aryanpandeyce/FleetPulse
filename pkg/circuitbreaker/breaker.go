// Package circuitbreaker implements a thread-safe three-state circuit breaker pattern.
package circuitbreaker

import (
	"errors"
	"sync"
	"time"
)

var (
	ErrCircuitOpen = errors.New("circuit breaker is open: request rejected")
)

// State represents the circuit breaker operating state.
type State string

const (
	StateClosed   State = "closed"
	StateHalfOpen State = "half-open"
	StateOpen     State = "open"
)

// Config configures circuit breaker thresholds.
type Config struct {
	FailureThreshold int           // consecutive failures to trip open
	SuccessThreshold int           // consecutive successes in half-open to reset closed
	Cooldown         time.Duration // open duration before transitioning to half-open
}

// CircuitBreaker guards downstream RPCs against cascading failures.
type CircuitBreaker struct {
	mu               sync.Mutex
	cfg              Config
	state            State
	failures         int
	successes        int
	lastStateChange  time.Time
}

// New creates a circuit breaker with specified configuration.
func New(cfg Config) *CircuitBreaker {
	if cfg.FailureThreshold <= 0 {
		cfg.FailureThreshold = 5
	}
	if cfg.SuccessThreshold <= 0 {
		cfg.SuccessThreshold = 2
	}
	if cfg.Cooldown <= 0 {
		cfg.Cooldown = 5 * time.Second
	}
	return &CircuitBreaker{
		cfg:             cfg,
		state:           StateClosed,
		lastStateChange: time.Now(),
	}
}

// Execute wraps an operation, tracking successes/failures and failing fast if Open.
func (cb *CircuitBreaker) Execute(fn func() error) error {
	cb.mu.Lock()

	// Check if Open cooldown has elapsed -> transition to Half-Open probe
	if cb.state == StateOpen {
		if time.Since(cb.lastStateChange) >= cb.cfg.Cooldown {
			cb.state = StateHalfOpen
			cb.successes = 0
			cb.lastStateChange = time.Now()
		} else {
			cb.mu.Unlock()
			return ErrCircuitOpen
		}
	}

	cb.mu.Unlock()

	// Execute protected operation
	err := fn()

	cb.mu.Lock()
	defer cb.mu.Unlock()

	if err != nil {
		cb.onFailure()
		return err
	}

	cb.onSuccess()
	return nil
}

// State returns current circuit state.
func (cb *CircuitBreaker) State() State {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	if cb.state == StateOpen && time.Since(cb.lastStateChange) >= cb.cfg.Cooldown {
		return StateHalfOpen
	}
	return cb.state
}

func (cb *CircuitBreaker) onFailure() {
	cb.failures++
	cb.successes = 0

	if cb.state == StateHalfOpen || cb.failures >= cb.cfg.FailureThreshold {
		cb.state = StateOpen
		cb.lastStateChange = time.Now()
	}
}

func (cb *CircuitBreaker) onSuccess() {
	if cb.state == StateHalfOpen {
		cb.successes++
		if cb.successes >= cb.cfg.SuccessThreshold {
			cb.state = StateClosed
			cb.failures = 0
			cb.successes = 0
			cb.lastStateChange = time.Now()
		}
	} else if cb.state == StateClosed {
		cb.failures = 0
	}
}
