// Package engine implements rule-based alarm evaluation and lifecycle tracking.
package engine

import (
	"fmt"
	"sync"
	"time"

	"github.com/fleetpulse/fleetpulse/services/telemetry-service/internal/domain"
	"github.com/google/uuid"
)

// RuleEvaluator monitors sensor thresholds, tracking consecutive samples and deduplicating active alarms.
type RuleEvaluator struct {
	mu           sync.RWMutex
	rules        []domain.AlarmRule
	consecutive  map[string]int           // "deviceID:ruleName" -> consecutive count
	activeAlarms map[string]*domain.Alarm // "deviceID:ruleName" -> active alarm
	alarmStore   map[string]*domain.Alarm // alarmID -> alarm
}

// NewRuleEvaluator initializes evaluator with production server alarm rules.
func NewRuleEvaluator() *RuleEvaluator {
	return &RuleEvaluator{
		rules: []domain.AlarmRule{
			{
				Name:                "HighInletTemp",
				Metric:              "inlet_temp",
				Operator:            ">",
				Threshold:           35.0,
				ConsecutiveRequired: 3,
				Severity:            domain.SeverityWarning,
			},
			{
				Name:                "CriticalCPUTemp",
				Metric:              "cpu_temp",
				Operator:            ">",
				Threshold:           85.0,
				ConsecutiveRequired: 2,
				Severity:            domain.SeverityCritical,
			},
			{
				Name:                "FanFailure",
				Metric:              "fan_rpm",
				Operator:            "<",
				Threshold:           1000.0,
				ConsecutiveRequired: 1,
				Severity:            domain.SeverityCritical,
			},
			{
				Name:                "MemoryErrorStorm",
				Metric:              "memory_errors",
				Operator:            ">",
				Threshold:           20.0,
				ConsecutiveRequired: 2,
				Severity:            domain.SeverityWarning,
			},
		},
		consecutive:  make(map[string]int),
		activeAlarms: make(map[string]*domain.Alarm),
		alarmStore:   make(map[string]*domain.Alarm),
	}
}

// Evaluate checks a telemetry sample against all configured rules.
// Returns any newly raised or updated alarms.
func (e *RuleEvaluator) Evaluate(sample domain.TelemetrySample) []*domain.Alarm {
	e.mu.Lock()
	defer e.mu.Unlock()

	var newAlarms []*domain.Alarm

	for _, rule := range e.rules {
		val := extractMetric(sample, rule.Metric)
		violated := checkCondition(val, rule.Operator, rule.Threshold)
		key := fmt.Sprintf("%s:%s", sample.DeviceID, rule.Name)

		if violated {
			e.consecutive[key]++
			if e.consecutive[key] >= rule.ConsecutiveRequired {
				// Condition met for required consecutive ticks
				active, exists := e.activeAlarms[key]
				if !exists || active.Status == domain.StatusCleared {
					// Raise new alarm
					alarmID := "alm-" + uuid.NewString()[:8]
					msg := fmt.Sprintf("%s on %s: current %.2f violates threshold %.2f",
						rule.Name, sample.Serial, val, rule.Threshold)
					alarm := &domain.Alarm{
						ID:             alarmID,
						OrgID:          sample.OrgID,
						DeviceID:       sample.DeviceID,
						Serial:         sample.Serial,
						RuleName:       rule.Name,
						Severity:       rule.Severity,
						Status:         domain.StatusRaised,
						Message:        msg,
						MetricName:     rule.Metric,
						CurrentValue:   val,
						ThresholdValue: rule.Threshold,
						RaisedAt:       time.Now(),
					}
					e.activeAlarms[key] = alarm
					e.alarmStore[alarmID] = alarm
					newAlarms = append(newAlarms, alarm)
				} else {
					// Deduplicate: already active, just update current observed value
					active.CurrentValue = val
				}
			}
		} else {
			// Normal reading: reset consecutive violation counter
			e.consecutive[key] = 0
			// If an alarm was active, auto-clear it
			if active, exists := e.activeAlarms[key]; exists && active.Status != domain.StatusCleared {
				active.Status = domain.StatusCleared
				active.ClearedAt = time.Now()
				delete(e.activeAlarms, key)
			}
		}
	}

	return newAlarms
}

// Acknowledge marks an active alarm as acknowledged.
func (e *RuleEvaluator) Acknowledge(alarmID, user string) (*domain.Alarm, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	alarm, exists := e.alarmStore[alarmID]
	if !exists {
		return nil, domain.ErrAlarmNotFound
	}
	if alarm.Status == domain.StatusAcknowledged {
		return alarm, nil // Idempotent
	}

	alarm.Status = domain.StatusAcknowledged
	alarm.AcknowledgedAt = time.Now()
	alarm.AcknowledgedBy = user
	return alarm, nil
}

// ListAlarms retrieves alarms filtered by org, device, status, or severity.
func (e *RuleEvaluator) ListAlarms(orgID, deviceID, severity, status string, limit int) []*domain.Alarm {
	e.mu.RLock()
	defer e.mu.RUnlock()

	if limit <= 0 || limit > 100 {
		limit = 50
	}

	out := make([]*domain.Alarm, 0)
	for _, a := range e.alarmStore {
		if orgID != "" && a.OrgID != orgID {
			continue
		}
		if deviceID != "" && a.DeviceID != deviceID {
			continue
		}
		if severity != "" && a.Severity != severity {
			continue
		}
		if status != "" && a.Status != status {
			continue
		}
		out = append(out, a)
		if len(out) >= limit {
			break
		}
	}
	return out
}

// ActiveAlarmCount returns the number of currently active (raised or acknowledged) alarms.
func (e *RuleEvaluator) ActiveAlarmCount() int {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return len(e.activeAlarms)
}

func extractMetric(sample domain.TelemetrySample, metric string) float64 {
	switch metric {
	case "inlet_temp":
		return sample.InletTemp
	case "cpu_temp":
		return sample.CPUTemp
	case "fan_rpm":
		// Return lowest fan RPM among the 4 fans
		minFan := sample.FanRPM[0]
		for i := 1; i < 4; i++ {
			if sample.FanRPM[i] < minFan {
				minFan = sample.FanRPM[i]
			}
		}
		return minFan
	case "memory_errors":
		return float64(sample.MemoryErrors)
	default:
		return 0.0
	}
}

func checkCondition(val float64, op string, threshold float64) bool {
	switch op {
	case ">":
		return val > threshold
	case "<":
		return val < threshold
	case ">=":
		return val >= threshold
	case "<=":
		return val <= threshold
	default:
		return false
	}
}
