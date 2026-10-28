// Package repo provides ring buffer storage for recent device telemetry.
package repo

import (
	"sync"

	"github.com/fleetpulse/fleetpulse/services/telemetry-service/internal/domain"
)

// RingBuffer keeps the last N telemetry samples per device in memory for sub-millisecond retrieval.
type RingBuffer struct {
	mu           sync.RWMutex
	capacity     int
	buffers      map[string][]domain.TelemetrySample // deviceID -> samples (newest first)
	latestHealth map[string]string                   // deviceID -> "OK" / "Warning" / "Critical"
}

// NewRingBuffer initializes ring storage holding up to capacity samples per device.
func NewRingBuffer(capacity int) *RingBuffer {
	if capacity <= 0 {
		capacity = 120
	}
	return &RingBuffer{
		capacity:     capacity,
		buffers:      make(map[string][]domain.TelemetrySample),
		latestHealth: make(map[string]string),
	}
}

// Push adds a new sample to the head of the device's ring buffer, trimming oldest entries.
func (r *RingBuffer) Push(sample domain.TelemetrySample) {
	r.mu.Lock()
	defer r.mu.Unlock()

	buf := r.buffers[sample.DeviceID]
	// Prepend newest sample
	buf = append([]domain.TelemetrySample{sample}, buf...)
	if len(buf) > r.capacity {
		buf = buf[:r.capacity]
	}
	r.buffers[sample.DeviceID] = buf

	// Determine latest health state
	health := "OK"
	if sample.CPUTemp >= 85.0 || sample.FanRPM[0] == 0 || sample.MemoryErrors >= 50 {
		health = "Critical"
	} else if sample.CPUTemp >= 72.0 || sample.InletTemp >= 32.0 || sample.MemoryErrors >= 10 {
		health = "Warning"
	}
	r.latestHealth[sample.DeviceID] = health
}

// GetRecent returns up to limit latest samples for a given device.
func (r *RingBuffer) GetRecent(deviceID string, limit int) []domain.TelemetrySample {
	r.mu.RLock()
	defer r.mu.RUnlock()

	buf, exists := r.buffers[deviceID]
	if !exists {
		return nil
	}

	if limit <= 0 || limit > len(buf) {
		limit = len(buf)
	}

	out := make([]domain.TelemetrySample, limit)
	copy(out, buf[:limit])
	return out
}

// GetFleetHealth aggregates device health totals across all monitored servers.
func (r *RingBuffer) GetFleetHealth() domain.FleetSummary {
	r.mu.RLock()
	defer r.mu.RUnlock()

	summary := domain.FleetSummary{
		TotalDevices: int64(len(r.latestHealth)),
	}

	for _, h := range r.latestHealth {
		switch h {
		case "Critical":
			summary.CriticalCount++
		case "Warning":
			summary.WarningCount++
		default:
			summary.OKCount++
		}
	}

	return summary
}
