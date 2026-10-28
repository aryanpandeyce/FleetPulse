// Package api implements the gRPC server for TelemetryService.
package api

import (
	"context"

	"github.com/fleetpulse/fleetpulse/api/proto/telemetryv1"
	"github.com/fleetpulse/fleetpulse/pkg/logging"
	"github.com/fleetpulse/fleetpulse/services/telemetry-service/internal/engine"
	"github.com/fleetpulse/fleetpulse/services/telemetry-service/internal/repo"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Server implements telemetryv1.TelemetryServiceServer.
type Server struct {
	telemetryv1.UnimplementedTelemetryServiceServer

	ring      *repo.RingBuffer
	evaluator *engine.RuleEvaluator
	anomalies *engine.AnomalyEngine
}

// NewServer creates a new TelemetryServiceServer.
func NewServer(ring *repo.RingBuffer, evaluator *engine.RuleEvaluator, anomalies *engine.AnomalyEngine) *Server {
	return &Server{
		ring:      ring,
		evaluator: evaluator,
		anomalies: anomalies,
	}
}

// GetDeviceTelemetry retrieves recent telemetry observations for a device.
func (s *Server) GetDeviceTelemetry(ctx context.Context, in *telemetryv1.GetDeviceTelemetryRequest) (*telemetryv1.GetDeviceTelemetryResponse, error) {
	if in.DeviceId == "" {
		return nil, status.Error(codes.InvalidArgument, "device_id is required")
	}

	samples := s.ring.GetRecent(in.DeviceId, int(in.Limit))
	protoSamples := make([]*telemetryv1.TelemetrySample, len(samples))
	for i, sm := range samples {
		fanRPM := make([]int32, len(sm.FanRPM))
		for j, f := range sm.FanRPM {
			fanRPM[j] = int32(f)
		}
		powerWatts := make([]float64, len(sm.PowerWatts))
		copy(powerWatts, sm.PowerWatts[:])

		protoSamples[i] = &telemetryv1.TelemetrySample{
			DeviceId:                 sm.DeviceID,
			Serial:                   sm.Serial,
			Model:                    sm.Model,
			InletTempCelsius:         sm.InletTemp,
			CpuTempCelsius:           sm.CPUTemp,
			FanRpm:                   fanRPM,
			PowerWatts:               powerWatts,
			MemoryErrorsCorrectable:  sm.MemoryErrors,
			TimestampUnix:            sm.TimestampUnix,
		}
	}

	return &telemetryv1.GetDeviceTelemetryResponse{
		Samples: protoSamples,
	}, nil
}

// ListAlarms retrieves alarms matching query criteria under tenant isolation.
func (s *Server) ListAlarms(ctx context.Context, in *telemetryv1.ListAlarmsRequest) (*telemetryv1.ListAlarmsResponse, error) {
	orgID := logging.OrgIDFromContext(ctx)

	alarms := s.evaluator.ListAlarms(orgID, in.DeviceId, in.Severity, in.Status, int(in.Limit))
	protoAlarms := make([]*telemetryv1.Alarm, len(alarms))
	for i, a := range alarms {
		protoAlarms[i] = &telemetryv1.Alarm{
			Id:                  a.ID,
			OrgId:               a.OrgID,
			DeviceId:            a.DeviceID,
			RuleName:            a.RuleName,
			Severity:            a.Severity,
			Status:              a.Status,
			Message:             a.Message,
			MetricName:          a.MetricName,
			CurrentValue:        a.CurrentValue,
			ThresholdValue:      a.ThresholdValue,
			RaisedAtUnix:        a.RaisedAt.Unix(),
			AcknowledgedAtUnix:  a.AcknowledgedAt.Unix(),
			AcknowledgedBy:      a.AcknowledgedBy,
		}
	}

	return &telemetryv1.ListAlarmsResponse{
		Alarms: protoAlarms,
	}, nil
}

// AcknowledgeAlarm records operator acknowledgment of an active alarm.
func (s *Server) AcknowledgeAlarm(ctx context.Context, in *telemetryv1.AcknowledgeAlarmRequest) (*telemetryv1.AcknowledgeAlarmResponse, error) {
	if in.AlarmId == "" {
		return nil, status.Error(codes.InvalidArgument, "alarm_id is required")
	}

	userID := logging.UserIDFromContext(ctx)
	if userID == "" {
		userID = "operator"
	}

	alarm, err := s.evaluator.Acknowledge(in.AlarmId, userID)
	if err != nil {
		return nil, status.Error(codes.NotFound, "alarm not found")
	}

	return &telemetryv1.AcknowledgeAlarmResponse{
		Success: true,
		Alarm: &telemetryv1.Alarm{
			Id:                 alarm.ID,
			OrgId:              alarm.OrgID,
			DeviceId:           alarm.DeviceID,
			RuleName:           alarm.RuleName,
			Severity:           alarm.Severity,
			Status:             alarm.Status,
			Message:            alarm.Message,
			MetricName:         alarm.MetricName,
			CurrentValue:       alarm.CurrentValue,
			ThresholdValue:     alarm.ThresholdValue,
			RaisedAtUnix:       alarm.RaisedAt.Unix(),
			AcknowledgedAtUnix: alarm.AcknowledgedAt.Unix(),
			AcknowledgedBy:     alarm.AcknowledgedBy,
		},
	}, nil
}

// GetFleetSummary returns counts by health and top anomalous devices.
func (s *Server) GetFleetSummary(ctx context.Context, in *telemetryv1.GetFleetSummaryRequest) (*telemetryv1.GetFleetSummaryResponse, error) {
	health := s.ring.GetFleetHealth()
	anomalies := s.anomalies.TopAnomalies(5)

	protoAnomalies := make([]*telemetryv1.AnomalousDevice, len(anomalies))
	for i, a := range anomalies {
		protoAnomalies[i] = &telemetryv1.AnomalousDevice{
			DeviceId:     a.DeviceID,
			Serial:       a.Serial,
			MetricName:   a.MetricName,
			AnomalyScore: a.AnomalyScore,
			Explanation:  a.Explanation,
		}
	}

	return &telemetryv1.GetFleetSummaryResponse{
		Health: &telemetryv1.FleetHealthCount{
			OkCount:       health.OKCount,
			WarningCount:  health.WarningCount,
			CriticalCount: health.CriticalCount,
			TotalDevices:  health.TotalDevices,
		},
		TopAnomalies: protoAnomalies,
	}, nil
}
