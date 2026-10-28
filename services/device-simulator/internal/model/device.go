// Package model defines virtual server state, telemetry schemas, and Redfish resources.
package model

import (
	"sync"
	"time"
)

// Supported server models
const (
	ModelUCSC220M7 = "UCS-C220-M7"
	ModelUCSC240M7 = "UCS-C240-M7"
	ModelUCSX210c  = "UCS-X210c"
)

// Fault types
const (
	FaultNone             = "none"
	FaultThermalSpike     = "thermal_spike"
	FaultFanFailure       = "fan_failure"
	FaultMemoryErrorStorm = "memory_error_storm"
	FaultOffline          = "offline"
)

// Firmware upgrade status
type FirmwareTask struct {
	State           string `json:"state"` // Downloading, Installing, Rebooting, Completed, Failed
	ProgressPercent int    `json:"progress_percent"`
	TargetVersion   string `json:"target_version"`
	ErrorMessage    string `json:"error_message,omitempty"`
}

// SimulatedDevice represents an in-memory virtual Cisco UCS server.
type SimulatedDevice struct {
	mu sync.RWMutex

	ID              string        `json:"id"`
	Serial          string        `json:"serial"`
	OrgID           string        `json:"org_id"`
	Model           string        `json:"model"`
	FirmwareVersion string        `json:"firmware_version"`
	PowerState      string        `json:"power_state"` // On, Off
	Health          string        `json:"health"`      // OK, Warning, Critical
	InletTemp       float64       `json:"inlet_temp_celsius"`
	CPUTemp         float64       `json:"cpu_temp_celsius"`
	FanRPM          [4]float64    `json:"fan_rpm"`
	PowerWatts      [2]float64    `json:"power_watts"`
	MemoryErrors    int64         `json:"memory_errors"`
	ActiveFault     string        `json:"active_fault"`
	FirmwareTask    *FirmwareTask `json:"firmware_task,omitempty"`
	LastSampled     time.Time     `json:"last_sampled"`
}

// TelemetryPayload is the JSON message streamed to NATS `telemetry.<org_id>.<serial>`.
type TelemetryPayload struct {
	DeviceID      string     `json:"device_id"`
	Serial        string     `json:"serial"`
	OrgID         string     `json:"org_id"`
	Model         string     `json:"model"`
	TimestampUnix int64      `json:"timestamp_unix"`
	Health        string     `json:"health"`
	InletTemp     float64    `json:"inlet_temp_celsius"`
	CPUTemp       float64    `json:"cpu_temp_celsius"`
	FanRPM        [4]float64 `json:"fan_rpm"`
	PowerWatts    [2]float64 `json:"power_watts"`
	MemoryErrors  int64      `json:"memory_errors"`
}

// Snapshot returns a point-in-time copy of device telemetry.
func (d *SimulatedDevice) Snapshot() TelemetryPayload {
	d.mu.RLock()
	defer d.mu.RUnlock()

	return TelemetryPayload{
		DeviceID:      d.ID,
		Serial:        d.Serial,
		OrgID:         d.OrgID,
		Model:         d.Model,
		TimestampUnix: time.Now().Unix(),
		Health:        d.Health,
		InletTemp:     d.InletTemp,
		CPUTemp:       d.CPUTemp,
		FanRPM:        d.FanRPM,
		PowerWatts:    d.PowerWatts,
		MemoryErrors:  d.MemoryErrors,
	}
}

// SetFault configures or clears a simulated hardware fault.
func (d *SimulatedDevice) SetFault(fault string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.ActiveFault = fault
}

// GetFault returns the active fault.
func (d *SimulatedDevice) GetFault() string {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.ActiveFault
}

// SetFirmwareTask updates the active firmware update task.
func (d *SimulatedDevice) SetFirmwareTask(task *FirmwareTask) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.FirmwareTask = task
}

// GetFirmwareTask returns the current firmware task.
func (d *SimulatedDevice) GetFirmwareTask() *FirmwareTask {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.FirmwareTask
}
