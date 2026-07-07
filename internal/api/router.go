package api

import (
	"io/fs"
	"net/http"
	"strings"

	"github.com/gitrgoliveira/muster/internal/api/beads"
	"github.com/gitrgoliveira/muster/internal/api/constitution"
	"github.com/gitrgoliveira/muster/internal/api/health"
	"github.com/gitrgoliveira/muster/internal/api/middleware"
	"github.com/gitrgoliveira/muster/internal/api/render"
	skillsapi "github.com/gitrgoliveira/muster/internal/api/skills"
	"github.com/gitrgoliveira/muster/internal/api/stream"
	"github.com/gitrgoliveira/muster/internal/services"
	"github.com/gitrgoliveira/muster/internal/ws"
	"github.com/go-chi/chi/v5"
)

// M6Services carries the optional M6 service dependencies (constitution, and —
// added in later increments — skills and memories). Each field is nil-safe: a
// nil service means its routes are not registered. Threading them via a struct
// keeps NewRouter's signature stable as M6 grows (additive, Principle V).
type M6Services struct {
	Constitution *services.ConstitutionService
	Skills       *services.SkillService
}

// NewRouter constructs the application's HTTP handler.
//
// uiFS should be an fs.FS whose "ui/" sub-directory holds the SPA files.
// NewRouter calls fs.Sub(uiFS, "ui") internally; if that fails it uses uiFS
// directly (allowing callers that already pass a sub-rooted FS).
func NewRouter(
	svc *services.BeadService,
	hub *ws.Hub,
	uiFS fs.FS,
	statusCfg health.StatusConfig,
	m6 M6Services,
) http.Handler {
	r := chi.NewRouter()

	// Global middleware.
	r.Use(middleware.RequestID)
	r.Use(middleware.Recovery)
	r.Use(beadsHeadersMiddleware(statusCfg.BeadsDir, statusCfg.DoltDatabase))

	// Custom 404 / 405 JSON responses for API routes only.
	r.NotFound(func(w http.ResponseWriter, req *http.Request) {
		render.WriteError(w, req, http.StatusNotFound, render.CodeNotFound, "not found")
	})
	r.MethodNotAllowed(func(w http.ResponseWriter, req *http.Request) {
		render.WriteError(w, req, http.StatusMethodNotAllowed, render.CodeMethodNotAllowed, "method not allowed")
	})

	// Health and orchestrator management endpoints.
	r.Get("/api/v1/healthz", health.HealthzHandler)
	r.Get("/api/v1/orchestrator/status", health.OrchestratorStatusHandler(statusCfg))
	if statusCfg.SchedulerSnapshotter != nil {
		oh := health.NewOrchestratorHandler(statusCfg.SchedulerSnapshotter)
		r.With(middleware.BodyLimit).Put("/api/v1/orchestrator/capacity", oh.SetCapacity)
	}

	// WebSocket stream endpoint.
	r.Get("/api/v1/stream", stream.StreamHandler(hub))

	// Bead endpoints (write methods get body-limit middleware).
	h := beads.NewHandlers(svc)
	r.Get("/api/v1/beads", h.List)
	r.With(middleware.BodyLimit).Post("/api/v1/beads", h.Create)
	r.Get("/api/v1/beads/{id}", h.Get)
	r.With(middleware.BodyLimit).Patch("/api/v1/beads/{id}", h.Patch)
	r.With(middleware.BodyLimit).Post("/api/v1/beads/{id}/move", h.Move)
	r.With(middleware.BodyLimit).Post("/api/v1/beads/{id}/dispatch", h.Dispatch)
	r.With(middleware.BodyLimit).Post("/api/v1/beads/{id}/comments", h.Comment)
	// M2 additions: step attach/send endpoints (US3).
	r.Get("/api/v1/beads/{id}/steps/{idx}/attach", h.Attach)
	r.With(middleware.BodyLimit).Post("/api/v1/beads/{id}/steps/{idx}/send", h.Send)
	// M4 additions: operator-driven step advance/loopback (US3).
	r.With(middleware.BodyLimit).Post("/api/v1/beads/{id}/steps/advance", h.AdvanceStep)
	r.With(middleware.BodyLimit).Post("/api/v1/beads/{id}/steps/loopback", h.LoopBackStep)
	// M3 additions: worktree and diff endpoints (US2).
	r.Get("/api/v1/beads/{id}/worktree", h.Worktree)
	r.Get("/api/v1/beads/{id}/diff", h.Diff)
	// M4 additions: worktree write-side endpoints (US2).
	r.With(middleware.BodyLimit).Post("/api/v1/beads/{id}/worktree/finalize", h.FinalizeWorktree)
	r.With(middleware.BodyLimit).Post("/api/v1/beads/{id}/worktree/push", h.PushWorktree)
	r.Delete("/api/v1/beads/{id}/worktree", h.RemoveWorktree)

	// M6 constitution endpoints (additive). Registered only when the service is
	// wired.
	if m6.Constitution != nil {
		ch := constitution.NewHandlers(m6.Constitution)
		r.Get("/api/v1/constitution", ch.Get)
		r.With(middleware.BodyLimit).Put("/api/v1/constitution", ch.Put)
	}
	if m6.Skills != nil {
		sh := skillsapi.NewHandlers(m6.Skills)
		r.Get("/api/v1/skills", sh.List)
		r.Get("/api/v1/skills/categories", sh.Categories)
		r.With(middleware.BodyLimit).Post("/api/v1/skills", sh.Import)
		r.Delete("/api/v1/skills/{id}", sh.Delete)
	}

	// Build the SPA file server.
	subFS, err := fs.Sub(uiFS, "ui")
	if err != nil {
		subFS = uiFS
	}
	spa := spaHandler(subFS)

	// Outer handler: route /api/ through chi; serve everything else via SPA.
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if strings.HasPrefix(req.URL.Path, "/api/") {
			r.ServeHTTP(w, req)
			return
		}
		spa.ServeHTTP(w, req)
	})
}

// beadsHeadersMiddleware adds X-Beads-Dir and X-Beads-Database headers to all responses.
func beadsHeadersMiddleware(beadsDir, doltDatabase string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if beadsDir != "" {
				w.Header().Set("X-Beads-Dir", beadsDir)
			}
			if doltDatabase != "" {
				w.Header().Set("X-Beads-Database", doltDatabase)
			}
			next.ServeHTTP(w, r)
		})
	}
}

// spaHandler serves static files from fsys. Any path that does not resolve to
// an existing file falls back to index.html so that SPA client-side routing works.
func spaHandler(fsys fs.FS) http.Handler {
	fileServer := http.FileServer(http.FS(fsys))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if path == "/" || path == "" {
			path = "index.html"
		} else if strings.HasPrefix(path, "/") {
			path = path[1:]
		}

		f, err := fsys.Open(path)
		if err != nil {
			r2 := r.Clone(r.Context())
			r2.URL.Path = "/"
			fileServer.ServeHTTP(w, r2)
			return
		}
		_ = f.Close()

		fileServer.ServeHTTP(w, r)
	})
}
