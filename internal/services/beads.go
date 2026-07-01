package services

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
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

	// M2 dispatch codes.
	CodeRunAlreadyActive    = "RUN_ALREADY_ACTIVE"    // 409
	CodeUnmappedPrefix      = "UNMAPPED_PREFIX"       // 422
	CodeAdapterNotFound     = "ADAPTER_NOT_FOUND"     // 501
	CodeAdapterNotInstalled = "ADAPTER_NOT_INSTALLED" // 501
	CodeAdapterNotLoggedIn  = "ADAPTER_NOT_LOGGED_IN" // 409 (need to run `claude auth login`)
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
// The real implementation is *orchestrator.Orchestrator, wrapped by
// Orchestrator.AsServiceDispatcher() which calls mapDispatchError to translate
// orchestrator sentinel errors into typed *ServiceError values before they
// cross the service boundary; tests may substitute a fake.
// This interface is defined here (not in orchestrator) to avoid import cycles.
// The Dispatch method accepts an OrchestratorDispatchRequest, which mirrors
// orchestrator.DispatchRequest — kept in sync by the API layer.
type OrchestratorDispatcher interface {
	// Dispatch launches an agent run for the given bead, returning the active bead.
	// Implementations return *ServiceError (with Code set per
	// orchestrator.mapDispatchError) rather than raw orchestrator sentinels;
	// anything else is treated as CodeInternal by wrapOrchestratorError.
	Dispatch(ctx context.Context, req OrchestratorDispatchRequest) (*core.Bead, error)
}

// OrchestratorDispatchRequest is the input for OrchestratorDispatcher.Dispatch.
// Mirrors orchestrator.DispatchRequest; defined here to avoid an import cycle.
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
	cli          CLIRunner              // may be nil; writes return CodeCLIMissing when nil
	orchestrator OrchestratorDispatcher // may be nil; dispatch returns CodeCLIMissing when nil
	attacher     SessionAttacher        // may be nil; attach/send return unavailable when nil
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

// WithAttacher returns a new BeadService with a session attacher attached.
// The attacher is used by GetAttach/SendKeys when present.
func (svc *BeadService) WithAttacher(a SessionAttacher) *BeadService {
	svc2 := *svc
	svc2.attacher = a
	return &svc2
}

// publishWrite broadcasts a write event when publish-on-write is enabled.
func (svc *BeadService) publishWrite(eventType ws.EventType, bead *core.Bead) {
	if !svc.publishOnWrite || svc.publish == nil {
		return
	}
	svc.publish(ws.Frame{Type: eventType, Bead: bead})
}

// publishForce broadcasts a write event unconditionally (ignoring
// publishOnWrite). Used when a real bd write did not happen — and so the
// embedded-mode file watcher won't fan a jsonl change into the hub either —
// but other connected clients still need to learn about a state transition.
func (svc *BeadService) publishForce(eventType ws.EventType, bead *core.Bead) {
	if svc.publish == nil {
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

// wrapOrchestratorError normalizes an error returned by the OrchestratorDispatcher
// into a *ServiceError. Orchestrator sentinel→code mapping happens on the
// orchestrator side (orchestrator.mapDispatchError, via errors.Is) and reaches
// here already typed; anything else is an internal error. No message-string
// matching — that was brittle (a message tweak would silently mis-map codes).
func wrapOrchestratorError(err error) *ServiceError {
	if err == nil {
		return nil
	}
	var se *ServiceError
	if errors.As(err, &se) {
		return se
	}
	// Unrecognized orchestrator errors (e.g. wrapped worktree/git, tmux spawn,
	// or adapter invoke failures) can carry filesystem paths or subprocess
	// stderr. Log the detail server-side and return a generic message to the
	// client, mirroring wrapCLIError's generic fallback for unmatched exit
	// codes below.
	slog.Error("orchestrator dispatch: internal error", "err", err)
	return &ServiceError{Code: CodeInternal, Message: "dispatch failed due to an internal error"}
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
// When an orchestrator is wired in, it delegates to the orchestrator to
// launch a real agent run, then persists the running transition via the same
// bd CLI claim the legacy path uses (best-effort: a failure here is logged,
// not fatal, since the run is already active) — beads is the source of truth
// for issue state, so the launch alone is not enough. Otherwise it falls back
// to the legacy bd-CLI path (stub).
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
		result, orchErr := svc.orchestrator.Dispatch(ctx, req)
		if orchErr != nil {
			return nil, wrapOrchestratorError(orchErr)
		}

		// Persist the running transition. Beads is the source of truth for
		// issue state (constitution II; FR-002 requires dispatch to
		// "transition the bead to a running state") — the orchestrator's own
		// result is an in-memory stub scoped to its run registry, not a
		// store write. Without this call, a subsequent GET (or a
		// reconnecting client re-reading the store) would keep showing the
		// pre-dispatch column even though the agent is genuinely running.
		// This is the same bd-CLI claim the legacy path below already uses,
		// and its write is what the rest of the notification pipeline
		// already assumes happens: in embedded mode the file watcher fans
		// the resulting jsonl change into the WS hub (no watcher runs in
		// remote mode, which is why publishWrite below exists).
		//
		// The agent has already launched by this point (orchestrator.Dispatch
		// succeeded above), so a failure here is logged, not fatal: failing
		// the whole request would be misleading (the run IS active) and a
		// client retry would just hit ErrRunAlreadyActive/409.
		persisted := false
		if svc.cli != nil {
			if iss, err := svc.cli.Dispatch(ctx, id); err != nil {
				slog.Warn("Dispatch: orchestrator run started but persisting the running state failed; bead state may lag until the run completes", "bead", id, "err", err)
			} else {
				b := IssueToBead(&iss, svc.repo)
				bead = &b
				persisted = true
			}
		} else {
			slog.Warn("Dispatch: bd CLI unavailable; cannot persist running state for orchestrator-backed run", "bead", id)
		}

		// Parity with the legacy bd-CLI dispatch path below: publish a
		// bead.updated frame so other connected clients learn about the
		// running transition. In remote mode there is no file watcher to fan
		// a jsonl write into the hub, so this manual publish is the only
		// signal beyond tmux.session.opened. Overlay the orchestrator's
		// column so the frame carries a complete payload even if the bd
		// claim above failed/was unavailable (result.Column is still
		// authoritative for "the run is active" either way).
		// Return the same merged bead from the HTTP path so the response
		// matches the WS frame (clients otherwise saw a stub here while a
		// concurrent socket-listener saw the full bead).
		if result != nil {
			merged := *bead
			merged.Column = result.Column
			if svc.publishOnWrite || !persisted {
				// svc.publishOnWrite: remote mode, no watcher runs at all, so
				// this manual publish is the only signal regardless of
				// persist outcome.
				// !persisted: embedded mode, but no real bd write happened
				// (cli nil or claim failed) — the file watcher has nothing
				// to fan into the hub, so publishWrite's normal no-op gate
				// would otherwise leave every other connected client
				// without a running-transition signal at all.
				svc.publishForce(ws.EventBeadUpdated, &merged)
			}
			return &merged, nil
		}
		return result, nil
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

// AttachResponse describes a live tmux session for user attachment.
type AttachResponse struct {
	Available bool   `json:"available"`
	Command   string `json:"command,omitempty"`
	Session   string `json:"session,omitempty"`
	Pane      string `json:"pane,omitempty"`
	Reason    string `json:"reason,omitempty"`
}

// SessionAttacher is the interface BeadService uses to get attach info.
// Implemented by the orchestrator.
type SessionAttacher interface {
	GetAttach(beadID string, stepIdx int) (*AttachResponse, error)
	SendKeys(beadID string, stepIdx int, keys string) error
}

// GetAttach returns attachment info for a running step.
// Delegates to the orchestrator if available.
func (svc *BeadService) GetAttach(ctx context.Context, beadID string, stepIdx int) (*AttachResponse, error) {
	// Validate bead exists.
	_, err := svc.GetBead(ctx, beadID)
	if err != nil {
		return nil, err
	}
	if svc.attacher != nil {
		return svc.attacher.GetAttach(beadID, stepIdx)
	}
	// No orchestrator: return available:false.
	return &AttachResponse{Available: false, Reason: "attach not available"}, nil
}

// SendKeys forwards keystrokes to a running step's tmux pane.
func (svc *BeadService) SendKeys(ctx context.Context, beadID string, stepIdx int, keys string) error {
	_, err := svc.GetBead(ctx, beadID)
	if err != nil {
		return err
	}
	if svc.attacher != nil {
		return svc.attacher.SendKeys(beadID, stepIdx, keys)
	}
	// No attacher wired in: this isn't a transient backend outage (which is
	// what CodeCLIUnavailable/503 signals to clients) — the feature is simply
	// not configured, matching the CodeCLIMissing/501 the legacy bd-CLI paths
	// use when svc.cli is nil.
	return &ServiceError{Code: CodeCLIMissing, Message: "send not available"}
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
