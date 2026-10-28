// Package connector implements the bounded worker pool publisher streaming telemetry to NATS.
package connector

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math/rand"
	"sync"
	"time"

	"github.com/fleetpulse/fleetpulse/pkg/ds/consistenthash"
	"github.com/fleetpulse/fleetpulse/services/device-simulator/internal/engine"
	"github.com/fleetpulse/fleetpulse/services/device-simulator/internal/model"
)

// Publisher defines an interface to publish telemetry messages (satisfied by NATS or mock).
type Publisher interface {
	Publish(subject string, data []byte) error
}

// Config configures the device connector publisher.
type Config struct {
	NumWorkers int           // bounded worker count (e.g. 8)
	Interval   time.Duration // sample interval (e.g. 5s)
	JitterPct  float64       // jitter fraction (e.g. 0.1 for +/- 10%)
	Logger     *slog.Logger
	Publisher  Publisher
	Registry   *engine.DeviceRegistry
	Engine     *engine.SensorEngine
}

// Connector shards simulated devices across a bounded worker pool using consistent hashing.
type Connector struct {
	cfg      Config
	ring     *consistenthash.Ring
	workers  map[string][]*model.SimulatedDevice // worker-id -> sharded devices
	stopChan chan struct{}
	wg       sync.WaitGroup
	mu       sync.RWMutex
}

// NewConnector creates a connector partitioning devices across workers via consistent hashing.
func NewConnector(cfg Config) *Connector {
	if cfg.NumWorkers <= 0 {
		cfg.NumWorkers = 4
	}
	if cfg.Interval <= 0 {
		cfg.Interval = 5 * time.Second
	}
	if cfg.JitterPct <= 0 {
		cfg.JitterPct = 0.1
	}

	ring := consistenthash.New(50, nil)
	workers := make(map[string][]*model.SimulatedDevice)
	for i := 0; i < cfg.NumWorkers; i++ {
		workerID := fmt.Sprintf("worker-%d", i)
		ring.Add(workerID)
		workers[workerID] = make([]*model.SimulatedDevice, 0)
	}

	// Shard all registered devices onto worker queues using consistent hash
	for _, d := range cfg.Registry.All() {
		targetWorker, err := ring.Get(d.Serial)
		if err == nil {
			workers[targetWorker] = append(workers[targetWorker], d)
		}
	}

	return &Connector{
		cfg:      cfg,
		ring:     ring,
		workers:  workers,
		stopChan: make(chan struct{}),
	}
}

// Start spawns the bounded worker pool goroutines.
func (c *Connector) Start(ctx context.Context) {
	for workerID, devices := range c.workers {
		c.wg.Add(1)
		go c.runWorker(ctx, workerID, devices)
	}
}

// Stop signals all workers to terminate gracefully.
func (c *Connector) Stop() {
	close(c.stopChan)
	c.wg.Wait()
}

// WorkerCount returns number of workers.
func (c *Connector) WorkerCount() int {
	return c.cfg.NumWorkers
}

// ShardedDeviceCount returns the count of devices assigned to a specific worker.
func (c *Connector) ShardedDeviceCount(workerID string) int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.workers[workerID])
}

func (c *Connector) runWorker(ctx context.Context, workerID string, devices []*model.SimulatedDevice) {
	defer c.wg.Done()

	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	ticker := time.NewTicker(c.cfg.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-c.stopChan:
			return
		case <-ticker.C:
			// Process tick for each sharded device assigned to this worker
			for _, d := range devices {
				if d.GetFault() == model.FaultOffline {
					continue // Offline server emits no telemetry
				}

				// Advance device sensors
				c.cfg.Engine.Tick(d)

				// Create telemetry payload
				snap := d.Snapshot()
				data, err := json.Marshal(snap)
				if err != nil {
					continue
				}

				subject := fmt.Sprintf("telemetry.%s.%s", snap.OrgID, snap.Serial)
				if c.cfg.Publisher != nil {
					if err := c.cfg.Publisher.Publish(subject, data); err != nil {
						if c.cfg.Logger != nil {
							c.cfg.Logger.Warn("failed to publish telemetry", "subject", subject, "error", err)
						}
					}
				}

				// Apply micro-jitter to prevent CPU thundering herd across devices
				if c.cfg.JitterPct > 0 {
					jitterMax := float64(c.cfg.Interval) * c.cfg.JitterPct / float64(len(devices)+1)
					if jitterMax > 0 {
						time.Sleep(time.Duration(rng.Float64() * jitterMax))
					}
				}
			}
		}
	}
}
