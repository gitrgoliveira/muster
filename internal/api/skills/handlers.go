// Package skills serves the M6 skill-registry endpoints. Handlers stay thin:
// decode/validate, call the service, render.
package skills

import (
	"encoding/json"
	"net/http"

	"github.com/gitrgoliveira/muster/internal/api/render"
	"github.com/gitrgoliveira/muster/internal/services"
	"github.com/gitrgoliveira/muster/internal/skills"
	"github.com/go-chi/chi/v5"
)

// Handlers serves the /api/v1/skills* routes.
type Handlers struct {
	svc *services.SkillService
}

// NewHandlers constructs a Handlers backed by the skill service.
func NewHandlers(svc *services.SkillService) *Handlers {
	return &Handlers{svc: svc}
}

type listResponse struct {
	Skills []skills.Skill `json:"skills"`
}

type categoriesResponse struct {
	Categories []string `json:"categories"`
}

type importRequest struct {
	URL string `json:"url"`
}

// List returns the full registry (built-in + imported).
func (h *Handlers) List(w http.ResponseWriter, r *http.Request) {
	render.WriteJSON(w, http.StatusOK, listResponse{Skills: h.svc.List()})
}

// Categories returns the distinct categories across the registry.
func (h *Handlers) Categories(w http.ResponseWriter, r *http.Request) {
	render.WriteJSON(w, http.StatusOK, categoriesResponse{Categories: h.svc.Categories()})
}

// Import imports a skill from a URL (body-limited by the router middleware).
func (h *Handlers) Import(w http.ResponseWriter, r *http.Request) {
	var req importRequest
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		render.WriteError(w, r, http.StatusBadRequest, render.CodeInvalidRequest, "invalid request body")
		return
	}
	if req.URL == "" {
		render.WriteError(w, r, http.StatusBadRequest, render.CodeInvalidRequest, "url is required")
		return
	}
	sk, err := h.svc.Import(req.URL)
	if writeServiceError(w, r, err) {
		return
	}
	render.WriteJSON(w, http.StatusCreated, sk)
}

// Delete removes an imported skill; a built-in id fails with SKILL_READONLY.
func (h *Handlers) Delete(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.Delete(chi.URLParam(r, "id")); writeServiceError(w, r, err) {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// writeServiceError renders a ServiceError, deriving the HTTP status from its
// code (the service uses codes the render layer knows). Returns true if an
// error was written.
func writeServiceError(w http.ResponseWriter, r *http.Request, err error) bool {
	if err == nil {
		return false
	}
	se, ok := err.(*services.ServiceError)
	if !ok {
		render.WriteError(w, r, http.StatusInternalServerError, render.CodeInternal, "internal server error")
		return true
	}
	render.WriteError(w, r, 0, se.Code, se.Message) // status derived from code
	return true
}
