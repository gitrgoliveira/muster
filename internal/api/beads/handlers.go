package beads

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/gitrgoliveira/muster/internal/api/render"
	"github.com/gitrgoliveira/muster/internal/core"
	"github.com/gitrgoliveira/muster/internal/services"
	"github.com/go-chi/chi/v5"
)

// Handlers groups all bead-related HTTP handlers.
type Handlers struct {
	svc *services.BeadService
}

// NewHandlers constructs a Handlers backed by the given service.
func NewHandlers(svc *services.BeadService) *Handlers {
	return &Handlers{svc: svc}
}

// mapServiceError translates a ServiceError into an HTTP response. Returns true
// if an error was written, false if err is nil.
func mapServiceError(w http.ResponseWriter, r *http.Request, err error) bool {
	if err == nil {
		return false
	}
	se, ok := err.(*services.ServiceError)
	if !ok {
		render.WriteError(w, r, http.StatusInternalServerError, render.CodeInternal, "internal server error")
		return true
	}
	switch se.Code {
	case services.CodeNotFound:
		render.WriteError(w, r, http.StatusNotFound, render.CodeBeadNotFound, se.Message)
	case services.CodeInvalidRequest:
		render.WriteError(w, r, http.StatusBadRequest, render.CodeInvalidRequest, se.Message)
	case services.CodeInvalidState:
		render.WriteError(w, r, http.StatusBadRequest, render.CodeInvalidState, se.Message)
	case services.CodeCLIMissing:
		render.WriteError(w, r, http.StatusNotImplemented, render.CodeCLIMissing, se.Message)
	case services.CodeCLIValidation:
		render.WriteError(w, r, http.StatusUnprocessableEntity, render.CodeCLIValidation, se.Message)
	case services.CodeCLIUnavailable:
		render.WriteError(w, r, http.StatusServiceUnavailable, render.CodeStoreUnavailable, se.Message)
	case services.CodeCLITimeout:
		render.WriteError(w, r, http.StatusGatewayTimeout, render.CodeGatewayTimeout, se.Message)
	case services.CodeInternal:
		render.WriteError(w, r, http.StatusInternalServerError, render.CodeInternal, se.Message)
	// M2 dispatch error codes.
	case services.CodeRunAlreadyActive:
		render.WriteError(w, r, http.StatusConflict, se.Code, se.Message)
	case services.CodeUnmappedPrefix:
		render.WriteError(w, r, http.StatusUnprocessableEntity, se.Code, se.Message)
	case services.CodeAdapterNotFound:
		render.WriteError(w, r, http.StatusNotImplemented, se.Code, se.Message)
	case services.CodeAdapterNotInstalled:
		render.WriteError(w, r, http.StatusNotImplemented, se.Code, se.Message)
	case services.CodeAdapterNotLoggedIn:
		render.WriteError(w, r, http.StatusConflict, se.Code, se.Message)
	default:
		render.WriteError(w, r, http.StatusInternalServerError, render.CodeInternal, se.Message)
	}
	return true
}

// validateID checks the bead ID format. Returns false and writes 400 if invalid.
func validateID(w http.ResponseWriter, r *http.Request, id string) bool {
	if !core.ValidBeadID(id) {
		render.WriteError(w, r, http.StatusBadRequest, render.CodeInvalidRequest, "invalid bead ID format")
		return false
	}
	return true
}

// parseStepIdx parses and validates the {idx} path param shared by Attach and
// Send. M2 only supports step 0, so it requires the canonical "0" exactly and
// writes a 404 (unknown step) with ok=false for anything else. Requiring the
// literal "0" (rather than strconv.Atoi(idxStr)==0) rejects non-canonical zero
// forms — "-0", "+0", "00" — that the contract doesn't define, keeping routing
// unambiguous.
func parseStepIdx(w http.ResponseWriter, r *http.Request, idxStr string) (idx int, ok bool) {
	if idxStr != "0" {
		render.WriteError(w, r, http.StatusNotFound, render.CodeNotFound, "step index not found")
		return 0, false
	}
	return 0, true
}

// decodeJSON decodes the request body with DisallowUnknownFields. On error,
// it writes the appropriate error response and returns false.
func decodeJSON(w http.ResponseWriter, r *http.Request, v interface{}) bool {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		msg := err.Error()
		if strings.Contains(msg, "unknown field") {
			const prefix = `json: unknown field "`
			if idx := strings.Index(msg, prefix); idx >= 0 {
				rest := msg[idx+len(prefix):]
				if end := strings.Index(rest, `"`); end >= 0 {
					field := rest[:end]
					msg = "unknown field: " + field
				}
			}
		}
		render.WriteError(w, r, http.StatusBadRequest, render.CodeInvalidRequest, msg)
		return false
	}
	return true
}

// List handles GET /beads.
func (h *Handlers) List(w http.ResponseWriter, r *http.Request) {
	column := r.URL.Query().Get("column")
	if column != "" && !core.Column(column).Valid() {
		render.WriteError(w, r, http.StatusBadRequest, render.CodeInvalidRequest, "invalid column: "+column)
		return
	}

	beads, err := h.svc.ListBeads(r.Context(), column)
	if mapServiceError(w, r, err) {
		return
	}

	render.WriteJSON(w, http.StatusOK, ListResponse{
		Items:      beads,
		NextCursor: nil,
		Total:      len(beads),
	})
}

// Get handles GET /beads/{id}.
func (h *Handlers) Get(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if !validateID(w, r, id) {
		return
	}

	bead, err := h.svc.GetBead(r.Context(), id)
	if mapServiceError(w, r, err) {
		return
	}

	render.WriteJSON(w, http.StatusOK, bead)
}

// Create handles POST /beads.
func (h *Handlers) Create(w http.ResponseWriter, r *http.Request) {
	var req CreateRequest
	if !decodeJSON(w, r, &req) {
		return
	}

	priority := core.Priority(2)
	if req.Priority != nil {
		priority = *req.Priority
	}

	input := services.CreateBeadInput{
		Title:        req.Title,
		Desc:         req.Desc,
		Type:         req.Type,
		Column:       req.Column,
		Priority:     priority,
		Assignee:     req.Assignee,
		Labels:       req.Labels,
		VCS:          req.VCS,
		TokensBudget: req.TokensBudget,
	}

	bead, err := h.svc.Create(r.Context(), input)
	if mapServiceError(w, r, err) {
		return
	}

	w.Header().Set("Location", "/api/v1/beads/"+bead.ID)
	render.WriteJSON(w, http.StatusCreated, bead)
}

// Patch handles PATCH /beads/{id}.
func (h *Handlers) Patch(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if !validateID(w, r, id) {
		return
	}

	var req PatchRequest
	if !decodeJSON(w, r, &req) {
		return
	}

	// desc and description are documented aliases; accept either but not both.
	desc := req.Desc
	if req.Description != nil {
		if req.Desc != nil {
			render.WriteError(w, r, http.StatusBadRequest, render.CodeInvalidRequest, "specify either desc or description, not both")
			return
		}
		desc = req.Description
	}

	input := services.PatchBeadInput{
		Title:        req.Title,
		Desc:         desc,
		Type:         req.Type,
		Column:       req.Column,
		Priority:     req.Priority,
		Assignee:     req.Assignee,
		Labels:       req.Labels,
		Ready:        req.Ready,
		TokensBudget: req.TokensBudget,
	}

	bead, err := h.svc.Patch(r.Context(), id, input)
	if mapServiceError(w, r, err) {
		return
	}

	render.WriteJSON(w, http.StatusOK, bead)
}

// Move handles POST /beads/{id}/move.
func (h *Handlers) Move(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if !validateID(w, r, id) {
		return
	}

	var req MoveRequest
	if !decodeJSON(w, r, &req) {
		return
	}

	input := services.MoveInput{
		ToColumn: req.ToColumn,
		BeforeID: req.BeforeID,
	}

	bead, err := h.svc.Move(r.Context(), id, input)
	if mapServiceError(w, r, err) {
		return
	}

	render.WriteJSON(w, http.StatusOK, bead)
}

// Dispatch handles POST /beads/{id}/dispatch.
// On success returns 202 Accepted with the bead in running state (FR-002).
func (h *Handlers) Dispatch(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if !validateID(w, r, id) {
		return
	}

	var req DispatchRequest
	if !decodeJSON(w, r, &req) {
		return
	}

	input := services.DispatchInput{
		Agent:          req.Agent,
		Mode:           req.Mode,
		PermissionMode: req.PermissionMode,
	}

	bead, err := h.svc.Dispatch(r.Context(), id, input)
	if mapServiceError(w, r, err) {
		return
	}

	// 202 Accepted — run has been launched asynchronously.
	render.WriteJSON(w, http.StatusAccepted, bead)
}

// Attach handles GET /beads/{id}/steps/{idx}/attach.
// Returns the tmux attach command for the live session (US3).
func (h *Handlers) Attach(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if !validateID(w, r, id) {
		return
	}
	idx, ok := parseStepIdx(w, r, chi.URLParam(r, "idx"))
	if !ok {
		return
	}

	resp, err := h.svc.GetAttach(r.Context(), id, idx)
	if mapServiceError(w, r, err) {
		return
	}
	render.WriteJSON(w, http.StatusOK, resp)
}

// Send handles POST /beads/{id}/steps/{idx}/send.
// Forwards keystrokes to the live tmux pane (US3).
func (h *Handlers) Send(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if !validateID(w, r, id) {
		return
	}
	idx, ok := parseStepIdx(w, r, chi.URLParam(r, "idx"))
	if !ok {
		return
	}

	var req SendRequest
	if !decodeJSON(w, r, &req) {
		return
	}

	// Keys is forwarded verbatim to the running agent's tmux pane — bound its
	// size so an oversized payload can't be used to flood the pane / pipe.
	if req.Keys == "" || len(req.Keys) > maxSendKeysLen {
		render.WriteError(w, r, http.StatusBadRequest, render.CodeInvalidRequest, "keys must be non-empty and at most 4096 bytes")
		return
	}

	if err := h.svc.SendKeys(r.Context(), id, idx, req.Keys); mapServiceError(w, r, err) {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// maxSendKeysLen bounds the Send request's keys payload (forwarded verbatim
// to the agent's tmux pane).
const maxSendKeysLen = 4096

// SendRequest is the body for POST /beads/{id}/steps/{idx}/send.
type SendRequest struct {
	Keys string `json:"keys"`
}

// Comment handles POST /beads/{id}/comments.
func (h *Handlers) Comment(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if !validateID(w, r, id) {
		return
	}

	var req CommentRequest
	if !decodeJSON(w, r, &req) {
		return
	}

	input := services.CommentInput{
		Actor: req.Actor,
		Note:  req.Note,
	}

	bead, err := h.svc.AddComment(r.Context(), id, input)
	if mapServiceError(w, r, err) {
		return
	}

	render.WriteJSON(w, http.StatusCreated, bead)
}
