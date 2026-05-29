package api

import (
	"io/fs"
	"net/http"
	"strings"

	"github.com/gitrgoliveira/muster/internal/api/beads"
	"github.com/gitrgoliveira/muster/internal/api/health"
	"github.com/gitrgoliveira/muster/internal/api/middleware"
	"github.com/gitrgoliveira/muster/internal/api/render"
	"github.com/gitrgoliveira/muster/internal/api/stream"
	"github.com/gitrgoliveira/muster/internal/services"
	"github.com/gitrgoliveira/muster/internal/ws"
	"github.com/go-chi/chi/v5"
)

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

	// Health endpoints.
	r.Get("/api/v1/healthz", health.HealthzHandler)
	r.Get("/api/v1/orchestrator/status", health.OrchestratorStatusHandler(statusCfg))

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
