// Package slidingwindow provides real-time streaming analytics primitives including
// Exponentially Weighted Moving Average (EWMA) control charts, rolling variance tracking,
// and statistical z-score anomaly detection.
//
// Time Complexity:
//   - Update: O(1) - single-pass scalar arithmetic update.
//   - ZScore: O(1) - constant time comparison against current distribution.
//
// Space Complexity:
//   - O(1) - constant memory footprint per detector with zero dynamic allocations.
package slidingwindow

import (
	"fmt"
	"math"
	"sync"
)

// AnomalyReport encapsulates the results of evaluating a metric sample.
type AnomalyReport struct {
	IsAnomaly      bool    // true if absolute z-score exceeds threshold post-warmup
	ZScore         float64 // number of standard deviations from the running mean
	ObservedValue  float64 // value presented for evaluation
	ExpectedMean   float64 // current EWMA mean prior to this sample
	ExpectedStdDev float64 // current running standard deviation
	MinExpected    float64 // mean - (threshold * stddev)
	MaxExpected    float64 // mean + (threshold * stddev)
	SampleCount    int64   // total number of samples processed
	Explanation    string  // human-readable diagnostic description
}

// DetectorConfig defines tuning parameters for statistical anomaly detection.
type DetectorConfig struct {
	MetricName      string  // identifier (e.g. "inlet_temp_celsius", "fan_rpm")
	Alpha           float64 // smoothing factor between 0.0 and 1.0 (default 0.1)
	ZScoreThreshold float64 // number of standard deviations for alarm (default 3.0)
	WarmupSamples   int64   // minimum samples required before raising anomalies (default 30)
	MinStdDev       float64 // floor to prevent division by zero in zero-variance streams (default 0.01)
}

// DefaultConfig returns recommended production defaults.
func DefaultConfig(metricName string) DetectorConfig {
	return DetectorConfig{
		MetricName:      metricName,
		Alpha:           0.1,
		ZScoreThreshold: 3.0,
		WarmupSamples:   30,
		MinStdDev:       0.05,
	}
}

// Detector tracks running statistical moments for a single time series.
// Thread-safe for concurrent calls.
type Detector struct {
	mu     sync.RWMutex
	cfg    DetectorConfig
	count  int64
	mean   float64
	varEst float64 // running variance estimate
}

// NewDetector creates a statistical anomaly detector with the given configuration.
func NewDetector(cfg DetectorConfig) *Detector {
	if cfg.Alpha <= 0 || cfg.Alpha >= 1 {
		cfg.Alpha = 0.1
	}
	if cfg.ZScoreThreshold <= 0 {
		cfg.ZScoreThreshold = 3.0
	}
	if cfg.WarmupSamples <= 0 {
		cfg.WarmupSamples = 30
	}
	if cfg.MinStdDev <= 0 {
		cfg.MinStdDev = 0.05
	}
	return &Detector{
		cfg: cfg,
	}
}

// Update evaluates a newly arrived metric sample in O(1) time and updates internal distribution moments.
func (d *Detector) Update(val float64) AnomalyReport {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.count++

	// First sample initializes the baseline
	if d.count == 1 {
		d.mean = val
		d.varEst = d.cfg.MinStdDev * d.cfg.MinStdDev
		return AnomalyReport{
			IsAnomaly:      false,
			ZScore:         0,
			ObservedValue:  val,
			ExpectedMean:   val,
			ExpectedStdDev: d.cfg.MinStdDev,
			MinExpected:    val,
			MaxExpected:    val,
			SampleCount:    1,
			Explanation:    fmt.Sprintf("Baseline initialized with first sample %.2f", val),
		}
	}

	priorMean := d.mean
	stdDev := math.Sqrt(d.varEst)
	if stdDev < d.cfg.MinStdDev {
		stdDev = d.cfg.MinStdDev
	}

	// Calculate z-score relative to prior established distribution
	diff := val - priorMean
	zScore := diff / stdDev

	minExp := priorMean - (d.cfg.ZScoreThreshold * stdDev)
	maxExp := priorMean + (d.cfg.ZScoreThreshold * stdDev)

	isAnomaly := false
	var explanation string

	// Anomaly is only raised if beyond warmup period and exceeds z-threshold
	if d.count > d.cfg.WarmupSamples && math.Abs(zScore) >= d.cfg.ZScoreThreshold {
		isAnomaly = true
		explanation = fmt.Sprintf(
			"Anomaly detected in %s: observed %.2f deviates by %.2f sigma from mean %.2f (expected range: [%.2f, %.2f])",
			d.cfg.MetricName, val, zScore, priorMean, minExp, maxExp,
		)
	} else if d.count <= d.cfg.WarmupSamples {
		explanation = fmt.Sprintf("Warming up (%d/%d samples)", d.count, d.cfg.WarmupSamples)
	} else {
		explanation = fmt.Sprintf("Normal: z-score %.2f within threshold +/-%.1f", zScore, d.cfg.ZScoreThreshold)
	}

	// Update EWMA mean and variance using Finch-West recursive online formulas:
	// mean_t = mean_{t-1} + alpha * (val - mean_{t-1})
	// var_t = (1 - alpha) * (var_{t-1} + alpha * (val - mean_{t-1})^2)
	alpha := d.cfg.Alpha
	d.mean = priorMean + (alpha * diff)
	d.varEst = (1.0 - alpha) * (d.varEst + (alpha * diff * diff))

	return AnomalyReport{
		IsAnomaly:      isAnomaly,
		ZScore:         zScore,
		ObservedValue:  val,
		ExpectedMean:   priorMean,
		ExpectedStdDev: stdDev,
		MinExpected:    minExp,
		MaxExpected:    maxExp,
		SampleCount:    d.count,
		Explanation:    explanation,
	}
}

// CurrentStats returns current running mean, standard deviation, and sample count.
func (d *Detector) CurrentStats() (mean, stdDev float64, samples int64) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	sd := math.Sqrt(d.varEst)
	if sd < d.cfg.MinStdDev {
		sd = d.cfg.MinStdDev
	}
	return d.mean, sd, d.count
}

// Reset clears the detector state.
func (d *Detector) Reset() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.count = 0
	d.mean = 0
	d.varEst = 0
}
