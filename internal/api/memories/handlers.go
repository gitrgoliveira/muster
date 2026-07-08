// Package memories serves the M6 memories endpoints — a thin facade over bd's
// remember/recall/forget/memories primitives. Handlers stay thin.
package memories

import (
	"encoding/json"
	"net/http"

	"github.com/gitrgoliveira/muster/internal/api/render"
	"github.com/gitrgoliveira/muster/internal/services"
	"github.com/go-chi/chi/v5"
)

// Handlers serves the /api/v1/memories* routes.
type Handlers struct {
	svc *services.MemoriesService
}

// NewHandlers constructs a Handlers backed by the memories service.
func NewHandlers(svc *services.MemoriesService) *Handlers {
	return &Handlers{svc: svc}
}

type listResponse struct {
	Memories []services.Memory `json:"memories"`
}

type upsertRequest struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type primeRequest struct {
	BeadID string `json:"beadID"`
}

type primeResponse struct {
	Primed int `json:"primed"`
}

// List returns memories, optionally filtered by ?q=.
func (h *Handlers) List(w http.ResponseWriter, r *http.Request) {
	mems, err := h.svc.List(r.Context(), r.URL.Query().Get("q"))
	if writeServiceError(w, r, err) {
		return
	}
	render.WriteJSON(w, http.StatusOK, listResponse{Memories: mems})
}

// Upsert creates or updates a memory (bd derives a key when none is given).
func (h *Handlers) Upsert(w http.ResponseWriter, r *http.Request) {
	var req upsertRequest
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		render.WriteError(w, r, http.StatusBadRequest, render.CodeInvalidRequest, "invalid request body")
		return
	}
	m, err := h.svc.Upsert(r.Context(), req.Key, req.Value)
	if writeServiceError(w, r, err) {
		return
	}
	render.WriteJSON(w, http.StatusOK, m)
}

// Delete removes a memory by key (404 when absent).
func (h *Handlers) Delete(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.Delete(r.Context(), chi.URLParam(r, "key")); writeServiceError(w, r, err) {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Prime snapshots current memories for a bead's next dispatch.
func (h *Handlers) Prime(w http.ResponseWriter, r *http.Request) {
	var req primeRequest
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		render.WriteError(w, r, http.StatusBadRequest, render.CodeInvalidRequest, "invalid request body")
		return
	}
	n, err := h.svc.Prime(r.Context(), req.BeadID)
	if writeServiceError(w, r, err) {
		return
	}
	render.WriteJSON(w, http.StatusOK, primeResponse{Primed: n})
}

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
