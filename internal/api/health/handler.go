package health

import (
	"context"
	"net/http"
	"time"

	"github.com/gitrgoliveira/muster/internal/api/render"
)

// Pinger is implemented by store.Backend.
type Pinger interface {
	Ping(ctx context.Context) error
}

// RunCounter is implemented by the orchestrator to report active run count.
type RunCounter interface {
	RunCount() int
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
		}
		render.WriteJSON(w, http.StatusOK, resp)
	}
}
