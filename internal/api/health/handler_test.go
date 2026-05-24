package health_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gitrgoliveira/muster/internal/api/health"
)

func TestHealthz_Returns200_AndOK(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	w := httptest.NewRecorder()

	health.HealthzHandler(w, req)

	res := w.Result()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", res.StatusCode)
	}

	var body health.HealthzResponse
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode body: %v", err)
	}
	if !body.OK {
		t.Errorf("expected ok=true, got ok=false")
	}
}

func TestOrchestratorStatus_ReturnsFullPayload(t *testing.T) {
	handler := health.OrchestratorStatusHandler("1.0.0")

	req := httptest.NewRequest(http.MethodGet, "/status", nil)
	w := httptest.NewRecorder()
	handler(w, req)

	res := w.Result()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", res.StatusCode)
	}

	var body health.OrchestratorStatusResponse
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode body: %v", err)
	}

	if body.Build == "" {
		t.Error("expected non-empty build")
	}
	if body.SchemaVersion <= 0 {
		t.Errorf("expected schemaVersion > 0, got %d", body.SchemaVersion)
	}
	if body.BeadsVersion == "" {
		t.Error("expected non-empty beadsVersion")
	}
	if !body.Online {
		t.Error("expected online=true")
	}
	if body.ServerTime == "" {
		t.Error("expected non-empty serverTime")
	}
}

func TestOrchestratorStatus_BeadsVersionMatchesSeed(t *testing.T) {
	const wantVersion = "0.9.1"
	handler := health.OrchestratorStatusHandler(wantVersion)

	req := httptest.NewRequest(http.MethodGet, "/status", nil)
	w := httptest.NewRecorder()
	handler(w, req)

	var body health.OrchestratorStatusResponse
	if err := json.NewDecoder(w.Result().Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode body: %v", err)
	}

	if body.BeadsVersion != wantVersion {
		t.Errorf("expected beadsVersion=%q, got %q", wantVersion, body.BeadsVersion)
	}
}

func TestOrchestratorStatus_ServerTimeIsRFC3339(t *testing.T) {
	handler := health.OrchestratorStatusHandler("1.0.0")

	req := httptest.NewRequest(http.MethodGet, "/status", nil)
	w := httptest.NewRecorder()
	handler(w, req)

	var body health.OrchestratorStatusResponse
	if err := json.NewDecoder(w.Result().Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode body: %v", err)
	}

	if _, err := time.Parse(time.RFC3339, body.ServerTime); err != nil {
		t.Errorf("serverTime %q is not valid RFC3339: %v", body.ServerTime, err)
	}
}
