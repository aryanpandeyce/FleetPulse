// Package domain provides core telemetry, alarm entities, and evaluation rules.
package domain

import (
	"errors"
	"time"
)

var (
	ErrAlarmNotFound     = errors.New("alarm not found")
	ErrAlarmAlreadyAcked = errors.New("alarm already acknowledged")
)

// Severity levels
const (
	SeverityInfo     = "info"
	SeverityWarning  = "warning"
	SeverityCritical = "critical"
)

// Alarm statuses
const (
	StatusRaised       = "raised"
	StatusAcknowledged = "acknowledged"
	StatusCleared      = "cleared"
)

// TelemetrySample represents a single sensor observation from a server.
type TelemetrySample struct {
	DeviceID      string     `json:"device_id"`
	Serial        string     `json:"serial"`
	Model         string     `json:"model"`
	OrgID         string     `json:"org_id"`
	InletTemp     float64    `json:"inlet_temp_celsius"`
	CPUTemp       float64    `json:"cpu_temp_celsius"`
	FanRPM        [4]float64 `json:"fan_rpm"`
	PowerWatts    [2]float64 `json:"power_watts"`
	MemoryErrors  int64      `json:"memory_errors"`
	TimestampUnix int64      `json:"timestamp_unix"`
}

// Alarm represents an active or resolved threshold or anomaly alert.
type Alarm struct {
	ID                 string    `json:"id"`
	OrgID              string    `json:"org_id"`
	DeviceID           string    `json:"device_id"`
	Serial             string    `json:"serial"`
	RuleName           string    `json:"rule_name"`
	Severity           string    `json:"severity"`
	Status             string    `json:"status"`
	Message            string    `json:"message"`
	MetricName         string    `json:"metric_name"`
	CurrentValue       float64   `json:"current_value"`
	ThresholdValue     float64   `json:"threshold_value"`
	AnomalyScore       float64   `json:"anomaly_score,omitempty"`
	RaisedAt           time.Time `json:"raised_at"`
	AcknowledgedAt     time.Time `json:"acknowledged_at,omitempty"`
	AcknowledgedBy     string    `json:"acknowledged_by,omitempty"`
	ClearedAt          time.Time `json:"cleared_at,omitempty"`
}

// AlarmRule specifies condition under which an alarm is raised.
type AlarmRule struct {
	Name                string  `json:"name"`
	Metric              string  `json:"metric"` // "inlet_temp", "cpu_temp", "fan_rpm", "memory_errors"
	Operator            string  `json:"operator"` // ">", "<", ">="
	Threshold           float64 `json:"threshold"`
	ConsecutiveRequired int     `json:"consecutive_required"`
	Severity            string  `json:"severity"`
}

// AnomalousDevice summary for fleet analytics.
type AnomalousDevice struct {
	DeviceID     string  `json:"device_id"`
	Serial       string  `json:"serial"`
	MetricName   string  `json:"metric_name"`
	AnomalyScore float64 `json:"anomaly_score"`
	Explanation  string  `json:"explanation"`
}

// FleetSummary aggregates overall fleet health counts and anomalies.
type FleetSummary struct {
	OKCount       int64             `json:"ok_count"`
	WarningCount  int64             `json:"warning_count"`
	CriticalCount int64             `json:"critical_count"`
	TotalDevices  int64             `json:"total_devices"`
	TopAnomalies  []AnomalousDevice `json:"top_anomalies"`
}
