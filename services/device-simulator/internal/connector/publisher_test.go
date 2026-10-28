package connector

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/fleetpulse/fleetpulse/services/device-simulator/internal/engine"
	"github.com/fleetpulse/fleetpulse/services/device-simulator/internal/model"
)

type mockPublisher struct {
	mu       sync.Mutex
	messages []string
}

func (m *mockPublisher) Publish(subject string, data []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.messages = append(m.messages, subject)
	return nil
}

func (m *mockPublisher) Count() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.messages)
}

func TestConnectorConsistentHashSharding(t *testing.T) {
	reg := engine.NewDeviceRegistry()
	eng := engine.NewSensorEngine(42)

	// Create 100 devices
	for i := 0; i < 100; i++ {
		d := &model.SimulatedDevice{
			ID:              fmt.Sprintf("dev-%d", i),
			Serial:          fmt.Sprintf("UCS-C220-M7-%04d", i),
			OrgID:           "org-acme",
			Model:           model.ModelUCSC220M7,
			FirmwareVersion: "4.2(3a)",
		}
		eng.InitializeDevice(d)
		reg.Register(d)
	}

	pub := &mockPublisher{}
	numWorkers := 4
	connector := NewConnector(Config{
		NumWorkers: numWorkers,
		Interval:   50 * time.Millisecond,
		JitterPct:  0.0,
		Publisher:  pub,
		Registry:   reg,
		Engine:     eng,
	})

	if connector.WorkerCount() != 4 {
		t.Fatalf("expected 4 workers, got %d", connector.WorkerCount())
	}

	// Verify consistent hash distribution across all 4 workers
	totalSharded := 0
	for i := 0; i < numWorkers; i++ {
		workerID := fmt.Sprintf("worker-%d", i)
		count := connector.ShardedDeviceCount(workerID)
		if count == 0 {
			t.Fatalf("worker %s received 0 devices, distribution failure", workerID)
		}
		totalSharded += count
	}

	if totalSharded != 100 {
		t.Fatalf("expected 100 total sharded devices, got %d", totalSharded)
	}

	// Start connector and let it publish
	ctx, cancel := context.WithCancel(context.Background())
	connector.Start(ctx)

	time.Sleep(150 * time.Millisecond)
	cancel()
	connector.Stop()

	if pub.Count() == 0 {
		t.Fatalf("expected telemetry messages to be published, got 0")
	}
}

func TestConnectorOfflineDeviceOmission(t *testing.T) {
	reg := engine.NewDeviceRegistry()
	eng := engine.NewSensorEngine(42)

	d := &model.SimulatedDevice{
		ID:              "dev-offline",
		Serial:          "UCS-OFFLINE-001",
		OrgID:           "org-acme",
		Model:           model.ModelUCSC220M7,
		FirmwareVersion: "4.2(3a)",
	}
	eng.InitializeDevice(d)
	d.SetFault(model.FaultOffline)
	reg.Register(d)

	pub := &mockPublisher{}
	connector := NewConnector(Config{
		NumWorkers: 1,
		Interval:   30 * time.Millisecond,
		JitterPct:  0.0,
		Publisher:  pub,
		Registry:   reg,
		Engine:     eng,
	})

	ctx, cancel := context.WithCancel(context.Background())
	connector.Start(ctx)
	time.Sleep(80 * time.Millisecond)
	cancel()
	connector.Stop()

	if pub.Count() != 0 {
		t.Fatalf("expected 0 messages for offline device, got %d", pub.Count())
	}
}
