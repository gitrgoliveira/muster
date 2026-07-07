// Package constitution serves the M6 constitution CRUD endpoints. Handlers stay
// thin (Constitution III): decode/validate, call the service, render.
package constitution

import (
	"encoding/json"
	"net/http"

	"github.com/gitrgoliveira/muster/internal/api/render"
	"github.com/gitrgoliveira/muster/internal/services"
)

// Handlers serves GET/PUT /api/v1/constitution.
type Handlers struct {
	svc *services.ConstitutionService
}

// NewHandlers constructs a Handlers backed by the constitution service.
func NewHandlers(svc *services.ConstitutionService) *Handlers {
	return &Handlers{svc: svc}
}

// Get returns the current constitution. A fresh install returns
// {markdown:"", version:0, updatedAt:null} — never a 404.
func (h *Handlers) Get(w http.ResponseWriter, r *http.Request) {
	render.WriteJSON(w, http.StatusOK, toResponse(h.svc.Get()))
}

// Put overwrites the constitution and returns the new versioned document. The
// body is bounded by the router's BodyLimit middleware.
func (h *Handlers) Put(w http.ResponseWriter, r *http.Request) {
	var req putRequest
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		render.WriteError(w, r, http.StatusBadRequest, render.CodeInvalidRequest, "invalid request body")
		return
	}
	c, err := h.svc.Set(req.Markdown)
	if err != nil {
		render.WriteError(w, r, http.StatusInternalServerError, render.CodeInternal, "failed to persist constitution")
		return
	}
	render.WriteJSON(w, http.StatusOK, toResponse(c))
}
