package repo

import (
	"fmt"
	"testing"

	"github.com/fleetpulse/fleetpulse/services/telemetry-service/internal/domain"
)

func TestRingBuffer(t *testing.T) {
	rb := NewRingBuffer(5) // Max 5 samples per device

	// Push 10 samples
	for i := 1; i <= 10; i++ {
		rb.Push(domain.TelemetrySample{
			DeviceID:  "dev-1",
			CPUTemp:   float64(40 + i),
			InletTemp: 22.0,
			FanRPM:    [4]float64{4500, 4500, 4500, 4500},
		})
	}

	// Should only have 5 most recent samples (50, 49, 48, 47, 46)
	recent := rb.GetRecent("dev-1", 10)
	if len(recent) != 5 {
		t.Fatalf("expected 5 samples, got %d", len(recent))
	}
	if recent[0].CPUTemp != 50.0 {
		t.Fatalf("expected newest sample CPUTemp 50.0, got %f", recent[0].CPUTemp)
	}
	if recent[4].CPUTemp != 46.0 {
		t.Fatalf("expected 5th sample CPUTemp 46.0, got %f", recent[4].CPUTemp)
	}

	// Limit smaller than available
	recentTwo := rb.GetRecent("dev-1", 2)
	if len(recentTwo) != 2 {
		t.Fatalf("expected 2 samples, got %d", len(recentTwo))
	}

	// Non-existent device
	if rb.GetRecent("missing-dev", 5) != nil {
		t.Fatalf("expected nil for missing device")
	}

	// Test Fleet Health Calculation
	// dev-1: CPUTemp = 50 -> OK
	// dev-warning: CPUTemp = 75 -> Warning
	// dev-critical: CPUTemp = 90 -> Critical
	rb.Push(domain.TelemetrySample{DeviceID: "dev-warning", CPUTemp: 75.0, InletTemp: 22.0, FanRPM: [4]float64{4500, 4500, 4500, 4500}})
	rb.Push(domain.TelemetrySample{DeviceID: "dev-critical", CPUTemp: 90.0, InletTemp: 22.0, FanRPM: [4]float64{4500, 4500, 4500, 4500}})

	summary := rb.GetFleetHealth()
	if summary.TotalDevices != 3 {
		t.Fatalf("expected 3 total devices, got %d", summary.TotalDevices)
	}
	if summary.OKCount != 1 || summary.WarningCount != 1 || summary.CriticalCount != 1 {
		t.Fatalf("unexpected fleet health counts: %+v", summary)
	}
}

func BenchmarkRingBufferPushAndRead(b *testing.B) {
	rb := NewRingBuffer(120)
	sample := domain.TelemetrySample{
		DeviceID:  "dev-bench",
		CPUTemp:   50.0,
		InletTemp: 22.0,
		FanRPM:    [4]float64{4500, 4500, 4500, 4500},
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		sample.DeviceID = fmt.Sprintf("dev-%d", i%100)
		rb.Push(sample)
		if i%5 == 0 {
			_ = rb.GetRecent(sample.DeviceID, 10)
		}
	}
}
