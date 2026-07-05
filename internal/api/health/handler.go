package health

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/gitrgoliveira/muster/internal/api/render"
	"github.com/gitrgoliveira/muster/internal/services"
)

// Pinger is implemented by store.Backend.
type Pinger interface {
	Ping(ctx context.Context) error
}

// SchedulerSnapshotter is implemented by services.BeadService (or a test double)
// to expose scheduler state and capacity management to the health handler.
// Defined here (not in services) to avoid import cycles.
type SchedulerSnapshotter interface {
	// SetCapacity changes the scheduler's maximum concurrency at runtime.
	// n must be > 0; returns an error otherwise.
	SetCapacity(n int) error
	// SchedulerSnapshot returns the current scheduler state.
	SchedulerSnapshot() SchedulerSnapshotDTO
}

// OrchestratorHandler handles orchestrator management endpoints
// (PUT /orchestrator/capacity). It is separate from OrchestratorStatusHandler
// so the handler can hold a reference to the service without changing the
// existing StatusConfig closure pattern.
type OrchestratorHandler struct {
	sched SchedulerSnapshotter
}

// NewOrchestratorHandler constructs an OrchestratorHandler with the given
// scheduler management interface.
func NewOrchestratorHandler(sched SchedulerSnapshotter) *OrchestratorHandler {
	return &OrchestratorHandler{sched: sched}
}

// SetCapacity handles PUT /api/v1/orchestrator/capacity.
//
// Request body: {"capacity": N}  (N must be > 0)
// Response 200: SchedulerSnapshotDTO
// Response 400: JSON error with code INVALID_CAPACITY or INVALID_REQUEST (unknown fields)
func (h *OrchestratorHandler) SetCapacity(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Capacity int `json:"capacity"`
	}
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&body); err != nil {
		render.WriteError(w, r, http.StatusBadRequest, render.CodeInvalidRequest, "invalid JSON body")
		return
	}
	if body.Capacity <= 0 {
		render.WriteError(w, r, http.StatusBadRequest, services.CodeInvalidCapacity, "capacity must be > 0")
		return
	}
	if err := h.sched.SetCapacity(body.Capacity); err != nil {
		render.WriteError(w, r, http.StatusBadRequest, services.CodeInvalidCapacity, err.Error())
		return
	}
	snap := h.sched.SchedulerSnapshot()
	render.WriteJSON(w, http.StatusOK, snap)
}

// RunCounter is implemented by the orchestrator to report active run count.
type RunCounter interface {
	RunCount() int
}

// RunLister is implemented by the orchestrator to provide per-run summaries for
// the status endpoint (T047). Returns a snapshot list — callers must not mutate it.
// May be nil; Runs will be an empty (non-null) slice in that case.
type RunLister interface {
	// ListRuns returns a point-in-time summary of all tracked runs.
	ListRuns() []RunSummaryDTO
}

// WorktreeCounter is implemented by the orchestrator to report the number of
// per-bead worktree directories under the configured --worktrees-dir.
type WorktreeCounter interface {
	WorktreeCount() int
}

// StatusConfig carries the configuration captured at startup for the status endpoint.
type StatusConfig struct {
	BeadsVersion  string
	BeadsDir      string
	DoltDatabase  string
	DoltMode      string
	ReadSource    string
	BdCLI         string
	ProjectID     string
	SchemaVersion int
	Pinger        Pinger

	// M2 additions.
	TmuxAvailable bool
	TmuxVersion   string
	Adapters      []AdapterInfo
	RunCounter    RunCounter // may be nil

	// M3 additions (FR-012: additive only).
	// VCS describes VCS backend availability at startup.
	VCS VCSStatus
	// WorktreeCount is the current count of per-bead worktree directories.
	// May be supplied directly or via WorktreeCounter (counter takes priority).
	WorktreeCount   int
	WorktreeCounter WorktreeCounter // may be nil; takes priority over WorktreeCount

	// M4 additions (additive — all M0–M3 fields unchanged).
	// SchedulerSnapshotter provides live scheduler state for the status response.
	// May be nil; the scheduler fields (capacity, activeCount, waiting) are
	// always present in the response (none are omitempty) — when nil they carry
	// zero/empty values (capacity 0, activeCount 0, waiting []), not omitted.
	SchedulerSnapshotter SchedulerSnapshotter

	// RunLister provides per-run summaries (stepIdx, chainLen) for T047.
	// May be nil; Runs field will be an empty (non-null) slice when nil.
	RunLister RunLister
}

// HealthzHandler handles GET /api/v1/healthz.
// It always responds 200 OK with {"ok":true}.
func HealthzHandler(w http.ResponseWriter, r *http.Request) {
	render.WriteJSON(w, http.StatusOK, HealthzResponse{OK: true})
}

// OrchestratorStatusHandler returns an http.HandlerFunc closure that captures
// the status configuration from the constructor.
func OrchestratorStatusHandler(cfg StatusConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		online := true
		if cfg.Pinger != nil {
			pingCtx, pingCancel := context.WithTimeout(r.Context(), 2*time.Second)
			online = cfg.Pinger.Ping(pingCtx) == nil
			pingCancel()
		}

		schemaVersion := cfg.SchemaVersion
		if schemaVersion == 0 {
			schemaVersion = 1
		}

		runningCount := 0
		if cfg.RunCounter != nil {
			runningCount = cfg.RunCounter.RunCount()
		}

		worktreeCount := cfg.WorktreeCount
		if cfg.WorktreeCounter != nil {
			worktreeCount = cfg.WorktreeCounter.WorktreeCount()
		}

		// M4: read live scheduler state. Waiting defaults to a non-nil empty
		// slice so it marshals as [] (not null) when no snapshotter is wired,
		// consistent with runs below.
		schedSnap := SchedulerSnapshotDTO{Waiting: []string{}}
		if cfg.SchedulerSnapshotter != nil {
			schedSnap = cfg.SchedulerSnapshotter.SchedulerSnapshot()
			if schedSnap.Waiting == nil {
				schedSnap.Waiting = []string{}
			}
		}

		// T047: read per-run summaries (stepIdx, chainLen).
		runs := []RunSummaryDTO{} // never nil; clients get [] not null
		if cfg.RunLister != nil {
			if listed := cfg.RunLister.ListRuns(); len(listed) > 0 {
				runs = listed
			}
		}

		resp := OrchestratorStatusResponse{
			Build:         "dev",
			SchemaVersion: schemaVersion,
			BeadsVersion:  cfg.BeadsVersion,
			Online:        online,
			ServerTime:    time.Now().UTC().Format(time.RFC3339),
			BeadsDir:      cfg.BeadsDir,
			DoltDatabase:  cfg.DoltDatabase,
			DoltMode:      cfg.DoltMode,
			ReadSource:    cfg.ReadSource,
			BdCLI:         cfg.BdCLI,
			ProjectID:     cfg.ProjectID,
			// M2 additions.
			TmuxAvailable: cfg.TmuxAvailable,
			TmuxVersion:   cfg.TmuxVersion,
			RunningCount:  runningCount,
			Adapters:      cfg.Adapters,
			// M3 additions.
			VCS:           cfg.VCS,
			WorktreeCount: worktreeCount,
			// M4 additions.
			Capacity:    schedSnap.Capacity,
			ActiveCount: schedSnap.ActiveCount,
			Waiting:     schedSnap.Waiting,
			// T047: per-run step chain progress.
			Runs: runs,
		}
		render.WriteJSON(w, http.StatusOK, resp)
	}
}
