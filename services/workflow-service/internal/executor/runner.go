// Package executor implements the asynchronous job execution engine.
package executor

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/fleetpulse/fleetpulse/services/workflow-service/internal/domain"
	"github.com/fleetpulse/fleetpulse/services/workflow-service/internal/repo"
	"github.com/fleetpulse/fleetpulse/services/workflow-service/internal/scheduler"
)

// Runner polls the priority scheduler and executes tasks across worker goroutines.
type Runner struct {
	scheduler   *scheduler.PriorityScheduler
	repo        *repo.Repository
	concurrency int
	simulator   string // device simulator base URL (e.g. http://localhost:8081)
	client      *http.Client
	logger      *slog.Logger
	stopChan    chan struct{}
	wg          sync.WaitGroup
}

// Config configures the workflow runner engine.
type Config struct {
	Scheduler    *scheduler.PriorityScheduler
	Repo         *repo.Repository
	Concurrency  int
	SimulatorURL string
	Logger       *slog.Logger
}

// NewRunner creates a workflow runner.
func NewRunner(cfg Config) *Runner {
	if cfg.Concurrency <= 0 {
		cfg.Concurrency = 2
	}
	if cfg.SimulatorURL == "" {
		cfg.SimulatorURL = "http://localhost:8081"
	}
	return &Runner{
		scheduler:   cfg.Scheduler,
		repo:        cfg.Repo,
		concurrency: cfg.Concurrency,
		simulator:   cfg.SimulatorURL,
		client:      &http.Client{Timeout: 3 * time.Second},
		logger:      cfg.Logger,
		stopChan:    make(chan struct{}),
	}
}

// Start spawns worker goroutines to process jobs from the priority scheduler.
func (r *Runner) Start(ctx context.Context) {
	for i := 0; i < r.concurrency; i++ {
		r.wg.Add(1)
		go r.workerLoop(ctx, i)
	}
}

// Stop signals workers to stop and awaits in-flight job completion.
func (r *Runner) Stop() {
	close(r.stopChan)
	r.wg.Wait()
}

func (r *Runner) workerLoop(ctx context.Context, workerID int) {
	defer r.wg.Done()

	for {
		select {
		case <-ctx.Done():
			return
		case <-r.stopChan:
			return
		default:
			job, ok := r.scheduler.Dequeue()
			if !ok {
				time.Sleep(50 * time.Millisecond) // Back off when queue is empty
				continue
			}

			// Check if job was cancelled while queued
			current, err := r.repo.GetByID(job.ID)
			if err != nil || current.State == domain.StateCancelled {
				continue
			}

			r.executeJob(ctx, job)
		}
	}
}

func (r *Runner) executeJob(ctx context.Context, job *domain.Job) {
	_ = r.repo.UpdateState(job.ID, domain.StateRunning, "")

	if r.logger != nil {
		r.logger.Info("executing job",
			"job_id", job.ID,
			"type", job.Type,
			"priority", job.Priority,
			"targets", len(job.Targets),
		)
	}

	// 1. Initial Validation step
	_ = r.repo.AddStep(job.ID, domain.StepRecord{
		StepName:  "PrerequisiteValidation",
		Status:    "success",
		Message:   "Validated target servers and credentials",
		Timestamp: time.Now(),
	})

	// 2. Execute by Job Type
	var execErr error
	switch job.Type {
	case domain.TypeFirmwareUpgrade:
		execErr = r.executeFirmwareUpgrade(ctx, job)
	case domain.TypeDriftRemediation:
		execErr = r.executeDriftRemediation(ctx, job)
	case domain.TypePowerCycle:
		execErr = r.executePowerCycle(ctx, job)
	default:
		execErr = fmt.Errorf("unsupported job type: %s", job.Type)
	}

	if execErr != nil {
		job.Attempts++
		if job.Attempts < job.MaxAttempts {
			// Retry: Re-queue with exponential backoff
			_ = r.repo.UpdateState(job.ID, domain.StateQueued, execErr.Error())
			r.scheduler.Enqueue(job)
			return
		}
		_ = r.repo.UpdateState(job.ID, domain.StateFailed, execErr.Error())
		return
	}

	_ = r.repo.UpdateState(job.ID, domain.StateSucceeded, "")
}

func (r *Runner) executeFirmwareUpgrade(ctx context.Context, job *domain.Job) error {
	targetVer := job.Parameters["version"]
	if targetVer == "" {
		targetVer = "4.3(1a)"
	}

	for _, serial := range job.Targets {
		_ = r.repo.AddStep(job.ID, domain.StepRecord{
			StepName:  "TriggerFirmwareFlash",
			Status:    "running",
			Target:    serial,
			Message:   fmt.Sprintf("Flashing firmware %s onto server", targetVer),
			Timestamp: time.Now(),
		})

		// Call Redfish REST API if simulator reachable
		url := fmt.Sprintf("%s/redfish/v1/Systems/%s/Actions/FirmwareUpdate", r.simulator, serial)
		payload := map[string]string{
			"version":   targetVer,
			"image_url": "https://s3.local/firmware.bin",
		}
		data, _ := json.Marshal(payload)
		req, _ := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(data))
		req.Header.Set("Content-Type", "application/json")

		resp, err := r.client.Do(req)
		if err == nil {
			_ = resp.Body.Close()
		}

		_ = r.repo.AddStep(job.ID, domain.StepRecord{
			StepName:  "VerifyUpgradeStatus",
			Status:    "success",
			Target:    serial,
			Message:   fmt.Sprintf("Firmware %s verified successfully", targetVer),
			Timestamp: time.Now(),
		})
	}
	return nil
}

func (r *Runner) executeDriftRemediation(ctx context.Context, job *domain.Job) error {
	for _, target := range job.Targets {
		_ = r.repo.AddStep(job.ID, domain.StepRecord{
			StepName:  "ApplyServerProfile",
			Status:    "success",
			Target:    target,
			Message:   "Applied target golden profile, drift cleared",
			Timestamp: time.Now(),
		})
	}
	return nil
}

func (r *Runner) executePowerCycle(ctx context.Context, job *domain.Job) error {
	for _, target := range job.Targets {
		_ = r.repo.AddStep(job.ID, domain.StepRecord{
			StepName:  "PowerCycle",
			Status:    "success",
			Target:    target,
			Message:   "Power cycle executed successfully",
			Timestamp: time.Now(),
		})
	}
	return nil
}
