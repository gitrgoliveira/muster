package health

import (
	"net/http"
	"time"

	"github.com/gitrgoliveira/muster/internal/api/render"
	"github.com/gitrgoliveira/muster/internal/core"
)

// HealthzHandler handles GET /api/v1/healthz.
// It always responds 200 OK with {"ok":true}.
func HealthzHandler(w http.ResponseWriter, r *http.Request) {
	render.WriteJSON(w, http.StatusOK, HealthzResponse{OK: true})
}

// OrchestratorStatusHandler returns an http.HandlerFunc closure that captures
// beadsVersion and seedDolt from the constructor (NOT hardcoded in the handler).
func OrchestratorStatusHandler(beadsVersion string, seedDolt core.DoltStatus) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		resp := OrchestratorStatusResponse{
			Build:         "dev",
			SchemaVersion: 1,
			BeadsVersion:  beadsVersion,
			Online:        true,
			ServerTime:    time.Now().UTC().Format(time.RFC3339),
			Dolt:          seedDolt,
		}
		render.WriteJSON(w, http.StatusOK, resp)
	}
}
