package model

import (
	"testing"
)

func TestSimulatedDeviceMethods(t *testing.T) {
	d := &SimulatedDevice{
		ID:              "dev-1",
		Serial:          "UCS-001",
		OrgID:           "org-1",
		Model:           ModelUCSC220M7,
		FirmwareVersion: "4.2(3a)",
		Health:          "OK",
		InletTemp:       22.5,
		CPUTemp:         50.0,
		FanRPM:          [4]float64{4500, 4500, 4500, 4500},
		PowerWatts:      [2]float64{150, 150},
		MemoryErrors:    2,
	}

	// 1. Snapshot
	snap := d.Snapshot()
	if snap.DeviceID != "dev-1" || snap.Serial != "UCS-001" || snap.InletTemp != 22.5 {
		t.Fatalf("unexpected snapshot: %+v", snap)
	}

	// 2. Fault methods
	d.SetFault(FaultThermalSpike)
	if d.GetFault() != FaultThermalSpike {
		t.Fatalf("expected FaultThermalSpike, got %s", d.GetFault())
	}

	// 3. FirmwareTask methods
	task := &FirmwareTask{
		State:           "Downloading",
		ProgressPercent: 25,
		TargetVersion:   "4.3(1a)",
	}
	d.SetFirmwareTask(task)
	retrieved := d.GetFirmwareTask()
	if retrieved == nil || retrieved.State != "Downloading" || retrieved.ProgressPercent != 25 {
		t.Fatalf("unexpected firmware task: %+v", retrieved)
	}
}
