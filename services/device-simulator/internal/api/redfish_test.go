package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/fleetpulse/fleetpulse/services/device-simulator/internal/engine"
	"github.com/fleetpulse/fleetpulse/services/device-simulator/internal/model"
)

func setupTestSimulator() (http.Handler, *engine.DeviceRegistry) {
	reg := engine.NewDeviceRegistry()
	eng := engine.NewSensorEngine(100)

	d := &model.SimulatedDevice{
		ID:              "dev-sim-1",
		Serial:          "UCS-C220-M7-001",
		OrgID:           "org-acme",
		Model:           model.ModelUCSC220M7,
		FirmwareVersion: "4.2(3a)",
	}
	eng.InitializeDevice(d)
	reg.Register(d)

	handler := NewHandler(reg, 0.0)
	return handler, reg
}

func TestRedfishEndpoints(t *testing.T) {
	handler, _ := setupTestSimulator()

	// 1. Healthz
	reqHealth := httptest.NewRequest("GET", "/healthz", nil)
	recHealth := httptest.NewRecorder()
	handler.ServeHTTP(recHealth, reqHealth)
	if recHealth.Code != http.StatusOK {
		t.Fatalf("expected 200 for /healthz, got %d", recHealth.Code)
	}

	// 2. Systems Resource
	reqSys := httptest.NewRequest("GET", "/redfish/v1/Systems/UCS-C220-M7-001", nil)
	recSys := httptest.NewRecorder()
	handler.ServeHTTP(recSys, reqSys)
	if recSys.Code != http.StatusOK {
		t.Fatalf("expected 200 for System, got %d", recSys.Code)
	}
	var sysData map[string]interface{}
	_ = json.Unmarshal(recSys.Body.Bytes(), &sysData)
	if sysData["SerialNumber"] != "UCS-C220-M7-001" || sysData["Manufacturer"] != "Cisco Systems Inc" {
		t.Fatalf("unexpected system data: %+v", sysData)
	}

	// 3. Thermal Resource
	reqTherm := httptest.NewRequest("GET", "/redfish/v1/Systems/UCS-C220-M7-001/Thermal", nil)
	recTherm := httptest.NewRecorder()
	handler.ServeHTTP(recTherm, reqTherm)
	if recTherm.Code != http.StatusOK {
		t.Fatalf("expected 200 for Thermal, got %d", recTherm.Code)
	}

	// 4. Power Resource
	reqPower := httptest.NewRequest("GET", "/redfish/v1/Systems/UCS-C220-M7-001/Power", nil)
	recPower := httptest.NewRecorder()
	handler.ServeHTTP(recPower, reqPower)
	if recPower.Code != http.StatusOK {
		t.Fatalf("expected 200 for Power, got %d", recPower.Code)
	}

	// 5. Firmware Resource
	reqFw := httptest.NewRequest("GET", "/redfish/v1/Systems/UCS-C220-M7-001/Firmware", nil)
	recFw := httptest.NewRecorder()
	handler.ServeHTTP(recFw, reqFw)
	if recFw.Code != http.StatusOK {
		t.Fatalf("expected 200 for Firmware, got %d", recFw.Code)
	}

	// 6. FirmwareTask (initial)
	reqTask := httptest.NewRequest("GET", "/redfish/v1/Systems/UCS-C220-M7-001/FirmwareTask", nil)
	recTask := httptest.NewRecorder()
	handler.ServeHTTP(recTask, reqTask)
	if recTask.Code != http.StatusOK {
		t.Fatalf("expected 200 for FirmwareTask, got %d", recTask.Code)
	}

	// 7. Start Firmware Update
	updatePayload := `{"image_url":"https://s3.local/firmware.bin","version":"4.3(1a)"}`
	reqUpd := httptest.NewRequest("POST", "/redfish/v1/Systems/UCS-C220-M7-001/Actions/FirmwareUpdate", bytes.NewBufferString(updatePayload))
	recUpd := httptest.NewRecorder()
	handler.ServeHTTP(recUpd, reqUpd)
	if recUpd.Code != http.StatusAccepted {
		t.Fatalf("expected 202 Accepted for firmware update, got %d", recUpd.Code)
	}

	// Wait briefly for background progression
	time.Sleep(100 * time.Millisecond)
	reqTaskPoll := httptest.NewRequest("GET", "/redfish/v1/Systems/UCS-C220-M7-001/FirmwareTask", nil)
	recTaskPoll := httptest.NewRecorder()
	handler.ServeHTTP(recTaskPoll, reqTaskPoll)
	var taskData map[string]interface{}
	_ = json.Unmarshal(recTaskPoll.Body.Bytes(), &taskData)
	if taskData["state"] == "None" {
		t.Fatalf("expected task progression, got: %+v", taskData)
	}

	// 8. Missing System 404
	req404 := httptest.NewRequest("GET", "/redfish/v1/Systems/UCS-MISSING", nil)
	rec404 := httptest.NewRecorder()
	handler.ServeHTTP(rec404, req404)
	if rec404.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for missing system, got %d", rec404.Code)
	}
}

func TestSimulatorFaultEndpoints(t *testing.T) {
	handler, _ := setupTestSimulator()

	// 1. List devices
	reqList := httptest.NewRequest("GET", "/sim/devices", nil)
	recList := httptest.NewRecorder()
	handler.ServeHTTP(recList, reqList)
	if recList.Code != http.StatusOK {
		t.Fatalf("expected 200 for /sim/devices, got %d", recList.Code)
	}

	// 2. Inject Fault
	body := `{"serial":"UCS-C220-M7-001","fault":"thermal_spike"}`
	reqInject := httptest.NewRequest("POST", "/sim/faults", bytes.NewBufferString(body))
	recInject := httptest.NewRecorder()
	handler.ServeHTTP(recInject, reqInject)
	if recInject.Code != http.StatusOK {
		t.Fatalf("expected 200 for fault injection, got %d", recInject.Code)
	}

	// 3. Clear Fault
	reqClear := httptest.NewRequest("DELETE", "/sim/faults/UCS-C220-M7-001", nil)
	recClear := httptest.NewRecorder()
	handler.ServeHTTP(recClear, reqClear)
	if recClear.Code != http.StatusOK {
		t.Fatalf("expected 200 for fault clear, got %d", recClear.Code)
	}

	// 4. Bad Fault
	badBody := `{"serial":"UCS-C220-M7-001","fault":"nonsense_fault"}`
	reqBad := httptest.NewRequest("POST", "/sim/faults", bytes.NewBufferString(badBody))
	recBad := httptest.NewRecorder()
	handler.ServeHTTP(recBad, reqBad)
	if recBad.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for bad fault, got %d", recBad.Code)
	}
}
