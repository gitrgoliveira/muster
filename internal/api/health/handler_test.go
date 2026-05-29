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
		ReadSource:    "dolt-sql",
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
	if body.ReadSource != "dolt-sql" {
		t.Errorf("readSource want dolt-sql got %q", body.ReadSource)
	}
	if body.SchemaVersion != 2 {
		t.Errorf("schemaVersion want 2 got %d", body.SchemaVersion)
	}
}

// ── M2 status DTO additions ───────────────────────────────────────────

type fakeRunCounter struct{ count int }

func (f *fakeRunCounter) RunCount() int { return f.count }

func TestOrchestratorStatus_M2_TmuxFields(t *testing.T) {
	cfg := health.StatusConfig{
		BeadsVersion:  "1.0.0",
		SchemaVersion: 1,
		TmuxAvailable: true,
		TmuxVersion:   "3.6b",
		RunCounter:    &fakeRunCounter{count: 2},
		Adapters: []health.AdapterInfo{
			{ID: "claude", Version: "2.1.145", LoggedIn: true},
		},
	}
	handler := health.OrchestratorStatusHandler(cfg)
	req := httptest.NewRequest(http.MethodGet, "/status", nil)
	w := httptest.NewRecorder()
	handler(w, req)

	var body health.OrchestratorStatusResponse
	json.NewDecoder(w.Result().Body).Decode(&body) //nolint:errcheck

	if !body.TmuxAvailable {
		t.Error("tmuxAvailable want true")
	}
	if body.TmuxVersion != "3.6b" {
		t.Errorf("tmuxVersion want 3.6b got %q", body.TmuxVersion)
	}
	if body.RunningCount != 2 {
		t.Errorf("runningCount want 2 got %d", body.RunningCount)
	}
	if len(body.Adapters) != 1 {
		t.Fatalf("adapters want 1 got %d", len(body.Adapters))
	}
	if body.Adapters[0].ID != "claude" {
		t.Errorf("adapter ID want claude got %q", body.Adapters[0].ID)
	}
	if !body.Adapters[0].LoggedIn {
		t.Error("adapter loggedIn want true")
	}
}

func TestOrchestratorStatus_M2_TmuxUnavailable(t *testing.T) {
	cfg := health.StatusConfig{
		BeadsVersion:  "1.0.0",
		SchemaVersion: 1,
		TmuxAvailable: false,
	}
	handler := health.OrchestratorStatusHandler(cfg)
	req := httptest.NewRequest(http.MethodGet, "/status", nil)
	w := httptest.NewRecorder()
	handler(w, req)

	var body health.OrchestratorStatusResponse
	json.NewDecoder(w.Result().Body).Decode(&body) //nolint:errcheck
	if body.TmuxAvailable {
		t.Error("tmuxAvailable want false when tmux not installed")
	}
}

func TestOrchestratorStatus_M2_RunningCountNilCounter(t *testing.T) {
	cfg := health.StatusConfig{
		BeadsVersion:  "1.0.0",
		SchemaVersion: 1,
		// no RunCounter
	}
	handler := health.OrchestratorStatusHandler(cfg)
	req := httptest.NewRequest(http.MethodGet, "/status", nil)
	w := httptest.NewRecorder()
	handler(w, req)

	var body health.OrchestratorStatusResponse
	json.NewDecoder(w.Result().Body).Decode(&body) //nolint:errcheck
	if body.RunningCount != 0 {
		t.Errorf("runningCount want 0 got %d", body.RunningCount)
	}
}
