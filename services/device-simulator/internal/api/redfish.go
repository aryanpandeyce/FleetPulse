// Package api implements Redfish REST endpoints and simulator fault injection APIs.
package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/fleetpulse/fleetpulse/pkg/httpx"
	"github.com/fleetpulse/fleetpulse/services/device-simulator/internal/engine"
	"github.com/fleetpulse/fleetpulse/services/device-simulator/internal/model"
	"github.com/go-chi/chi/v5"
)

// Handler sets up Redfish and simulation control routes.
type Handler struct {
	registry    *engine.DeviceRegistry
	failureRate float64 // 0.0 to 1.0 probability of simulated firmware upgrade failure
}

// NewHandler constructs a new Redfish HTTP API handler.
func NewHandler(registry *engine.DeviceRegistry, failureRate float64) http.Handler {
	h := &Handler{
		registry:    registry,
		failureRate: failureRate,
	}

	r := chi.NewRouter()
	r.Use(httpx.RequestID)
	r.Use(httpx.CORS)
	r.Use(httpx.Recovery)

	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		httpx.JSON(w, http.StatusOK, map[string]string{"status": "ok", "service": "device-simulator"})
	})

	// Redfish v1 Endpoints
	r.Route("/redfish/v1/Systems/{serial}", func(sr chi.Router) {
		sr.Get("/", h.getSystem)
		sr.Get("/Thermal", h.getThermal)
		sr.Get("/Power", h.getPower)
		sr.Get("/Firmware", h.getFirmware)
		sr.Get("/FirmwareTask", h.getFirmwareTask)
		sr.Post("/Actions/FirmwareUpdate", h.startFirmwareUpdate)
	})

	// Simulator Management & Fault Injection
	r.Get("/sim/devices", h.listDevices)
	r.Post("/sim/faults", h.injectFault)
	r.Delete("/sim/faults/{serial}", h.clearFault)

	return r
}

func (h *Handler) getSystem(w http.ResponseWriter, r *http.Request) {
	serial := chi.URLParam(r, "serial")
	d, err := h.registry.GetBySerial(serial)
	if err != nil {
		httpx.Error(w, r, http.StatusNotFound, "NOT_FOUND", err.Error())
		return
	}

	snap := d.Snapshot()
	resp := map[string]interface{}{
		"@odata.id":    fmt.Sprintf("/redfish/v1/Systems/%s", serial),
		"@odata.type":  "#ComputerSystem.v1_16_0.ComputerSystem",
		"Id":           d.ID,
		"Name":         d.Model,
		"SystemType":   "Physical",
		"Manufacturer": "Cisco Systems Inc",
		"Model":        d.Model,
		"SerialNumber": d.Serial,
		"PowerState":   d.PowerState,
		"BiosVersion":  d.FirmwareVersion,
		"Status": map[string]string{
			"State":  "Enabled",
			"Health": snap.Health,
		},
	}
	httpx.JSON(w, http.StatusOK, resp)
}

func (h *Handler) getThermal(w http.ResponseWriter, r *http.Request) {
	serial := chi.URLParam(r, "serial")
	d, err := h.registry.GetBySerial(serial)
	if err != nil {
		httpx.Error(w, r, http.StatusNotFound, "NOT_FOUND", err.Error())
		return
	}

	snap := d.Snapshot()
	resp := map[string]interface{}{
		"@odata.id":   fmt.Sprintf("/redfish/v1/Systems/%s/Thermal", serial),
		"@odata.type": "#Thermal.v1_7_0.Thermal",
		"Temperatures": []map[string]interface{}{
			{
				"Name":                   "Inlet Temp",
				"ReadingCelsius":         snap.InletTemp,
				"UpperThresholdCritical": 40.0,
			},
			{
				"Name":                   "CPU Temp",
				"ReadingCelsius":         snap.CPUTemp,
				"UpperThresholdCritical": 85.0,
			},
		},
		"Fans": []map[string]interface{}{
			{"Name": "Fan 1", "Reading": snap.FanRPM[0], "ReadingUnits": "RPM"},
			{"Name": "Fan 2", "Reading": snap.FanRPM[1], "ReadingUnits": "RPM"},
			{"Name": "Fan 3", "Reading": snap.FanRPM[2], "ReadingUnits": "RPM"},
			{"Name": "Fan 4", "Reading": snap.FanRPM[3], "ReadingUnits": "RPM"},
		},
	}
	httpx.JSON(w, http.StatusOK, resp)
}

func (h *Handler) getPower(w http.ResponseWriter, r *http.Request) {
	serial := chi.URLParam(r, "serial")
	d, err := h.registry.GetBySerial(serial)
	if err != nil {
		httpx.Error(w, r, http.StatusNotFound, "NOT_FOUND", err.Error())
		return
	}

	snap := d.Snapshot()
	resp := map[string]interface{}{
		"@odata.id":   fmt.Sprintf("/redfish/v1/Systems/%s/Power", serial),
		"@odata.type": "#Power.v1_7_0.Power",
		"PowerSupplies": []map[string]interface{}{
			{"Name": "PSU 1", "LastPowerOutputWatts": snap.PowerWatts[0], "LineInputVoltage": 220.0},
			{"Name": "PSU 2", "LastPowerOutputWatts": snap.PowerWatts[1], "LineInputVoltage": 220.0},
		},
	}
	httpx.JSON(w, http.StatusOK, resp)
}

func (h *Handler) getFirmware(w http.ResponseWriter, r *http.Request) {
	serial := chi.URLParam(r, "serial")
	d, err := h.registry.GetBySerial(serial)
	if err != nil {
		httpx.Error(w, r, http.StatusNotFound, "NOT_FOUND", err.Error())
		return
	}

	resp := map[string]interface{}{
		"@odata.id":       fmt.Sprintf("/redfish/v1/Systems/%s/Firmware", serial),
		"CurrentVersion":  d.FirmwareVersion,
		"Updateable":      true,
		"InstalledBundle": "Cisco-UCSC-M7-Bundle",
	}
	httpx.JSON(w, http.StatusOK, resp)
}

func (h *Handler) getFirmwareTask(w http.ResponseWriter, r *http.Request) {
	serial := chi.URLParam(r, "serial")
	d, err := h.registry.GetBySerial(serial)
	if err != nil {
		httpx.Error(w, r, http.StatusNotFound, "NOT_FOUND", err.Error())
		return
	}

	task := d.GetFirmwareTask()
	if task == nil {
		httpx.JSON(w, http.StatusOK, map[string]string{"state": "None", "progress_percent": "0"})
		return
	}
	httpx.JSON(w, http.StatusOK, task)
}

type FirmwareUpdateReq struct {
	ImageURL string `json:"image_url"`
	Version  string `json:"version"`
}

func (h *Handler) startFirmwareUpdate(w http.ResponseWriter, r *http.Request) {
	serial := chi.URLParam(r, "serial")
	d, err := h.registry.GetBySerial(serial)
	if err != nil {
		httpx.Error(w, r, http.StatusNotFound, "NOT_FOUND", err.Error())
		return
	}

	var req FirmwareUpdateReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Version == "" {
		httpx.Error(w, r, http.StatusBadRequest, "INVALID_REQUEST", "image_url and version are required")
		return
	}

	task := &model.FirmwareTask{
		State:           "Downloading",
		ProgressPercent: 10,
		TargetVersion:   req.Version,
	}
	d.SetFirmwareTask(task)

	// Launch async progression simulator
	go func() {
		stages := []struct {
			state    string
			progress int
			delay    time.Duration
		}{
			{"Downloading", 30, 200 * time.Millisecond},
			{"Installing", 60, 300 * time.Millisecond},
			{"Rebooting", 85, 250 * time.Millisecond},
			{"Completed", 100, 150 * time.Millisecond},
		}

		for _, stage := range stages {
			time.Sleep(stage.delay)
			d.SetFirmwareTask(&model.FirmwareTask{
				State:           stage.state,
				ProgressPercent: stage.progress,
				TargetVersion:   req.Version,
			})
		}
		// Upon completion, update device version
		d.FirmwareVersion = req.Version
	}()

	httpx.JSON(w, http.StatusAccepted, map[string]interface{}{
		"status":      "TaskStarted",
		"task_uri":    fmt.Sprintf("/redfish/v1/Systems/%s/FirmwareTask", serial),
		"target_ver":  req.Version,
	})
}

func (h *Handler) listDevices(w http.ResponseWriter, r *http.Request) {
	devices := h.registry.All()
	type DeviceSummary struct {
		Serial   string `json:"serial"`
		Model    string `json:"model"`
		Health   string `json:"health"`
		Fault    string `json:"active_fault"`
		Firmware string `json:"firmware_version"`
	}
	out := make([]DeviceSummary, 0, len(devices))
	for _, d := range devices {
		snap := d.Snapshot()
		out = append(out, DeviceSummary{
			Serial:   d.Serial,
			Model:    d.Model,
			Health:   snap.Health,
			Fault:    d.GetFault(),
			Firmware: d.FirmwareVersion,
		})
	}
	httpx.JSON(w, http.StatusOK, map[string]interface{}{
		"total":   len(out),
		"devices": out,
	})
}

type FaultRequest struct {
	Serial string `json:"serial"`
	Fault  string `json:"fault"`
}

func (h *Handler) injectFault(w http.ResponseWriter, r *http.Request) {
	var req FaultRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.Error(w, r, http.StatusBadRequest, "INVALID_BODY", "malformed json")
		return
	}

	if err := h.registry.InjectFault(req.Serial, req.Fault); err != nil {
		httpx.Error(w, r, http.StatusBadRequest, "FAULT_INJECTION_FAILED", err.Error())
		return
	}

	httpx.JSON(w, http.StatusOK, map[string]string{
		"status": "fault_injected",
		"serial": req.Serial,
		"fault":  req.Fault,
	})
}

func (h *Handler) clearFault(w http.ResponseWriter, r *http.Request) {
	serial := chi.URLParam(r, "serial")
	if err := h.registry.ClearFault(serial); err != nil {
		httpx.Error(w, r, http.StatusNotFound, "CLEAR_FAILED", err.Error())
		return
	}

	httpx.JSON(w, http.StatusOK, map[string]string{
		"status": "fault_cleared",
		"serial": serial,
	})
}
