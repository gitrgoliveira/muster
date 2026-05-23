package services

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gitrgoliveira/muster/internal/core"
	"github.com/gitrgoliveira/muster/internal/store"
	"github.com/gitrgoliveira/muster/internal/ws"
)

// CreateBeadInput is the post-validation, fully-defaulted form of a create
// request. Built by the API handler from the HTTP DTO.
type CreateBeadInput struct {
	Title        string
	Desc         string
	Type         core.BeadType
	Column       core.Column
	Priority     core.Priority
	Labels       []string
	VCS          core.VCS
	TokensBudget int
}

// PatchBeadInput is the post-validation patch shape passed from the API handler.
type PatchBeadInput struct {
	Title        *string
	Desc         *string
	Type         *core.BeadType
	Column       *core.Column
	Priority     *core.Priority
	Labels       *[]string
	Ready        *bool
	TokensBudget *int
}

// MoveInput carries the validated move parameters.
type MoveInput struct {
	ToColumn core.Column
	BeforeID string
}

// DispatchInput carries the validated dispatch parameters.
type DispatchInput struct {
	Agent core.AgentID
	Mode  core.Mode
}

// CommentInput carries the validated comment parameters.
type CommentInput struct {
	Actor string
	Note  string
}

// ServiceError wraps a validation or state error with a code understood by
// the API layer.
type ServiceError struct {
	Code    string
	Message string
}

func (e *ServiceError) Error() string { return fmt.Sprintf("%s: %s", e.Code, e.Message) }

const (
	CodeInvalidRequest = "INVALID_REQUEST"
	CodeInvalidState   = "INVALID_STATE"
	CodeNotFound       = "BEAD_NOT_FOUND"
	CodeInternal       = "INTERNAL"
)

// BeadService implements business logic on top of the store.
type BeadService struct {
	store   store.Store
	publish Publisher
}

// NewBeadService constructs a BeadService.
func NewBeadService(s store.Store, pub Publisher) *BeadService {
	return &BeadService{store: s, publish: pub}
}

// Create validates input, applies defaults, stores the bead, and publishes a
// bead.created event.
func (svc *BeadService) Create(ctx context.Context, input CreateBeadInput) (*core.Bead, error) {
	title := strings.TrimSpace(input.Title)
	if title == "" {
		return nil, &ServiceError{Code: CodeInvalidRequest, Message: "title is required"}
	}
	if utf8.RuneCountInString(title) > 255 {
		return nil, &ServiceError{Code: CodeInvalidRequest, Message: "title exceeds 255 chars"}
	}
	if input.Type != "" && !input.Type.Valid() {
		return nil, &ServiceError{Code: CodeInvalidRequest, Message: "invalid type"}
	}
	if !input.Priority.Valid() {
		return nil, &ServiceError{Code: CodeInvalidRequest, Message: "invalid priority"}
	}

	b := core.Bead{
		Title:        title,
		Desc:         input.Desc,
		Type:         input.Type,
		Column:       input.Column,
		Priority:     input.Priority,
		Labels:       input.Labels,
		VCS:          input.VCS,
		TokensBudget: input.TokensBudget,
	}
	applyCreateDefaults(&b)
	b.Estimate = core.DeriveEstimate(b.TokensBudget)

	stored, err := svc.store.Create(ctx, b)
	if err != nil {
		return nil, &ServiceError{Code: CodeInternal, Message: err.Error()}
	}

	svc.publish(ws.Frame{Type: ws.EventBeadCreated, Bead: stored})
	return stored, nil
}

// Patch validates and applies a partial update to a bead.
func (svc *BeadService) Patch(ctx context.Context, id string, input PatchBeadInput) (*core.Bead, error) {
	// Reject empty PATCH bodies.
	if input.Title == nil && input.Desc == nil && input.Type == nil &&
		input.Column == nil && input.Priority == nil && input.Labels == nil &&
		input.Ready == nil && input.TokensBudget == nil {
		return nil, &ServiceError{Code: CodeInvalidRequest, Message: "patch body must contain at least one field"}
	}

	if input.Type != nil && !input.Type.Valid() {
		return nil, &ServiceError{Code: CodeInvalidRequest, Message: "invalid type"}
	}
	if input.Priority != nil && !input.Priority.Valid() {
		return nil, &ServiceError{Code: CodeInvalidRequest, Message: "invalid priority"}
	}
	if input.Title != nil {
		t := strings.TrimSpace(*input.Title)
		if t == "" {
			return nil, &ServiceError{Code: CodeInvalidRequest, Message: "title is required"}
		}
		input.Title = &t
	}

	sp := store.PatchBeadInput{
		Title:        input.Title,
		Desc:         input.Desc,
		Type:         input.Type,
		Column:       input.Column,
		Priority:     input.Priority,
		Labels:       input.Labels,
		Ready:        input.Ready,
		TokensBudget: input.TokensBudget,
	}
	updated, err := svc.store.Patch(ctx, id, sp)
	if err != nil {
		if err == store.ErrNotFound {
			return nil, &ServiceError{Code: CodeNotFound, Message: "bead not found"}
		}
		return nil, &ServiceError{Code: CodeInternal, Message: err.Error()}
	}

	svc.publish(ws.Frame{Type: ws.EventBeadUpdated, Bead: updated})
	return updated, nil
}

// Move places a bead in a target column.
func (svc *BeadService) Move(ctx context.Context, id string, input MoveInput) (*core.Bead, error) {
	if !input.ToColumn.Valid() {
		return nil, &ServiceError{Code: CodeInvalidRequest, Message: "invalid toColumn"}
	}

	orig, err := svc.store.Get(ctx, id)
	if err != nil {
		if err == store.ErrNotFound {
			return nil, &ServiceError{Code: CodeNotFound, Message: "bead not found"}
		}
		return nil, &ServiceError{Code: CodeInternal, Message: err.Error()}
	}
	fromColumn := orig.Column

	updated, err := svc.store.Move(ctx, id, string(input.ToColumn), input.BeforeID)
	if err != nil {
		switch err {
		case store.ErrNotFound:
			return nil, &ServiceError{Code: CodeNotFound, Message: "bead not found"}
		case store.ErrBeforeIDNotFound:
			return nil, &ServiceError{Code: CodeInvalidRequest, Message: "no such beforeID: " + input.BeforeID}
		case store.ErrBeforeIDDifferentColumn:
			return nil, &ServiceError{Code: CodeInvalidRequest, Message: "beforeID must be in toColumn"}
		case store.ErrBeforeIDSameAsMoved:
			return nil, &ServiceError{Code: CodeInvalidRequest, Message: "bead cannot insert before itself"}
		default:
			return nil, &ServiceError{Code: CodeInternal, Message: err.Error()}
		}
	}

	svc.publish(ws.Frame{
		Type:       ws.EventBeadMoved,
		Bead:       updated,
		ID:         id,
		FromColumn: fromColumn,
		ToColumn:   input.ToColumn,
		BeforeID:   input.BeforeID,
	})
	return updated, nil
}

// Dispatch transitions a scheduled bead to running.
func (svc *BeadService) Dispatch(ctx context.Context, id string, input DispatchInput) (*core.Bead, error) {
	if !input.Agent.Valid() {
		return nil, &ServiceError{Code: CodeInvalidRequest, Message: "invalid agent"}
	}
	if !input.Mode.Valid() {
		return nil, &ServiceError{Code: CodeInvalidRequest, Message: "invalid mode"}
	}

	updated, err := svc.store.Dispatch(ctx, id, store.DispatchRequest{
		Agent: input.Agent,
		Mode:  input.Mode,
	})
	if err != nil {
		switch err {
		case store.ErrNotFound:
			return nil, &ServiceError{Code: CodeNotFound, Message: "bead not found"}
		case store.ErrInvalidState:
			return nil, &ServiceError{Code: CodeInvalidState, Message: "cannot dispatch bead in current column"}
		default:
			return nil, &ServiceError{Code: CodeInternal, Message: err.Error()}
		}
	}

	svc.publish(ws.Frame{Type: ws.EventBeadUpdated, Bead: updated})
	return updated, nil
}

// AddComment appends a lifecycle comment to a bead.
func (svc *BeadService) AddComment(ctx context.Context, id string, input CommentInput) (*core.Bead, error) {
	actor := strings.TrimSpace(input.Actor)
	note := strings.TrimSpace(input.Note)
	if actor == "" {
		return nil, &ServiceError{Code: CodeInvalidRequest, Message: "actor is required"}
	}
	if note == "" {
		return nil, &ServiceError{Code: CodeInvalidRequest, Message: "note is required"}
	}

	updated, err := svc.store.AddComment(ctx, id, store.CommentRequest{Actor: actor, Note: note})
	if err != nil {
		if err == store.ErrNotFound {
			return nil, &ServiceError{Code: CodeNotFound, Message: "bead not found"}
		}
		return nil, &ServiceError{Code: CodeInternal, Message: err.Error()}
	}

	// Find the comment event that was just appended.
	var commentEvent *core.HistoryEvent
	for i := len(updated.History) - 1; i >= 0; i-- {
		if updated.History[i].Kind == core.EvComment {
			ev := updated.History[i]
			commentEvent = &ev
			break
		}
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if commentEvent == nil {
		commentEvent = &core.HistoryEvent{Kind: core.EvComment, Actor: actor, Note: note, At: now}
	}

	svc.publish(ws.Frame{
		Type:  ws.EventCommentAdded,
		Bead:  updated,
		ID:    id,
		Event: commentEvent,
	})
	return updated, nil
}
