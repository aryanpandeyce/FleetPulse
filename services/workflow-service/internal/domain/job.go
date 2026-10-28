// Package domain defines workflow orchestration entities, priority queues, and execution steps.
package domain

import (
	"errors"
	"time"
)

var (
	ErrJobNotFound      = errors.New("job not found")
	ErrJobAlreadyDone   = errors.New("job is already in a terminal state")
	ErrInvalidJobType   = errors.New("invalid job type")
	ErrNoTargets        = errors.New("at least one target device is required")
)

// Supported job priorities (lower numerical value = higher urgency)
const (
	PriorityHigh   = 1
	PriorityNormal = 2
	PriorityLow    = 3
)

// Job state lifecycle
const (
	StateQueued    = "queued"
	StateRunning   = "running"
	StateSucceeded = "succeeded"
	StateFailed    = "failed"
	StateCancelled = "cancelled"
)

// Job types
const (
	TypeFirmwareUpgrade  = "firmware_upgrade"
	TypeDriftRemediation = "drift_remediation"
	TypePowerCycle       = "power_cycle"
)

// StepRecord documents a discrete milestone during job execution.
type StepRecord struct {
	StepName  string    `json:"step_name"`
	Status    string    `json:"status"` // "running", "success", "failed"
	Target    string    `json:"target"`
	Message   string    `json:"message"`
	Timestamp time.Time `json:"timestamp"`
}

// Job represents an asynchronous maintenance workflow.
type Job struct {
	ID             string            `json:"id"`
	OrgID          string            `json:"org_id"`
	Type           string            `json:"type"`
	Priority       int32             `json:"priority"` // 1=High, 2=Normal, 3=Low
	State          string            `json:"state"`
	Targets        []string          `json:"targets"`
	Parameters     map[string]string `json:"parameters"`
	IdempotencyKey string            `json:"idempotency_key"`
	Attempts       int32             `json:"attempts"`
	MaxAttempts    int32             `json:"max_attempts"`
	StepHistory    []StepRecord      `json:"step_history"`
	ErrorMessage   string            `json:"error_message,omitempty"`
	CreatedAt      time.Time         `json:"created_at"`
	StartedAt      time.Time         `json:"started_at,omitempty"`
	CompletedAt    time.Time         `json:"completed_at,omitempty"`
}

// IsTerminal returns true if the job has completed, failed, or was cancelled.
func (j *Job) IsTerminal() bool {
	return j.State == StateSucceeded || j.State == StateFailed || j.State == StateCancelled
}
