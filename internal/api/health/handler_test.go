package health_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
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
			{ID: "claude", Installed: true, Version: "2.1.145", LoggedIn: true},
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
	if !body.Adapters[0].Installed {
		t.Error("adapter installed want true")
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

// ── T027: M3 status DTO additions ─────────────────────────────────────────

// TestOrchestratorStatus_M3_VCSFields verifies vcs.{defaultVCS,git,jj} and
// worktreeCount are present in the response (FR-012).
func TestOrchestratorStatus_M3_VCSFields(t *testing.T) {
	cfg := health.StatusConfig{
		BeadsVersion:  "1.0.0",
		SchemaVersion: 1,
		VCS: health.VCSStatus{
			DefaultVCS: "git",
			Git:        health.VCSAvailability{Available: true, Version: "git 2.40.0"},
			JJ:         health.VCSAvailability{Available: false},
		},
		WorktreeCount: 3,
	}
	handler := health.OrchestratorStatusHandler(cfg)
	req := httptest.NewRequest(http.MethodGet, "/status", nil)
	w := httptest.NewRecorder()
	handler(w, req)

	var body health.OrchestratorStatusResponse
	if err := json.NewDecoder(w.Result().Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if body.VCS.DefaultVCS != "git" {
		t.Errorf("vcs.defaultVCS want git got %q", body.VCS.DefaultVCS)
	}
	if !body.VCS.Git.Available {
		t.Error("vcs.git.available want true")
	}
	if body.VCS.JJ.Available {
		t.Error("vcs.jj.available want false")
	}
	if body.WorktreeCount != 3 {
		t.Errorf("worktreeCount want 3 got %d", body.WorktreeCount)
	}
}

// TestOrchestratorStatus_M3_JJAvailable verifies jj availability surfaced correctly.
func TestOrchestratorStatus_M3_JJAvailable(t *testing.T) {
	cfg := health.StatusConfig{
		BeadsVersion:  "1.0.0",
		SchemaVersion: 1,
		VCS: health.VCSStatus{
			DefaultVCS: "jj",
			Git:        health.VCSAvailability{Available: true},
			JJ:         health.VCSAvailability{Available: true, Version: "jj 0.42.0"},
		},
	}
	handler := health.OrchestratorStatusHandler(cfg)
	req := httptest.NewRequest(http.MethodGet, "/status", nil)
	w := httptest.NewRecorder()
	handler(w, req)

	var body health.OrchestratorStatusResponse
	json.NewDecoder(w.Result().Body).Decode(&body) //nolint:errcheck

	if body.VCS.DefaultVCS != "jj" {
		t.Errorf("vcs.defaultVCS want jj got %q", body.VCS.DefaultVCS)
	}
	if !body.VCS.JJ.Available {
		t.Error("vcs.jj.available want true")
	}
	if body.VCS.JJ.Version != "jj 0.42.0" {
		t.Errorf("vcs.jj.version want 'jj 0.42.0' got %q", body.VCS.JJ.Version)
	}
}

// TestOrchestratorStatus_M3_M2FieldsIntact verifies all M0/M1/M2 fields are
// still present (SC-007 additive-surface assertion for the DTO).
func TestOrchestratorStatus_M3_M2FieldsIntact(t *testing.T) {
	cfg := health.StatusConfig{
		BeadsVersion:  "2.0.0",
		BeadsDir:      "/data/beads",
		DoltDatabase:  "testdb",
		DoltMode:      "local",
		ReadSource:    "jsonl",
		BdCLI:         "/bin/bd",
		ProjectID:     "proj-x",
		SchemaVersion: 2,
		TmuxAvailable: true,
		TmuxVersion:   "3.6",
		RunCounter:    &fakeRunCounter{count: 1},
		Adapters: []health.AdapterInfo{
			{ID: "claude", Installed: true, Version: "2.2", LoggedIn: true},
		},
		// M3 additions.
		VCS: health.VCSStatus{
			DefaultVCS: "git",
			Git:        health.VCSAvailability{Available: true},
		},
		WorktreeCount: 2,
	}
	handler := health.OrchestratorStatusHandler(cfg)
	req := httptest.NewRequest(http.MethodGet, "/status", nil)
	w := httptest.NewRecorder()
	handler(w, req)

	// Decode into a raw map so we check all keys.
	var raw map[string]json.RawMessage
	if err := json.NewDecoder(w.Result().Body).Decode(&raw); err != nil {
		t.Fatalf("decode: %v", err)
	}

	// M0/M1 fields.
	mustHaveKey(t, raw, "build")
	mustHaveKey(t, raw, "schemaVersion")
	mustHaveKey(t, raw, "beadsVersion")
	mustHaveKey(t, raw, "online")
	mustHaveKey(t, raw, "serverTime")
	mustHaveKey(t, raw, "beadsDir")
	mustHaveKey(t, raw, "doltDatabase")
	mustHaveKey(t, raw, "doltMode")
	mustHaveKey(t, raw, "readSource")
	mustHaveKey(t, raw, "bdCLI")
	mustHaveKey(t, raw, "projectID")
	// M2 fields.
	mustHaveKey(t, raw, "tmuxAvailable")
	mustHaveKey(t, raw, "tmuxVersion")
	mustHaveKey(t, raw, "runningCount")
	mustHaveKey(t, raw, "adapters")
	// M3 additions.
	mustHaveKey(t, raw, "vcs")
	mustHaveKey(t, raw, "worktreeCount")
}

func mustHaveKey(t *testing.T, m map[string]json.RawMessage, key string) {
	t.Helper()
	if _, ok := m[key]; !ok {
		t.Errorf("response missing key %q", key)
	}
}

// ── T019/T020: SetCapacity handler ────────────────────────────────────────────

type fakeCapacitySetter struct {
	gotN     int
	setErr   error
	snapshot health.SchedulerSnapshotDTO
}

func (f *fakeCapacitySetter) SetCapacity(n int) error {
	f.gotN = n
	if f.setErr != nil {
		return f.setErr
	}
	return nil
}

func (f *fakeCapacitySetter) SchedulerSnapshot() health.SchedulerSnapshotDTO {
	return f.snapshot
}

func TestSetCapacityHandler_ValidCapacity(t *testing.T) {
	fake := &fakeCapacitySetter{snapshot: health.SchedulerSnapshotDTO{Capacity: 5, ActiveCount: 1, Waiting: []string{}}}
	h := health.NewOrchestratorHandler(fake)

	body := `{"capacity":5}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/orchestrator/capacity", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.SetCapacity(w, req)

	res := w.Result()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("want 200 got %d", res.StatusCode)
	}
	if fake.gotN != 5 {
		t.Errorf("want capacity=5 got %d", fake.gotN)
	}
	var snap health.SchedulerSnapshotDTO
	if err := json.NewDecoder(res.Body).Decode(&snap); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if snap.Capacity != 5 {
		t.Errorf("response capacity want 5 got %d", snap.Capacity)
	}
}

func TestSetCapacityHandler_MissingBody(t *testing.T) {
	fake := &fakeCapacitySetter{}
	h := health.NewOrchestratorHandler(fake)

	req := httptest.NewRequest(http.MethodPut, "/api/v1/orchestrator/capacity", strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.SetCapacity(w, req)

	if w.Result().StatusCode != http.StatusBadRequest {
		t.Errorf("want 400 got %d", w.Result().StatusCode)
	}
}

func TestSetCapacityHandler_InvalidCapacity(t *testing.T) {
	fake := &fakeCapacitySetter{setErr: fmt.Errorf("capacity must be > 0")}
	h := health.NewOrchestratorHandler(fake)

	body := `{"capacity":0}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/orchestrator/capacity", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.SetCapacity(w, req)

	if w.Result().StatusCode != http.StatusBadRequest {
		t.Errorf("want 400 got %d", w.Result().StatusCode)
	}
}

// ── T021: M4 scheduler fields in status DTO ────────────────────────────────────

type fakeSchedulerSnapshotter struct {
	snap health.SchedulerSnapshotDTO
}

func (f *fakeSchedulerSnapshotter) SetCapacity(_ int) error { return nil }
func (f *fakeSchedulerSnapshotter) SchedulerSnapshot() health.SchedulerSnapshotDTO {
	return f.snap
}

func TestOrchestratorStatus_M4_SchedulerFields(t *testing.T) {
	snap := health.SchedulerSnapshotDTO{
		Capacity:    4,
		ActiveCount: 2,
		Waiting:     []string{"bd-001", "bd-002"},
	}
	cfg := health.StatusConfig{
		BeadsVersion:         "1.0.0",
		SchemaVersion:        1,
		SchedulerSnapshotter: &fakeSchedulerSnapshotter{snap: snap},
	}
	handler := health.OrchestratorStatusHandler(cfg)
	req := httptest.NewRequest(http.MethodGet, "/status", nil)
	w := httptest.NewRecorder()
	handler(w, req)

	var body health.OrchestratorStatusResponse
	if err := json.NewDecoder(w.Result().Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Capacity != 4 {
		t.Errorf("capacity want 4 got %d", body.Capacity)
	}
	if body.ActiveCount != 2 {
		t.Errorf("activeCount want 2 got %d", body.ActiveCount)
	}
	if len(body.Waiting) != 2 || body.Waiting[0] != "bd-001" {
		t.Errorf("waiting want [bd-001 bd-002] got %v", body.Waiting)
	}
}

func TestOrchestratorStatus_M4_M3FieldsIntact(t *testing.T) {
	cfg := health.StatusConfig{
		BeadsVersion:  "2.0.0",
		BeadsDir:      "/data/beads",
		SchemaVersion: 2,
		TmuxAvailable: true,
		TmuxVersion:   "3.6",
		RunCounter:    &fakeRunCounter{count: 1},
		VCS: health.VCSStatus{
			DefaultVCS: "git",
			Git:        health.VCSAvailability{Available: true},
		},
		WorktreeCount: 2,
		SchedulerSnapshotter: &fakeSchedulerSnapshotter{snap: health.SchedulerSnapshotDTO{
			Capacity: 4, ActiveCount: 1, Waiting: []string{},
		}},
	}
	handler := health.OrchestratorStatusHandler(cfg)
	req := httptest.NewRequest(http.MethodGet, "/status", nil)
	w := httptest.NewRecorder()
	handler(w, req)

	var raw map[string]json.RawMessage
	if err := json.NewDecoder(w.Result().Body).Decode(&raw); err != nil {
		t.Fatalf("decode: %v", err)
	}
	// M4 additions.
	mustHaveKey(t, raw, "capacity")
	mustHaveKey(t, raw, "activeCount")
	mustHaveKey(t, raw, "waiting")
	// Prior-milestone fields still present.
	mustHaveKey(t, raw, "build")
	mustHaveKey(t, raw, "vcs")
	mustHaveKey(t, raw, "worktreeCount")
}
