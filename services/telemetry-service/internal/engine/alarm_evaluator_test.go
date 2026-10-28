package engine

import (
	"testing"

	"github.com/fleetpulse/fleetpulse/services/telemetry-service/internal/domain"
)

func TestRuleEvaluator_AlarmLifecycle(t *testing.T) {
	eval := NewRuleEvaluator()

	// Base sample
	sample := domain.TelemetrySample{
		DeviceID:     "dev-1",
		Serial:       "UCS-001",
		OrgID:        "org-acme",
		InletTemp:    36.5, // Violates HighInletTemp (threshold 35.0, requires 3 consecutive)
		CPUTemp:      50.0,
		FanRPM:       [4]float64{4500, 4500, 4500, 4500},
		MemoryErrors: 0,
	}

	// Sample 1: consecutive = 1 -> no alarm
	alarms := eval.Evaluate(sample)
	if len(alarms) != 0 {
		t.Fatalf("expected 0 alarms on 1st violation, got %d", len(alarms))
	}

	// Sample 2: consecutive = 2 -> no alarm
	alarms = eval.Evaluate(sample)
	if len(alarms) != 0 {
		t.Fatalf("expected 0 alarms on 2nd violation, got %d", len(alarms))
	}

	// Sample 3: consecutive = 3 -> ALARM RAISED!
	alarms = eval.Evaluate(sample)
	if len(alarms) != 1 {
		t.Fatalf("expected 1 alarm raised on 3rd violation, got %d", len(alarms))
	}
	alm := alarms[0]
	if alm.RuleName != "HighInletTemp" || alm.Status != domain.StatusRaised || alm.Severity != domain.SeverityWarning {
		t.Fatalf("unexpected alarm properties: %+v", alm)
	}

	// Sample 4: consecutive = 4 -> DEDUPLICATION (same active alarm, not duplicated)
	alarms = eval.Evaluate(sample)
	if len(alarms) != 0 {
		t.Fatalf("expected 0 new alarms on 4th consecutive violation due to deduplication, got %d", len(alarms))
	}
	if eval.ActiveAlarmCount() != 1 {
		t.Fatalf("expected active alarm count 1, got %d", eval.ActiveAlarmCount())
	}

	// Acknowledge alarm
	acked, err := eval.Acknowledge(alm.ID, "usr-operator")
	if err != nil || acked.Status != domain.StatusAcknowledged || acked.AcknowledgedBy != "usr-operator" {
		t.Fatalf("failed to acknowledge alarm: %v, %+v", err, acked)
	}

	// Sample 5: metric returns to normal (22.0) -> Auto-clears alarm!
	sample.InletTemp = 22.0
	eval.Evaluate(sample)
	if eval.ActiveAlarmCount() != 0 {
		t.Fatalf("expected active alarm to be auto-cleared when normal, got active count %d", eval.ActiveAlarmCount())
	}

	// List alarms with filters
	list := eval.ListAlarms("org-acme", "dev-1", domain.SeverityWarning, domain.StatusCleared, 10)
	if len(list) != 1 || list[0].Status != domain.StatusCleared {
		t.Fatalf("expected 1 cleared alarm in list, got %+v", list)
	}
}

func TestRuleEvaluator_FanAndCPURules(t *testing.T) {
	eval := NewRuleEvaluator()

	// 1. Fan Failure rule (requires only 1 sample < 1000 RPM)
	fanFailSample := domain.TelemetrySample{
		DeviceID:  "dev-2",
		Serial:    "UCS-002",
		OrgID:     "org-acme",
		FanRPM:    [4]float64{0, 4500, 4500, 4500},
		InletTemp: 22.0,
		CPUTemp:   50.0,
	}
	alarms := eval.Evaluate(fanFailSample)
	if len(alarms) != 1 || alarms[0].RuleName != "FanFailure" || alarms[0].Severity != domain.SeverityCritical {
		t.Fatalf("expected critical FanFailure alarm, got %+v", alarms)
	}

	// 2. Non-existent alarm acknowledgment
	_, err := eval.Acknowledge("non-existent-id", "op")
	if err != domain.ErrAlarmNotFound {
		t.Fatalf("expected ErrAlarmNotFound, got %v", err)
	}
}
