package metrics

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMetricsCollectionAndServing(t *testing.T) {
	// Increment metrics
	HTTPRequestsTotal.WithLabelValues("GET", "/api/v1/devices", "200").Inc()
	HTTPRequestDurationSeconds.WithLabelValues("GET", "/api/v1/devices").Observe(0.042)
	DevicesTotal.WithLabelValues("org-1", "UCS-C220-M7", "ok").Set(150)
	AlarmsActive.WithLabelValues("org-1", "critical").Set(3)

	handler := Handler()
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK from /metrics, got %d", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "fleetpulse_http_requests_total") {
		t.Errorf("missing fleetpulse_http_requests_total in metrics output")
	}
	if !strings.Contains(body, "fleetpulse_fleet_devices_total") {
		t.Errorf("missing fleetpulse_fleet_devices_total in metrics output")
	}
}
