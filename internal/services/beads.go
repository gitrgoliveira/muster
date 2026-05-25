package services

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/gitrgoliveira/muster/internal/core"
	"github.com/gitrgoliveira/muster/internal/store"
	"github.com/gitrgoliveira/muster/internal/store/bdshell"
)

// CLIRunner abstracts bd CLI write operations.
type CLIRunner interface {
	Create(ctx context.Context, in bdshell.CreateInput) (store.Issue, error)
	Update(ctx context.Context, id string, p bdshell.UpdatePatch) (store.Issue, error)
	Close(ctx context.Context, id string) error
	Dispatch(ctx context.Context, id string) (store.Issue, error)
	AppendNote(ctx context.Context, id, text string) (store.Issue, error)
	RunJSON(ctx context.Context, dst any, args ...string) error
	RunVoid(ctx context.Context, args ...string) error
}

const (
	CodeInvalidRequest = "INVALID_REQUEST"
	CodeInvalidState   = "INVALID_STATE"
	CodeNotFound       = "BEAD_NOT_FOUND"
	CodeInternal       = "INTERNAL"
	CodeCLIMissing     = "BD_CLI_MISSING"
	CodeCLIValidation  = "BD_CLI_VALIDATION"
	CodeCLIUnavailable = "BD_CLI_UNAVAILABLE"
	CodeCLITimeout     = "BD_CLI_TIMEOUT"
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

// wrapCLIError maps *bdshell.CLIError exit codes to service error codes.
func wrapCLIError(err error) *ServiceError {
	var ce *bdshell.CLIError
	if errors.As(err, &ce) {
		switch ce.ExitCode {
		case 1:
			return &ServiceError{Code: CodeCLIValidation, Message: ce.Stderr}
		case 2:
			return &ServiceError{Code: CodeNotFound, Message: ce.Stderr}
		case 3:
			return &ServiceError{Code: CodeCLIUnavailable, Message: ce.Stderr}
		default:
			return &ServiceError{Code: CodeInternal, Message: ce.Stderr}
		}
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return &ServiceError{Code: CodeCLITimeout, Message: "bd CLI timed out"}
	}
	return &ServiceError{Code: CodeInternal, Message: err.Error()}
}

// validateField rejects fields containing control characters (NUL, CR, LF) or exceeding 64 KB.
// LF/CR are rejected because bd may parse multi-line input unexpectedly.
func validateField(name, value string) error {
	if strings.ContainsRune(value, '\x00') {
		return &ServiceError{Code: CodeInvalidRequest, Message: name + " contains invalid NUL byte"}
	}
	if strings.ContainsAny(value, "\n\r") {
		return &ServiceError{Code: CodeInvalidRequest, Message: name + " contains newline"}
	}
	if len(value) > 64*1024 {
		return &ServiceError{Code: CodeInvalidRequest, Message: name + " exceeds 64 KB limit"}
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
	if err := validateField("title", title); err != nil {
		return nil, err
	}
	if err := validateField("desc", input.Desc); err != nil {
		return nil, err
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

	iss, err := svc.cli.Create(ctx, bdshell.CreateInput{
		Title:       title,
		Description: input.Desc,
		Type:        string(input.Type),
		Priority:    int(input.Priority),
	})
	if err != nil {
		return nil, wrapCLIError(err)
	}
	b := IssueToBead(&iss, svc.repo)
	return &b, nil
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
		if err := validateField("title", t); err != nil {
			return nil, err
		}
	}
	if input.Desc != nil {
		if err := validateField("desc", *input.Desc); err != nil {
			return nil, err
		}
	}
	if err := svc.requireCLI(); err != nil {
		return nil, err
	}

	patch := bdshell.UpdatePatch{}
	if input.Title != nil {
		t := strings.TrimSpace(*input.Title)
		patch.Title = &t
	}
	if input.Desc != nil {
		patch.Description = input.Desc
	}
	if input.Column != nil {
		statuses := columnToStatuses(string(*input.Column))
		if len(statuses) > 0 {
			patch.Status = &statuses[0]
		}
	}
	if input.Priority != nil {
		p := int(*input.Priority)
		patch.Priority = &p
	}

	iss, err := svc.cli.Update(ctx, id, patch)
	if err != nil {
		return nil, wrapCLIError(err)
	}
	b := IssueToBead(&iss, svc.repo)
	return &b, nil
}

// Move places a bead in a target column via the CLI.
func (svc *BeadService) Move(ctx context.Context, id string, input MoveInput) (*core.Bead, error) {
	if !input.ToColumn.Valid() {
		return nil, &ServiceError{Code: CodeInvalidRequest, Message: "invalid toColumn"}
	}
	if err := svc.requireCLI(); err != nil {
		return nil, err
	}

	var iss store.Issue
	var err error

	switch input.ToColumn {
	case core.ColDone:
		if cerr := svc.cli.Close(ctx, id); cerr != nil {
			return nil, wrapCLIError(cerr)
		}
		// Re-read the issue after close.
		issue, rerr := svc.backend.Get(ctx, id)
		if rerr != nil {
			return nil, &ServiceError{Code: CodeInternal, Message: rerr.Error()}
		}
		iss = *issue
	case core.ColRunning:
		iss, err = svc.cli.Update(ctx, id, bdshell.UpdatePatch{Claim: true})
		if err != nil {
			return nil, wrapCLIError(err)
		}
	default:
		statuses := columnToStatuses(string(input.ToColumn))
		if len(statuses) == 0 {
			return nil, &ServiceError{Code: CodeInvalidRequest, Message: "unknown column"}
		}
		iss, err = svc.cli.Update(ctx, id, bdshell.UpdatePatch{Status: &statuses[0]})
		if err != nil {
			return nil, wrapCLIError(err)
		}
	}

	b := IssueToBead(&iss, svc.repo)
	return &b, nil
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

	iss, err := svc.cli.Dispatch(ctx, id)
	if err != nil {
		return nil, wrapCLIError(err)
	}
	b := IssueToBead(&iss, svc.repo)
	return &b, nil
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
	if err := validateField("actor", actor); err != nil {
		return nil, err
	}
	if err := validateField("note", note); err != nil {
		return nil, err
	}
	if err := svc.requireCLI(); err != nil {
		return nil, err
	}

	// Prepend actor to note text since bd v1.0 has no --actor flag.
	fullNote := actor + ": " + note
	iss, err := svc.cli.AppendNote(ctx, id, fullNote)
	if err != nil {
		return nil, wrapCLIError(err)
	}
	b := IssueToBead(&iss, svc.repo)
	return &b, nil
}
