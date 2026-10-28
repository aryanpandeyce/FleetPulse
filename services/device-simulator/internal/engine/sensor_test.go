package engine

import (
	"testing"

	"github.com/fleetpulse/fleetpulse/services/device-simulator/internal/model"
)

func TestSensorEngine_NormalAndFaults(t *testing.T) {
	eng := NewSensorEngine(42)

	d := &model.SimulatedDevice{
		ID:              "dev-001",
		Serial:          "UCS-TEST-001",
		OrgID:           "org-acme",
		Model:           model.ModelUCSC220M7,
		FirmwareVersion: "4.2(3a)",
	}
	eng.InitializeDevice(d)

	if d.InletTemp < 18.0 || d.InletTemp > 28.0 {
		t.Fatalf("unexpected initial inlet temp: %f", d.InletTemp)
	}
	if d.CPUTemp < 40.0 || d.CPUTemp > 56.0 {
		t.Fatalf("unexpected initial cpu temp: %f", d.CPUTemp)
	}
	if d.Health != "OK" {
		t.Fatalf("expected initial health OK, got %s", d.Health)
	}

	// 1. Normal Tick
	eng.Tick(d)
	if d.Health != "OK" {
		t.Fatalf("expected health OK after 1 tick, got %s", d.Health)
	}

	// 2. Thermal Spike Fault
	d.SetFault(model.FaultThermalSpike)
	for i := 0; i < 20; i++ {
		eng.Tick(d)
	}
	if d.CPUTemp < 80.0 {
		t.Fatalf("expected high CPU temp during thermal spike, got %f", d.CPUTemp)
	}
	if d.Health != "Critical" {
		t.Fatalf("expected Critical health under thermal spike, got %s", d.Health)
	}

	// 3. Clear fault & settle
	d.SetFault(model.FaultNone)
	for i := 0; i < 100; i++ {
		eng.Tick(d)
	}
	if d.Health != "OK" {
		t.Fatalf("expected health to recover to OK, got %s (cpu=%f)", d.Health, d.CPUTemp)
	}

	// 4. Fan Failure Fault
	d.SetFault(model.FaultFanFailure)
	eng.Tick(d)
	if d.FanRPM[0] != 0 {
		t.Fatalf("expected Fan 0 to have 0 RPM, got %f", d.FanRPM[0])
	}
	if d.Health != "Critical" {
		t.Fatalf("expected Critical health under fan failure, got %s", d.Health)
	}

	// 5. Memory Error Storm Fault
	d.SetFault(model.FaultMemoryErrorStorm)
	initialErrors := d.MemoryErrors
	eng.Tick(d)
	if d.MemoryErrors <= initialErrors {
		t.Fatalf("expected memory errors to increase, got %d vs %d", d.MemoryErrors, initialErrors)
	}

	// 6. Power Off settling
	d.PowerState = "Off"
	eng.Tick(d)
	if d.PowerWatts[0] != 8.0 || d.FanRPM[0] != 0 {
		t.Fatalf("expected low power and 0 rpm when Off, got watts=%f, fan=%f", d.PowerWatts[0], d.FanRPM[0])
	}
}

func TestDeviceRegistry(t *testing.T) {
	reg := NewDeviceRegistry()

	d1 := &model.SimulatedDevice{ID: "1", Serial: "UCS-1", Model: model.ModelUCSC220M7}
	d2 := &model.SimulatedDevice{ID: "2", Serial: "UCS-2", Model: model.ModelUCSC240M7}

	reg.Register(d1)
	reg.Register(d2)

	if reg.Count() != 2 {
		t.Fatalf("expected count 2, got %d", reg.Count())
	}

	found, err := reg.GetBySerial("UCS-1")
	if err != nil || found.Serial != "UCS-1" {
		t.Fatalf("failed to retrieve device: %v", err)
	}

	// Non-existent device
	_, err = reg.GetBySerial("UCS-MISSING")
	if err == nil {
		t.Fatalf("expected error for missing serial")
	}

	// Fault injection & clearing
	if err := reg.InjectFault("UCS-1", model.FaultThermalSpike); err != nil {
		t.Fatalf("failed to inject fault: %v", err)
	}
	if d1.GetFault() != model.FaultThermalSpike {
		t.Fatalf("fault not updated on device")
	}

	if err := reg.ClearFault("UCS-1"); err != nil {
		t.Fatalf("failed to clear fault: %v", err)
	}
	if d1.GetFault() != model.FaultNone {
		t.Fatalf("fault not cleared")
	}

	// Invalid fault
	if err := reg.InjectFault("UCS-1", "invalid_bogus_fault"); err == nil {
		t.Fatalf("expected error for invalid fault")
	}

	// All devices order preserved
	all := reg.All()
	if len(all) != 2 || all[0].Serial != "UCS-1" || all[1].Serial != "UCS-2" {
		t.Fatalf("unexpected All devices order: %+v", all)
	}
}
