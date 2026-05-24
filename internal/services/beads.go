package services

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/gitrgoliveira/muster/internal/core"
	"github.com/gitrgoliveira/muster/internal/store"
)

// CLIRunner abstracts bd CLI write operations.
type CLIRunner interface {
	RunJSON(ctx context.Context, dst interface{}, args ...string) error
	RunVoid(ctx context.Context, args ...string) error
}

const (
	CodeInvalidRequest = "INVALID_REQUEST"
	CodeInvalidState   = "INVALID_STATE"
	CodeNotFound       = "BEAD_NOT_FOUND"
	CodeInternal       = "INTERNAL"
	CodeCLIMissing     = "BD_CLI_MISSING"
)

// ServiceError wraps a validation or state error with a code understood by
// the API layer.
type ServiceError struct {
	Code    string
	Message string
}

func (e *ServiceError) Error() string { return fmt.Sprintf("%s: %s", e.Code, e.Message) }

// CreateBeadInput is the post-validation, fully-defaulted form of a create request.
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

// BeadService implements business logic on top of the store.
type BeadService struct {
	backend store.Backend
	cli     CLIRunner // may be nil; writes return CodeCLIMissing when nil
	repo    string
	publish Publisher
}

// NewBeadService constructs a BeadService.
// cli may be nil; write operations return CodeCLIMissing in that case.
func NewBeadService(backend store.Backend, cli CLIRunner, pub Publisher) *BeadService {
	return &BeadService{backend: backend, cli: cli, publish: pub}
}

// NewBeadServiceWithRepo constructs a BeadService with an explicit repo name.
func NewBeadServiceWithRepo(backend store.Backend, cli CLIRunner, pub Publisher, repo string) *BeadService {
	return &BeadService{backend: backend, cli: cli, repo: repo, publish: pub}
}

func (svc *BeadService) requireCLI() error {
	if svc.cli == nil {
		return &ServiceError{Code: CodeCLIMissing, Message: "bd CLI not available"}
	}
	return nil
}

// ListBeads returns all beads, optionally filtered by column.
func (svc *BeadService) ListBeads(ctx context.Context, column string) ([]core.Bead, error) {
	f := store.Filter{TruncateDesc: 2048}
	if column != "" {
		f.Status = columnToStatuses(column)
	}
	issues, err := svc.backend.List(ctx, f)
	if err != nil {
		return nil, &ServiceError{Code: CodeInternal, Message: err.Error()}
	}
	return IssueToBeads(issues, svc.repo), nil
}

// GetBead returns the bead with the given ID.
func (svc *BeadService) GetBead(ctx context.Context, id string) (*core.Bead, error) {
	issue, err := svc.backend.Get(ctx, id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, &ServiceError{Code: CodeNotFound, Message: "bead not found"}
		}
		return nil, &ServiceError{Code: CodeInternal, Message: err.Error()}
	}
	b := IssueToBead(issue, svc.repo)
	return &b, nil
}

// Create validates input and creates a bead via the CLI.
func (svc *BeadService) Create(ctx context.Context, input CreateBeadInput) (*core.Bead, error) {
	title := strings.TrimSpace(input.Title)
	if title == "" {
		return nil, &ServiceError{Code: CodeInvalidRequest, Message: "title is required"}
	}
	if len([]rune(title)) > 255 {
		return nil, &ServiceError{Code: CodeInvalidRequest, Message: "title exceeds 255 chars"}
	}
	if input.Type != "" && !input.Type.Valid() {
		return nil, &ServiceError{Code: CodeInvalidRequest, Message: "invalid type"}
	}
	if !input.Priority.Valid() {
		return nil, &ServiceError{Code: CodeInvalidRequest, Message: "invalid priority"}
	}
	if err := svc.requireCLI(); err != nil {
		return nil, err
	}
	// TODO T067: implement via bdshell CLI
	return nil, &ServiceError{Code: CodeInternal, Message: "bd CLI integration not yet implemented"}
}

// Patch validates and applies a partial update via the CLI.
func (svc *BeadService) Patch(ctx context.Context, id string, input PatchBeadInput) (*core.Bead, error) {
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
	}
	if err := svc.requireCLI(); err != nil {
		return nil, err
	}
	// TODO T066: implement via bdshell CLI
	return nil, &ServiceError{Code: CodeInternal, Message: "bd CLI integration not yet implemented"}
}

// Move places a bead in a target column via the CLI.
func (svc *BeadService) Move(ctx context.Context, id string, input MoveInput) (*core.Bead, error) {
	if !input.ToColumn.Valid() {
		return nil, &ServiceError{Code: CodeInvalidRequest, Message: "invalid toColumn"}
	}
	if err := svc.requireCLI(); err != nil {
		return nil, err
	}
	// TODO T068: implement via bdshell CLI
	return nil, &ServiceError{Code: CodeInternal, Message: "bd CLI integration not yet implemented"}
}

// Dispatch transitions a bead to running via the CLI.
func (svc *BeadService) Dispatch(ctx context.Context, id string, input DispatchInput) (*core.Bead, error) {
	if !input.Agent.Valid() {
		return nil, &ServiceError{Code: CodeInvalidRequest, Message: "invalid agent"}
	}
	if !input.Mode.Valid() {
		return nil, &ServiceError{Code: CodeInvalidRequest, Message: "invalid mode"}
	}
	if err := svc.requireCLI(); err != nil {
		return nil, err
	}
	// TODO T069: implement via bdshell CLI
	return nil, &ServiceError{Code: CodeInternal, Message: "bd CLI integration not yet implemented"}
}

// AddComment appends a comment via the CLI.
func (svc *BeadService) AddComment(ctx context.Context, id string, input CommentInput) (*core.Bead, error) {
	actor := strings.TrimSpace(input.Actor)
	if actor == "" {
		return nil, &ServiceError{Code: CodeInvalidRequest, Message: "actor is required"}
	}
	note := strings.TrimSpace(input.Note)
	if note == "" {
		return nil, &ServiceError{Code: CodeInvalidRequest, Message: "note is required"}
	}
	if err := svc.requireCLI(); err != nil {
		return nil, err
	}
	// TODO T070: implement via bdshell CLI
	return nil, &ServiceError{Code: CodeInternal, Message: "bd CLI integration not yet implemented"}
}
