// Package engine implements sensor simulation, random walk dynamics, and fault injection.
package engine

import (
	"math/rand"
	"sync"
	"time"

	"github.com/fleetpulse/fleetpulse/services/device-simulator/internal/model"
)

// SensorEngine calculates sensor readings using a seeded pseudo-random walk.
type SensorEngine struct {
	mu  sync.Mutex
	rng *rand.Rand
}

// NewSensorEngine initializes the engine with a reproducible seed.
func NewSensorEngine(seed int64) *SensorEngine {
	if seed == 0 {
		seed = time.Now().UnixNano()
	}
	return &SensorEngine{
		rng: rand.New(rand.NewSource(seed)),
	}
}

// InitializeDevice assigns initial realistic baseline values to a device.
func (e *SensorEngine) InitializeDevice(d *model.SimulatedDevice) {
	e.mu.Lock()
	defer e.mu.Unlock()

	d.PowerState = "On"
	d.Health = "OK"
	d.InletTemp = 22.0 + (e.rng.Float64()*4.0 - 2.0) // 20.0 - 24.0 C
	d.CPUTemp = 48.0 + (e.rng.Float64()*8.0 - 4.0)   // 44.0 - 52.0 C
	for i := 0; i < 4; i++ {
		d.FanRPM[i] = 4500.0 + (e.rng.Float64()*400.0 - 200.0) // 4300 - 4700 RPM
	}
	d.PowerWatts[0] = 160.0 + (e.rng.Float64()*20.0 - 10.0)
	d.PowerWatts[1] = 158.0 + (e.rng.Float64()*20.0 - 10.0)
	d.MemoryErrors = 0
	d.ActiveFault = model.FaultNone
	d.LastSampled = time.Now()
}

// Tick steps the random walk forward one interval and applies any active hardware faults.
func (e *SensorEngine) Tick(d *model.SimulatedDevice) {
	e.mu.Lock()
	defer e.mu.Unlock()

	fault := d.GetFault()

	if d.PowerState == "Off" {
		d.InletTemp = 21.0
		d.CPUTemp = 22.0
		for i := 0; i < 4; i++ {
			d.FanRPM[i] = 0
		}
		d.PowerWatts[0] = 8.0
		d.PowerWatts[1] = 8.0
		d.Health = "OK"
		d.LastSampled = time.Now()
		return
	}

	// 1. Process Active Faults
	switch fault {
	case model.FaultThermalSpike:
		d.InletTemp = clamp(d.InletTemp+0.8, 20.0, 42.0)
		d.CPUTemp = clamp(d.CPUTemp+2.5, 45.0, 96.0)
		for i := 0; i < 4; i++ {
			d.FanRPM[i] = clamp(d.FanRPM[i]+300.0, 4000.0, 8000.0) // Fans ramp up in response
		}
	case model.FaultFanFailure:
		d.FanRPM[0] = 0 // Fan 1 dies
		for i := 1; i < 4; i++ {
			d.FanRPM[i] = clamp(d.FanRPM[i]+200.0, 4000.0, 7500.0) // Others compensate
		}
		d.CPUTemp = clamp(d.CPUTemp+1.2, 45.0, 88.0)
	case model.FaultMemoryErrorStorm:
		d.MemoryErrors += int64(10 + e.rng.Intn(15))
	case model.FaultOffline:
		d.Health = "Critical"
		d.LastSampled = time.Now()
		return
	default:
		// Normal random walk
		// Inlet Temp: mean-reverting towards 22.0
		inletDelta := (e.rng.Float64() - 0.5) * 0.4
		inletReversion := (22.0 - d.InletTemp) * 0.1
		d.InletTemp = clamp(d.InletTemp+inletDelta+inletReversion, 19.0, 26.0)

		// CPU Temp: tracks inlet + small fluctuations
		cpuTarget := 48.0 + (d.InletTemp-22.0)*1.8
		cpuDelta := (e.rng.Float64() - 0.5) * 1.0
		cpuReversion := (cpuTarget - d.CPUTemp) * 0.15
		d.CPUTemp = clamp(d.CPUTemp+cpuDelta+cpuReversion, 40.0, 68.0)

		// Fan RPM: tracks CPU temp
		for i := 0; i < 4; i++ {
			fanTarget := 4500.0 + (d.CPUTemp-48.0)*80.0
			fanDelta := (e.rng.Float64() - 0.5) * 80.0
			d.FanRPM[i] = clamp(d.FanRPM[i]+fanDelta+(fanTarget-d.FanRPM[i])*0.2, 3800.0, 6000.0)
		}

		// Power Watts: shared load
		loadDelta := (e.rng.Float64() - 0.5) * 8.0
		d.PowerWatts[0] = clamp(160.0+loadDelta, 130.0, 240.0)
		d.PowerWatts[1] = clamp(158.0+loadDelta, 130.0, 240.0)

		// Occasional benign memory correctable error
		if e.rng.Float64() < 0.02 {
			d.MemoryErrors++
		}
	}

	// 2. Compute Health State
	if d.CPUTemp >= 85.0 || d.FanRPM[0] == 0 || d.MemoryErrors >= 50 {
		d.Health = "Critical"
	} else if d.CPUTemp >= 72.0 || d.InletTemp >= 30.0 || d.MemoryErrors >= 10 {
		d.Health = "Warning"
	} else {
		d.Health = "OK"
	}

	d.LastSampled = time.Now()
}

func clamp(val, min, max float64) float64 {
	if val < min {
		return min
	}
	if val > max {
		return max
	}
	return val
}
