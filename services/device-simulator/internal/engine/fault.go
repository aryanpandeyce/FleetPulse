// Package engine provides fault injection registry for simulator.
package engine

import (
	"errors"
	"fmt"
	"sync"

	"github.com/fleetpulse/fleetpulse/services/device-simulator/internal/model"
)

var (
	ErrDeviceNotFound = errors.New("simulated device not found")
	ErrInvalidFault   = errors.New("invalid fault type")
)

// DeviceRegistry holds in-memory references to simulated virtual servers.
type DeviceRegistry struct {
	mu      sync.RWMutex
	devices map[string]*model.SimulatedDevice // serial -> device
	order   []string
}

// NewDeviceRegistry creates a thread-safe registry.
func NewDeviceRegistry() *DeviceRegistry {
	return &DeviceRegistry{
		devices: make(map[string]*model.SimulatedDevice),
		order:   make([]string, 0),
	}
}

// Register registers a new simulated server into the registry.
func (r *DeviceRegistry) Register(d *model.SimulatedDevice) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.devices[d.Serial] = d
	r.order = append(r.order, d.Serial)
}

// GetBySerial retrieves a device by serial number.
func (r *DeviceRegistry) GetBySerial(serial string) (*model.SimulatedDevice, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	d, exists := r.devices[serial]
	if !exists {
		return nil, fmt.Errorf("%w: %s", ErrDeviceNotFound, serial)
	}
	return d, nil
}

// All returns all registered devices.
func (r *DeviceRegistry) All() []*model.SimulatedDevice {
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make([]*model.SimulatedDevice, len(r.order))
	for i, serial := range r.order {
		out[i] = r.devices[serial]
	}
	return out
}

// Count returns the total number of registered devices.
func (r *DeviceRegistry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.devices)
}

// InjectFault applies a hardware fault to a specific device.
func (r *DeviceRegistry) InjectFault(serial, fault string) error {
	switch fault {
	case model.FaultNone, model.FaultThermalSpike, model.FaultFanFailure, model.FaultMemoryErrorStorm, model.FaultOffline:
	default:
		return fmt.Errorf("%w: %s", ErrInvalidFault, fault)
	}

	d, err := r.GetBySerial(serial)
	if err != nil {
		return err
	}

	d.SetFault(fault)
	return nil
}

// ClearFault resets fault on device to 'none'.
func (r *DeviceRegistry) ClearFault(serial string) error {
	return r.InjectFault(serial, model.FaultNone)
}
