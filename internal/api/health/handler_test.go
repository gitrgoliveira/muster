package health_test

import (
	"context"
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

func minCfg(beadsVersion string) health.StatusConfig {
	return health.StatusConfig{BeadsVersion: beadsVersion, SchemaVersion: 1}
}

func TestOrchestratorStatus_ReturnsFullPayload(t *testing.T) {
	handler := health.OrchestratorStatusHandler(minCfg("1.0.0"))

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
	handler := health.OrchestratorStatusHandler(minCfg(wantVersion))

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
	handler := health.OrchestratorStatusHandler(minCfg("1.0.0"))

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

// pingOK is a Pinger that always returns nil.
type pingOK struct{}

func (pingOK) Ping(_ context.Context) error { return nil }

// pingFail is a Pinger that always returns an error.
type pingFail struct{}

func (pingFail) Ping(_ context.Context) error { return context.DeadlineExceeded }

func TestOrchestratorStatus_OnlineWhenPingOK(t *testing.T) {
	cfg := health.StatusConfig{BeadsVersion: "1.0.0", SchemaVersion: 1, Pinger: pingOK{}}
	handler := health.OrchestratorStatusHandler(cfg)

	req := httptest.NewRequest(http.MethodGet, "/status", nil)
	w := httptest.NewRecorder()
	handler(w, req)

	var body health.OrchestratorStatusResponse
	json.NewDecoder(w.Result().Body).Decode(&body) //nolint:errcheck
	if !body.Online {
		t.Error("expected online=true when Ping succeeds")
	}
}

func TestOrchestratorStatus_OfflineWhenPingFails(t *testing.T) {
	cfg := health.StatusConfig{BeadsVersion: "1.0.0", SchemaVersion: 1, Pinger: pingFail{}}
	handler := health.OrchestratorStatusHandler(cfg)

	req := httptest.NewRequest(http.MethodGet, "/status", nil)
	w := httptest.NewRecorder()
	handler(w, req)

	var body health.OrchestratorStatusResponse
	json.NewDecoder(w.Result().Body).Decode(&body) //nolint:errcheck
	if body.Online {
		t.Error("expected online=false when Ping fails")
	}
}

func TestOrchestratorStatus_ConfigFieldsPopulated(t *testing.T) {
	cfg := health.StatusConfig{
		BeadsVersion:  "2.0.0",
		BeadsDir:      "/data/beads",
		DoltDatabase:  "mydb",
		DoltMode:      "remote",
		ReadSource:    "dolt",
		BdCLI:         "/usr/local/bin/bd",
		ProjectID:     "proj-123",
		SchemaVersion: 2,
	}
	handler := health.OrchestratorStatusHandler(cfg)
	req := httptest.NewRequest(http.MethodGet, "/status", nil)
	w := httptest.NewRecorder()
	handler(w, req)

	var body health.OrchestratorStatusResponse
	json.NewDecoder(w.Result().Body).Decode(&body) //nolint:errcheck

	if body.BeadsDir != "/data/beads" {
		t.Errorf("beadsDir want /data/beads got %q", body.BeadsDir)
	}
	if body.DoltDatabase != "mydb" {
		t.Errorf("doltDatabase want mydb got %q", body.DoltDatabase)
	}
	if body.ReadSource != "dolt" {
		t.Errorf("readSource want dolt got %q", body.ReadSource)
	}
	if body.SchemaVersion != 2 {
		t.Errorf("schemaVersion want 2 got %d", body.SchemaVersion)
	}
}
