package services

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/gitrgoliveira/muster/internal/core"
	"github.com/gitrgoliveira/muster/internal/store"
	"github.com/gitrgoliveira/muster/internal/store/bdshell"
	"github.com/gitrgoliveira/muster/internal/ws"
)

// CLIRunner abstracts bd CLI write operations.
type CLIRunner interface {
	Create(ctx context.Context, in bdshell.CreateInput) (store.Issue, error)
	Update(ctx context.Context, id string, p bdshell.UpdatePatch) (store.Issue, error)
	Close(ctx context.Context, id string) (store.Issue, error)
	Dispatch(ctx context.Context, id string) (store.Issue, error)
	AppendNote(ctx context.Context, id, text string) (store.Issue, error)
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
	Assignee     string
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
	Assignee     *string
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
	Agent          core.AgentID
	Mode           core.Mode
	PermissionMode core.PermissionMode // NEW (FR-021); validated/allow-listed
}

// CommentInput carries the validated comment parameters.
type CommentInput struct {
	Actor string
	Note  string
}

// OrchestratorDispatcher is the interface BeadService uses to launch agent runs.
// The real implementation is *orchestrator.Orchestrator; tests may substitute a fake.
// This interface is defined here (not in orchestrator) to avoid import cycles.
type OrchestratorDispatcher interface {
	// Dispatch launches an agent run for the given bead, returning the active bead.
	// Implementations return ErrRunAlreadyActive (maps to 409), ErrUnmappedPrefix
	// (maps to 422), or ErrAdapterNotFound / ErrAdapterNotInstalled (maps to 501/422).
	Dispatch(ctx context.Context, req OrchestratorDispatchRequest) (*core.Bead, error)
}

// OrchestratorDispatchRequest is the input for OrchestratorDispatcher.Dispatch.
type OrchestratorDispatchRequest struct {
	BeadID         string
	BeadTitle      string
	BeadDesc       string
	Agent          core.AgentID
	Mode           core.Mode
	PermissionMode core.PermissionMode
}

// BeadService implements business logic on top of the store.
type BeadService struct {
	backend      store.Backend
	cli          CLIRunner            // may be nil; writes return CodeCLIMissing when nil
	orchestrator OrchestratorDispatcher // may be nil; dispatch returns CodeCLIMissing when nil
	repo         string
	publish      Publisher
	// publishOnWrite broadcasts a WS frame after each successful CLI write.
	// Enabled in remote mode (no file watcher); embedded mode leaves it false
	// so the watcher remains the single WS source and writes aren't double-announced.
	publishOnWrite bool
}

// NewBeadService constructs a BeadService.
// cli may be nil; write operations return CodeCLIMissing in that case.
func NewBeadService(backend store.Backend, cli CLIRunner, pub Publisher) *BeadService {
	return &BeadService{backend: backend, cli: cli, publish: pub}
}

// NewBeadServiceWithRepo constructs a BeadService with an explicit repo name.
// publishOnWrite should be true in remote mode (where no watcher runs).
func NewBeadServiceWithRepo(backend store.Backend, cli CLIRunner, pub Publisher, repo string, publishOnWrite bool) *BeadService {
	return &BeadService{backend: backend, cli: cli, repo: repo, publish: pub, publishOnWrite: publishOnWrite}
}

// WithOrchestrator returns a new BeadService with an orchestrator dispatcher
// attached. The orchestrator is used by Dispatch when present.
func (svc *BeadService) WithOrchestrator(o OrchestratorDispatcher) *BeadService {
	svc2 := *svc
	svc2.orchestrator = o
	return &svc2
}

// publishWrite broadcasts a write event when publish-on-write is enabled.
func (svc *BeadService) publishWrite(eventType ws.EventType, bead *core.Bead) {
	if !svc.publishOnWrite || svc.publish == nil {
		return
	}
	svc.publish(ws.Frame{Type: eventType, Bead: bead})
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
	return &ServiceError{Code: CodeInternal, Message: "bd CLI failed"}
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
		return nil, &ServiceError{Code: CodeInternal, Message: "failed to list beads"}
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
		return nil, &ServiceError{Code: CodeInternal, Message: "failed to get bead"}
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
	if utf8.RuneCountInString(title) > 255 {
		return nil, &ServiceError{Code: CodeInvalidRequest, Message: "title exceeds 255 chars"}
	}
	if err := validateField("title", title); err != nil {
		return nil, err
	}
	if err := validateField("desc", input.Desc); err != nil {
		return nil, err
	}
	if err := validateField("assignee", input.Assignee); err != nil {
		return nil, err
	}
	if len(input.Labels) > 0 {
		return nil, &ServiceError{Code: CodeInvalidRequest, Message: "labels not supported by bd CLI"}
	}
	if input.Column != "" {
		return nil, &ServiceError{Code: CodeInvalidRequest, Message: "column not supported on create (beads default to backlog)"}
	}
	if input.VCS != "" {
		return nil, &ServiceError{Code: CodeInvalidRequest, Message: "vcs not supported by bd CLI"}
	}
	if input.TokensBudget != 0 {
		return nil, &ServiceError{Code: CodeInvalidRequest, Message: "tokensBudget not supported by bd CLI"}
	}
	if input.Type == "" {
		return nil, &ServiceError{Code: CodeInvalidRequest, Message: "type is required"}
	}
	if !input.Type.Valid() {
		return nil, &ServiceError{Code: CodeInvalidRequest, Message: "invalid type"}
	}
	if !input.Priority.Valid() {
		return nil, &ServiceError{Code: CodeInvalidRequest, Message: "invalid priority"}
	}
	if err := svc.requireCLI(); err != nil {
		return nil, err
	}

	p := int(input.Priority)
	iss, err := svc.cli.Create(ctx, bdshell.CreateInput{
		Title:       title,
		Description: input.Desc,
		Type:        string(input.Type),
		Priority:    &p,
		Assignee:    input.Assignee,
	})
	if err != nil {
		return nil, wrapCLIError(err)
	}
	b := IssueToBead(&iss, svc.repo)
	svc.publishWrite(ws.EventBeadCreated, &b)
	return &b, nil
}

// Patch validates and applies a partial update via the CLI.
func (svc *BeadService) Patch(ctx context.Context, id string, input PatchBeadInput) (*core.Bead, error) {
	if input.Title == nil && input.Desc == nil && input.Type == nil &&
		input.Column == nil && input.Priority == nil && input.Assignee == nil &&
		input.Labels == nil && input.Ready == nil && input.TokensBudget == nil {
		return nil, &ServiceError{Code: CodeInvalidRequest, Message: "patch body must contain at least one field"}
	}
	if input.Labels != nil {
		return nil, &ServiceError{Code: CodeInvalidRequest, Message: "labels patch not supported by bd CLI"}
	}
	if input.Ready != nil {
		return nil, &ServiceError{Code: CodeInvalidRequest, Message: "ready patch not supported by bd CLI"}
	}
	if input.TokensBudget != nil {
		return nil, &ServiceError{Code: CodeInvalidRequest, Message: "tokensBudget patch not supported by bd CLI"}
	}
	if input.Type != nil && !input.Type.Valid() {
		return nil, &ServiceError{Code: CodeInvalidRequest, Message: "invalid type"}
	}
	if input.Column != nil && !input.Column.Valid() {
		return nil, &ServiceError{Code: CodeInvalidRequest, Message: "invalid column"}
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
		input.Title = &t
	}
	if input.Desc != nil {
		if err := validateField("desc", *input.Desc); err != nil {
			return nil, err
		}
	}
	if input.Assignee != nil {
		if err := validateField("assignee", *input.Assignee); err != nil {
			return nil, err
		}
	}
	if err := svc.requireCLI(); err != nil {
		return nil, err
	}

	patch := bdshell.UpdatePatch{}
	if input.Title != nil {
		patch.Title = input.Title
	}
	if input.Desc != nil {
		patch.Description = input.Desc
	}
	if input.Type != nil {
		t := string(*input.Type)
		patch.Type = &t
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
	if input.Assignee != nil {
		patch.Assignee = input.Assignee
	}

	iss, err := svc.cli.Update(ctx, id, patch)
	if err != nil {
		return nil, wrapCLIError(err)
	}
	b := IssueToBead(&iss, svc.repo)
	svc.publishWrite(ws.EventBeadUpdated, &b)
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
		// bd close --json returns the closed issue directly, so we use it
		// rather than re-reading the backend (which may still serve the
		// pre-close JSONL cache before bd's export lands).
		iss, err = svc.cli.Close(ctx, id)
		if err != nil {
			return nil, wrapCLIError(err)
		}
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
	svc.publishWrite(ws.EventBeadUpdated, &b)
	return &b, nil
}

// Dispatch transitions a bead to running.
// When an orchestrator is wired in, it delegates to the orchestrator to launch
// a real agent run; otherwise it falls back to the legacy bd-CLI path (stub).
func (svc *BeadService) Dispatch(ctx context.Context, id string, input DispatchInput) (*core.Bead, error) {
	if !input.Agent.Valid() {
		return nil, &ServiceError{Code: CodeInvalidRequest, Message: "invalid agent"}
	}
	if !input.Mode.Valid() {
		return nil, &ServiceError{Code: CodeInvalidRequest, Message: "invalid mode"}
	}
	if input.PermissionMode != "" && !input.PermissionMode.Valid() {
		return nil, &ServiceError{Code: CodeInvalidRequest, Message: "invalid permissionMode"}
	}

	// If an orchestrator is available, delegate real dispatch to it.
	if svc.orchestrator != nil {
		// Get the bead first to obtain title/desc for the prompt.
		bead, err := svc.GetBead(ctx, id)
		if err != nil {
			return nil, err
		}
		req := OrchestratorDispatchRequest{
			BeadID:         id,
			BeadTitle:      bead.Title,
			BeadDesc:       bead.Desc,
			Agent:          input.Agent,
			Mode:           input.Mode,
			PermissionMode: input.PermissionMode,
		}
		return svc.orchestrator.Dispatch(ctx, req)
	}

	// Legacy stub path: just move the bead to running via bd CLI.
	if err := svc.requireCLI(); err != nil {
		return nil, err
	}

	iss, err := svc.cli.Dispatch(ctx, id)
	if err != nil {
		return nil, wrapCLIError(err)
	}
	b := IssueToBead(&iss, svc.repo)
	svc.publishWrite(ws.EventBeadUpdated, &b)
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
	svc.publishWrite(ws.EventBeadUpdated, &b)
	return &b, nil
}
