package engine

import (
	"fmt"
	"math/rand"
	"testing"

	"github.com/fleetpulse/fleetpulse/services/telemetry-service/internal/domain"
)

func TestAnomalyEngine(t *testing.T) {
	eng := NewAnomalyEngine(15, 3.0, 0.15)
	rng := rand.New(rand.NewSource(12345))

	sample := domain.TelemetrySample{
		DeviceID: "dev-anom-1",
		Serial:   "UCS-ANOM-001",
		OrgID:    "org-acme",
	}

	// 1. Warm-up phase: 15 samples with normal variations (48 +/- 1 C)
	for i := 0; i < 15; i++ {
		sample.CPUTemp = 48.0 + (rng.Float64()*2.0 - 1.0)
		sample.InletTemp = 22.0 + (rng.Float64()*1.0 - 0.5)
		detected := eng.Process(sample)
		if len(detected) != 0 {
			t.Fatalf("unexpected anomaly during warm-up at sample %d", i)
		}
	}

	// 2. Normal operation: 10 more baseline samples
	for i := 0; i < 10; i++ {
		sample.CPUTemp = 48.0 + (rng.Float64()*2.0 - 1.0)
		sample.InletTemp = 22.0 + (rng.Float64()*1.0 - 0.5)
		detected := eng.Process(sample)
		if len(detected) != 0 {
			t.Fatalf("unexpected anomaly during baseline at sample %d", i)
		}
	}

	// 3. Sudden massive spike: CPUTemp = 88.0 C!
	sample.CPUTemp = 88.0
	detected := eng.Process(sample)
	if len(detected) == 0 {
		t.Fatalf("expected anomaly detection on 88C spike, but got none")
	}

	anom := detected[0]
	if anom.MetricName != "cpu_temp" || anom.AnomalyScore < 3.0 {
		t.Fatalf("unexpected anomaly properties: score=%f, metric=%s", anom.AnomalyScore, anom.MetricName)
	}

	// 4. Verify TopAnomalies list
	top := eng.TopAnomalies(5)
	if len(top) != 1 || top[0].DeviceID != "dev-anom-1" {
		t.Fatalf("expected top anomalies to contain dev-anom-1, got %+v", top)
	}
}

func BenchmarkAnomalyDetector(b *testing.B) {
	eng := NewAnomalyEngine(15, 3.0, 0.15)
	sample := domain.TelemetrySample{
		DeviceID:  "dev-bench",
		Serial:    "UCS-BENCH-1",
		OrgID:     "org-1",
		CPUTemp:   50.0,
		InletTemp: 22.0,
	}

	// Warmup
	for i := 0; i < 20; i++ {
		eng.Process(sample)
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		sample.CPUTemp = 50.0 + float64(i%5)
		eng.Process(sample)
	}
}

func BenchmarkMultiDeviceAnomalyDetection(b *testing.B) {
	eng := NewAnomalyEngine(15, 3.0, 0.15)
	devices := make([]domain.TelemetrySample, 100)
	for i := 0; i < 100; i++ {
		devices[i] = domain.TelemetrySample{
			DeviceID:  fmt.Sprintf("dev-%d", i),
			Serial:    fmt.Sprintf("UCS-%d", i),
			CPUTemp:   50.0,
			InletTemp: 22.0,
		}
		for j := 0; j < 20; j++ {
			eng.Process(devices[i])
		}
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		idx := i % 100
		devices[idx].CPUTemp = 50.0 + float64(i%3)
		eng.Process(devices[idx])
	}
}
