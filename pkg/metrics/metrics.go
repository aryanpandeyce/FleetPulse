// Package metrics defines standardized Prometheus collectors for RED metrics
// (Rate, Errors, Duration) and business-level domain indicators.
package metrics

import (
	"net/http"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Standard Prometheus collectors
var (
	// HTTP RED Metrics
	HTTPRequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "fleetpulse",
			Subsystem: "http",
			Name:      "requests_total",
			Help:      "Total number of HTTP requests processed partitioned by method, path, and status code.",
		},
		[]string{"method", "path", "status"},
	)

	HTTPRequestDurationSeconds = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "fleetpulse",
			Subsystem: "http",
			Name:      "request_duration_seconds",
			Help:      "Latency distribution of HTTP requests in seconds.",
			Buckets:   []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0, 2.5, 5.0},
		},
		[]string{"method", "path"},
	)

	// gRPC RED Metrics
	GRPCRequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "fleetpulse",
			Subsystem: "grpc",
			Name:      "requests_total",
			Help:      "Total number of gRPC unary calls partitioned by service, method, and code.",
		},
		[]string{"service", "method", "code"},
	)

	GRPCRequestDurationSeconds = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "fleetpulse",
			Subsystem: "grpc",
			Name:      "request_duration_seconds",
			Help:      "Latency distribution of gRPC unary calls in seconds.",
			Buckets:   []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0},
		},
		[]string{"service", "method"},
	)

	// Fleet Business Metrics
	DevicesTotal = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "fleetpulse",
			Subsystem: "fleet",
			Name:      "devices_total",
			Help:      "Total number of servers registered in inventory by model and health status.",
		},
		[]string{"org_id", "model", "health_status"},
	)

	AlarmsActive = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "fleetpulse",
			Subsystem: "alarms",
			Name:      "active_total",
			Help:      "Current count of active alarms grouped by organization and severity.",
		},
		[]string{"org_id", "severity"},
	)

	JobsByState = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "fleetpulse",
			Subsystem: "workflow",
			Name:      "jobs_total",
			Help:      "Total workflow jobs tracked by type and execution state.",
		},
		[]string{"org_id", "type", "state"},
	)

	IngestLagSeconds = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "fleetpulse",
			Subsystem: "telemetry",
			Name:      "ingest_lag_seconds",
			Help:      "Consumer group lag in seconds relative to publisher timestamp.",
		},
		[]string{"consumer_group"},
	)
)

var registerOnce sync.Once

// RegisterAll registers all collectors with the default Prometheus registry idempotently.
func RegisterAll() {
	registerOnce.Do(func() {
		prometheus.MustRegister(
			HTTPRequestsTotal,
			HTTPRequestDurationSeconds,
			GRPCRequestsTotal,
			GRPCRequestDurationSeconds,
			DevicesTotal,
			AlarmsActive,
			JobsByState,
			IngestLagSeconds,
		)
	})
}

func init() {
	RegisterAll()
}

// Handler returns standard HTTP handler for serving /metrics.
func Handler() http.Handler {
	return promhttp.Handler()
}
