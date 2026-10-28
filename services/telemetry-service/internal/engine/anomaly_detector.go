// Package engine provides online EWMA and rolling z-score anomaly detection.
package engine

import (
	"fmt"
	"math"
	"sort"
	"sync"

	"github.com/fleetpulse/fleetpulse/pkg/ds/slidingwindow"
	"github.com/fleetpulse/fleetpulse/services/telemetry-service/internal/domain"
)

// AnomalyEngine maintains online sliding-window statistical detectors for each device metric.
type AnomalyEngine struct {
	mu           sync.RWMutex
	detectors    map[string]*slidingwindow.Detector // "deviceID:metric" -> detector
	topAnomalies map[string]domain.AnomalousDevice  // deviceID -> anomalous device
	warmup       int
	zThreshold   float64
	alpha        float64
}

// NewAnomalyEngine initializes the detector with specified warm-up and z-score threshold.
func NewAnomalyEngine(warmup int, zThreshold, alpha float64) *AnomalyEngine {
	if warmup <= 0 {
		warmup = 15
	}
	if zThreshold <= 0 {
		zThreshold = 3.0
	}
	if alpha <= 0 {
		alpha = 0.15
	}
	return &AnomalyEngine{
		detectors:    make(map[string]*slidingwindow.Detector),
		topAnomalies: make(map[string]domain.AnomalousDevice),
		warmup:       warmup,
		zThreshold:   zThreshold,
		alpha:        alpha,
	}
}

// Process evaluates a telemetry sample for statistical anomalies.
func (e *AnomalyEngine) Process(sample domain.TelemetrySample) []*domain.AnomalousDevice {
	e.mu.Lock()
	defer e.mu.Unlock()

	metrics := map[string]float64{
		"cpu_temp":   sample.CPUTemp,
		"inlet_temp": sample.InletTemp,
	}

	var detected []*domain.AnomalousDevice

	for metricName, val := range metrics {
		key := fmt.Sprintf("%s:%s", sample.DeviceID, metricName)
		det, exists := e.detectors[key]
		if !exists {
			det = slidingwindow.NewDetector(slidingwindow.DetectorConfig{
				Alpha:           e.alpha,
				ZScoreThreshold: e.zThreshold,
				WarmupSamples:   int64(e.warmup),
				MinStdDev:       0.2,
			})
			e.detectors[key] = det
		}

		res := det.Update(val)
		if res.IsAnomaly {
			explanation := fmt.Sprintf("observed %.2f %s exceeds expected %.2f +/- %.2f (z-score: %.2f)",
				val, metricName, res.ExpectedMean, res.ExpectedStdDev*e.zThreshold, res.ZScore)

			dev := &domain.AnomalousDevice{
				DeviceID:     sample.DeviceID,
				Serial:       sample.Serial,
				MetricName:   metricName,
				AnomalyScore: math.Abs(res.ZScore),
				Explanation:  explanation,
			}
			e.topAnomalies[sample.DeviceID] = *dev
			detected = append(detected, dev)
		}
	}

	return detected
}

// TopAnomalies returns the highest scoring anomalous devices up to limit.
func (e *AnomalyEngine) TopAnomalies(limit int) []domain.AnomalousDevice {
	e.mu.RLock()
	defer e.mu.RUnlock()

	if limit <= 0 {
		limit = 5
	}

	list := make([]domain.AnomalousDevice, 0, len(e.topAnomalies))
	for _, a := range e.topAnomalies {
		list = append(list, a)
	}

	sort.Slice(list, func(i, j int) bool {
		return list[i].AnomalyScore > list[j].AnomalyScore
	})

	if len(list) > limit {
		list = list[:limit]
	}
	return list
}
