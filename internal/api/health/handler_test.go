package health_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gitrgoliveira/muster/internal/api/health"
	"github.com/gitrgoliveira/muster/internal/store"
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
	seed := store.SeedDolt()
	handler := health.OrchestratorStatusHandler("1.0.0", seed)

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
	// Dolt sub-fields should be non-zero (from seed).
	if body.Dolt.Branch == "" {
		t.Error("expected non-empty dolt.branch")
	}
}

func TestOrchestratorStatus_BeadsVersionMatchesSeed(t *testing.T) {
	const wantVersion = "0.9.1"
	seed := store.SeedDolt()
	handler := health.OrchestratorStatusHandler(wantVersion, seed)

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

func TestOrchestratorStatus_DoltMatchesSeed(t *testing.T) {
	seed := store.SeedDolt()
	handler := health.OrchestratorStatusHandler("0.1.0", seed)

	req := httptest.NewRequest(http.MethodGet, "/status", nil)
	w := httptest.NewRecorder()
	handler(w, req)

	var body health.OrchestratorStatusResponse
	if err := json.NewDecoder(w.Result().Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode body: %v", err)
	}

	got := body.Dolt
	if got.Branch != seed.Branch {
		t.Errorf("dolt.branch: want %q, got %q", seed.Branch, got.Branch)
	}
	if got.Status != seed.Status {
		t.Errorf("dolt.status: want %q, got %q", seed.Status, got.Status)
	}
	if got.Port != seed.Port {
		t.Errorf("dolt.port: want %d, got %d", seed.Port, got.Port)
	}
	if got.Remote != seed.Remote {
		t.Errorf("dolt.remote: want %q, got %q", seed.Remote, got.Remote)
	}
	if got.Ahead != seed.Ahead {
		t.Errorf("dolt.ahead: want %d, got %d", seed.Ahead, got.Ahead)
	}
	if got.Behind != seed.Behind {
		t.Errorf("dolt.behind: want %d, got %d", seed.Behind, got.Behind)
	}
	if got.LastSync != seed.LastSync {
		t.Errorf("dolt.lastSync: want %q, got %q", seed.LastSync, got.LastSync)
	}
	if got.Server != seed.Server {
		t.Errorf("dolt.server: want %q, got %q", seed.Server, got.Server)
	}
	if got.Writers != seed.Writers {
		t.Errorf("dolt.writers: want %d, got %d", seed.Writers, got.Writers)
	}
}

func TestOrchestratorStatus_ServerTimeIsRFC3339(t *testing.T) {
	seed := store.SeedDolt()
	handler := health.OrchestratorStatusHandler("1.0.0", seed)

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
