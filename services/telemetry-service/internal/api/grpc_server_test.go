package api

import (
	"context"
	"testing"

	"github.com/fleetpulse/fleetpulse/api/proto/telemetryv1"
	"github.com/fleetpulse/fleetpulse/pkg/logging"
	"github.com/fleetpulse/fleetpulse/services/telemetry-service/internal/domain"
	"github.com/fleetpulse/fleetpulse/services/telemetry-service/internal/engine"
	"github.com/fleetpulse/fleetpulse/services/telemetry-service/internal/repo"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func setupTestServer() (*Server, *repo.RingBuffer, *engine.RuleEvaluator, *engine.AnomalyEngine) {
	ring := repo.NewRingBuffer(10)
	evaluator := engine.NewRuleEvaluator()
	anomalies := engine.NewAnomalyEngine(15, 3.0, 0.15)
	server := NewServer(ring, evaluator, anomalies)
	return server, ring, evaluator, anomalies
}

func TestServer_GetDeviceTelemetry(t *testing.T) {
	server, ring, _, _ := setupTestServer()
	ctx := context.Background()

	// 1. Missing Device ID -> InvalidArgument
	_, err := server.GetDeviceTelemetry(ctx, &telemetryv1.GetDeviceTelemetryRequest{DeviceId: ""})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("expected InvalidArgument, got %v", err)
	}

	// 2. Populate sample
	ring.Push(domain.TelemetrySample{
		DeviceID:      "dev-1",
		Serial:        "UCS-1",
		Model:         "UCS-C220-M7",
		InletTemp:     22.5,
		CPUTemp:       50.0,
		FanRPM:        [4]float64{4500, 4500, 4500, 4500},
		PowerWatts:    [2]float64{160, 160},
		MemoryErrors:  0,
		TimestampUnix: 1700000000,
	})

	resp, err := server.GetDeviceTelemetry(ctx, &telemetryv1.GetDeviceTelemetryRequest{DeviceId: "dev-1", Limit: 5})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Samples) != 1 || resp.Samples[0].InletTempCelsius != 22.5 {
		t.Fatalf("unexpected samples returned: %+v", resp.Samples)
	}
}

func TestServer_AlarmsAndTenantIsolation(t *testing.T) {
	server, _, evaluator, _ := setupTestServer()

	// Seed alarms under two different tenants
	evaluator.Evaluate(domain.TelemetrySample{
		DeviceID:  "dev-acme",
		Serial:    "UCS-ACME-1",
		OrgID:     "org-acme",
		FanRPM:    [4]float64{0, 4500, 4500, 4500}, // Fan failure -> critical alarm
		InletTemp: 22.0,
		CPUTemp:   50.0,
	})
	evaluator.Evaluate(domain.TelemetrySample{
		DeviceID:  "dev-globex",
		Serial:    "UCS-GLOBEX-1",
		OrgID:     "org-globex",
		FanRPM:    [4]float64{0, 4500, 4500, 4500},
		InletTemp: 22.0,
		CPUTemp:   50.0,
	})

	// 1. Query as Acme user (tenant isolation)
	acmeCtx := logging.WithOrgID(context.Background(), "org-acme")
	acmeAlarms, err := server.ListAlarms(acmeCtx, &telemetryv1.ListAlarmsRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(acmeAlarms.Alarms) != 1 || acmeAlarms.Alarms[0].OrgId != "org-acme" {
		t.Fatalf("TENANT LEAK: expected only Acme alarms, got %+v", acmeAlarms.Alarms)
	}

	alarmID := acmeAlarms.Alarms[0].Id

	// 2. Acknowledge alarm
	ackCtx := logging.WithUserID(acmeCtx, "usr-operator-1")
	ackResp, err := server.AcknowledgeAlarm(ackCtx, &telemetryv1.AcknowledgeAlarmRequest{AlarmId: alarmID})
	if err != nil || !ackResp.Success || ackResp.Alarm.Status != "acknowledged" {
		t.Fatalf("failed to acknowledge alarm: %v, %+v", err, ackResp)
	}

	// 3. Acknowledge missing alarm -> NotFound
	_, err = server.AcknowledgeAlarm(ackCtx, &telemetryv1.AcknowledgeAlarmRequest{AlarmId: "missing-id"})
	if status.Code(err) != codes.NotFound {
		t.Fatalf("expected NotFound, got %v", err)
	}

	// 4. Missing alarm_id -> InvalidArgument
	_, err = server.AcknowledgeAlarm(ackCtx, &telemetryv1.AcknowledgeAlarmRequest{AlarmId: ""})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("expected InvalidArgument, got %v", err)
	}
}

func TestServer_GetFleetSummary(t *testing.T) {
	server, ring, _, anomalies := setupTestServer()
	ctx := context.Background()

	// Push devices
	ring.Push(domain.TelemetrySample{DeviceID: "dev-ok", CPUTemp: 50.0, InletTemp: 22.0, FanRPM: [4]float64{4500, 4500, 4500, 4500}})
	ring.Push(domain.TelemetrySample{DeviceID: "dev-crit", CPUTemp: 92.0, InletTemp: 22.0, FanRPM: [4]float64{4500, 4500, 4500, 4500}})

	// Trigger anomaly
	for i := 0; i < 20; i++ {
		anomalies.Process(domain.TelemetrySample{DeviceID: "dev-anom", Serial: "UCS-A", CPUTemp: 48.0, InletTemp: 22.0})
	}
	anomalies.Process(domain.TelemetrySample{DeviceID: "dev-anom", Serial: "UCS-A", CPUTemp: 89.0, InletTemp: 22.0})

	resp, err := server.GetFleetSummary(ctx, &telemetryv1.GetFleetSummaryRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Health.TotalDevices != 2 || resp.Health.OkCount != 1 || resp.Health.CriticalCount != 1 {
		t.Fatalf("unexpected health counts: %+v", resp.Health)
	}
	if len(resp.TopAnomalies) != 1 || resp.TopAnomalies[0].DeviceId != "dev-anom" {
		t.Fatalf("expected anomaly in top anomalies: %+v", resp.TopAnomalies)
	}
}
