package slidingwindow

import (
	"math"
	"math/rand"
	"sync"
	"testing"
)

func TestDetectorWarmup(t *testing.T) {
	cfg := DefaultConfig("inlet_temp_celsius")
	cfg.WarmupSamples = 20
	detector := NewDetector(cfg)

	// Feed 15 samples with large jumps during warmup
	for i := 1; i <= 15; i++ {
		val := 20.0
		if i == 10 {
			val = 90.0 // spike during warmup
		}
		rep := detector.Update(val)
		if rep.IsAnomaly {
			t.Fatalf("sample %d: anomaly reported during warmup period!", i)
		}
	}
}

func TestDetectorStableStream(t *testing.T) {
	cfg := DefaultConfig("inlet_temp_celsius")
	cfg.WarmupSamples = 30
	detector := NewDetector(cfg)

	// Seeded RNG for reproducible pseudo-normal random walk
	rng := rand.New(rand.NewSource(42))

	// Feed 200 normal samples (mean ~23.0C, stddev ~0.5C)
	for i := 1; i <= 200; i++ {
		noise := (rng.Float64() - 0.5) * 1.0 // [-0.5, +0.5]
		val := 23.0 + noise
		rep := detector.Update(val)

		// Once warmed up, no anomalies should fire on steady data
		if rep.SampleCount > 30 && rep.IsAnomaly {
			t.Fatalf("unexpected false positive at sample %d: val=%.2f, z=%.2f",
				rep.SampleCount, val, rep.ZScore)
		}
	}

	mean, stdDev, count := detector.CurrentStats()
	if count != 200 {
		t.Fatalf("expected 200 samples, got %d", count)
	}
	if math.Abs(mean-23.0) > 0.5 {
		t.Fatalf("expected mean ~23.0, got %.2f", mean)
	}
	t.Logf("Stable stream stats: mean=%.2f, stdDev=%.2f", mean, stdDev)
}

func TestDetectorThermalSpike(t *testing.T) {
	cfg := DefaultConfig("inlet_temp_celsius")
	cfg.WarmupSamples = 30
	cfg.ZScoreThreshold = 3.0
	detector := NewDetector(cfg)

	// Establish baseline around 22.0C with very small variance
	for i := 0; i < 40; i++ {
		rep := detector.Update(22.0)
		if rep.IsAnomaly {
			t.Fatalf("unexpected anomaly during baseline at %d", i)
		}
	}

	// Inject severe thermal spike to 45.0C
	rep := detector.Update(45.0)

	if !rep.IsAnomaly {
		t.Fatalf("expected thermal spike 45.0C to be detected as anomaly! z-score: %.2f", rep.ZScore)
	}
	if rep.ZScore <= 3.0 {
		t.Fatalf("expected z-score > 3.0, got %.2f", rep.ZScore)
	}
	if rep.ObservedValue != 45.0 {
		t.Fatalf("expected observed 45.0, got %.2f", rep.ObservedValue)
	}
	t.Logf("Detected anomaly: %s", rep.Explanation)
}

func TestDetectorFanFailureDrop(t *testing.T) {
	cfg := DefaultConfig("fan_rpm")
	cfg.WarmupSamples = 30
	cfg.ZScoreThreshold = 3.0
	detector := NewDetector(cfg)

	// Normal fan RPM around 6000
	for i := 0; i < 40; i++ {
		detector.Update(6000.0)
	}

	// Fan fails: drops to 500 RPM
	rep := detector.Update(500.0)

	if !rep.IsAnomaly {
		t.Fatalf("expected fan drop to 500 RPM to be flagged as anomaly! z=%.2f", rep.ZScore)
	}
	if rep.ZScore >= -3.0 {
		t.Fatalf("expected negative z-score < -3.0 for fan drop, got %.2f", rep.ZScore)
	}
	t.Logf("Detected fan failure anomaly: %s", rep.Explanation)
}

func TestDetectorConcurrency(t *testing.T) {
	cfg := DefaultConfig("cpu_temp_celsius")
	detector := NewDetector(cfg)
	var wg sync.WaitGroup

	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				detector.Update(40.0 + float64(id%5))
			}
		}(i)
	}

	wg.Wait()
	_, _, count := detector.CurrentStats()
	if count != 1000 {
		t.Fatalf("expected 1000 total samples, got %d", count)
	}
}

func TestDetectorConfigAndReset(t *testing.T) {
	// Test fallback configs (zero / negative values)
	cfg := DetectorConfig{
		MetricName:      "test",
		Alpha:           -1,
		ZScoreThreshold: -1,
		WarmupSamples:   -1,
		MinStdDev:       -1,
	}
	det := NewDetector(cfg)
	det.Update(10.0)
	mean, sd, samples := det.CurrentStats()
	if samples != 1 || mean != 10.0 || sd <= 0 {
		t.Fatalf("unexpected stats: %v, %v, %v", mean, sd, samples)
	}
	det.Reset()
	_, _, samples = det.CurrentStats()
	if samples != 0 {
		t.Fatalf("expected 0 samples after reset")
	}
}

func BenchmarkDetectorUpdate(b *testing.B) {
	cfg := DefaultConfig("benchmark_metric")
	detector := NewDetector(cfg)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = detector.Update(25.0 + float64(i%10)*0.1)
	}
}
