package services

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os/exec"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/gitrgoliveira/muster/internal/core"
	"github.com/gitrgoliveira/muster/internal/store"
	"github.com/gitrgoliveira/muster/internal/store/bdshell"
	"github.com/gitrgoliveira/muster/internal/ws"
	"github.com/gitrgoliveira/muster/internal/wt"
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
	CodeAttachUnavailable   = "ATTACH_UNAVAILABLE"    // 501 (attach/send feature not configured)

	// M3 worktree codes.
	CodeWorktreeNotFound = "WORKTREE_NOT_FOUND" // 404 — bead exists but has no worktree
	CodeVCSUnavailable   = "VCS_UNAVAILABLE"    // 412 — VCS binary absent or wrong VCS type
	CodeWorktreeDirty    = "WORKTREE_DIRTY"     // 409 — worktree has uncommitted changes; finalize first

	// M4 dispatcher codes (additive).
	CodeStepOutOfRange  = "STEP_OUT_OF_RANGE" // 422 — step index outside [0, chainLen)
	CodeInvalidCapacity = "INVALID_CAPACITY"  // 400 — scheduler capacity ≤0
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
	// Chain is an optional per-dispatch step-chain override (contract
	// http-endpoints.md "chain" field). Nil/empty means the M2 single-step
	// default (or the orchestrator's configured default chain, if any).
	Chain []ChainStepInput
}

// ChainStepInput is a wire-agnostic mirror of orchestrator.StepProfile.
// Defined here (not in orchestrator) to avoid an import cycle — same reason
// OrchestratorDispatchRequest below is defined in services rather than
// referencing orchestrator.DispatchRequest directly.
type ChainStepInput struct {
	Name           string
	PermissionMode core.PermissionMode
	PromptRef      string
}

// DispatchResult is the return value of BeadService.Dispatch.
// It carries the active bead plus idempotency flags surfaced to the HTTP layer.
type DispatchResult struct {
	Bead   *core.Bead
	Joined bool // true when joining an existing in-flight run (M4 US4)
	Queued bool // true when the run is waiting for a capacity slot (M4 US1)
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
	// Dispatch launches an agent run for the given bead. On success it returns
	// an OrchestratorDispatchResult containing the active bead plus idempotency
	// flags (Joined, Queued). Implementations return *ServiceError (with Code
	// set per orchestrator.mapDispatchError) rather than raw orchestrator
	// sentinels; anything else is treated as CodeInternal by
	// wrapOrchestratorError.
	Dispatch(ctx context.Context, req OrchestratorDispatchRequest) (OrchestratorDispatchResult, error)
}

// labelReader is the optional capability (satisfied by *bdshell.CLI) to read a
// bead's labels. It is intentionally NOT part of CLIRunner, so existing fakes
// are unaffected; a CLI without it simply yields no label-derived skills.
type labelReader interface {
	Labels(ctx context.Context, id string) ([]string, error)
}

// resolveBeadSkills computes the effective skill loadout for a dispatch: the
// bead's reserved skill:<id> labels (read via bd, best-effort) unioned with any
// skills already on the bead, de-duplicated. A label-read failure is
// non-blocking — it yields no label-derived skills rather than failing dispatch.
func (svc *BeadService) resolveBeadSkills(ctx context.Context, id string, bead *core.Bead) []string {
	var ids []string
	seen := map[string]bool{}
	add := func(s string) {
		if !seen[s] {
			seen[s] = true
			ids = append(ids, s)
		}
	}
	if lr, ok := svc.cli.(labelReader); ok {
		if labels, err := lr.Labels(ctx, id); err == nil {
			// A bare/malformed skill id (e.g. from a "skill:" label) is kept so
			// assembly warns on it (FR-020) rather than dropping it silently.
			skillIDs, _ := SplitSkillLabels(labels)
			for _, s := range skillIDs {
				add(s)
			}
		}
	}
	if bead != nil {
		for _, s := range bead.Skills {
			if s != "" {
				add(s)
			}
		}
	}
	return ids
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
	// Chain mirrors orchestrator.DispatchRequest.Chain (via ChainStepInput /
	// orchestrator.StepProfile); nil/empty means no per-dispatch override.
	Chain []ChainStepInput

	// Skills mirrors orchestrator.DispatchRequest.Skills — the effective skill
	// loadout for this dispatch (from the bead's skill:<id> labels ∪ any
	// step-level selection). Unknown ids warn at assembly, never block (M6 US4).
	Skills []string
}

// OrchestratorDispatchResult is the service-layer mirror of
// orchestrator.DispatchResult; defined here (not in orchestrator) to avoid an
// import cycle. Kept in sync with orchestrator.DispatchResult per the
// OrchestratorDispatchRequest mirroring pattern.
type OrchestratorDispatchResult struct {
	Bead   *core.Bead
	Joined bool // true when joining an in-flight run (idempotent dispatch, M4 US4)
	Queued bool // true when admitted to the waiting queue (capacity full, M4 US1)
}

// SchedulerSnapshot is the service-layer view of the scheduler's current state.
// It mirrors orchestrator.SchedulerSnapshot; defined here to avoid import cycles.
type SchedulerSnapshot struct {
	Capacity    int
	ActiveCount int
	Waiting     []string // bead IDs in FIFO order
}

// SchedulerManager is the interface BeadService uses to manage scheduler capacity.
// The real implementation is *orchestrator.Orchestrator; tests substitute a fake.
// Defined here (not in orchestrator) to avoid import cycles.
type SchedulerManager interface {
	// SetCapacity changes the scheduler's maximum concurrency at runtime.
	// n must be > 0; returns an error (code INVALID_CAPACITY) otherwise.
	SetCapacity(n int) error
	// SchedulerSnapshot returns the current scheduler state (capacity, active, waiting).
	SchedulerSnapshot() SchedulerSnapshot
}

// BeadService implements business logic on top of the store.
type BeadService struct {
	backend      store.Backend
	cli          CLIRunner              // may be nil; writes return CodeCLIMissing when nil
	orchestrator OrchestratorDispatcher // may be nil; dispatch returns CodeCLIMissing when nil
	scheduler    SchedulerManager       // may be nil; SetCapacity/SchedulerSnapshot return error when nil
	attacher     SessionAttacher        // may be nil; attach/send return unavailable when nil
	wtAccessor   WorktreeAccessor       // may be nil; worktree/diff return CodeVCSUnavailable when nil
	stepAdvancer StepAdvancer           // may be nil; advance/loopback return unavailable when nil
	repo         string
	publish      Publisher
	// publishOnWrite broadcasts a WS frame after each successful CLI write.
	// Enabled in remote mode (no file watcher); embedded mode leaves it false
	// so the watcher remains the single WS source and writes aren't double-announced.
	publishOnWrite bool
	// wtOpMu serializes mutating worktree operations (Finalize, Remove) per bead.
	// Push is excluded because it is read-side from the worktree's perspective and
	// is explicitly permitted at any run state.
	// Key: beadID (string) → *sync.Mutex.
	// A pointer so the With* builder copies share the same lock map (they are the same logical service).
	wtOpMu *sync.Map
}

// NewBeadService constructs a BeadService.
// cli may be nil; write operations return CodeCLIMissing in that case.
func NewBeadService(backend store.Backend, cli CLIRunner, pub Publisher) *BeadService {
	return &BeadService{backend: backend, cli: cli, publish: pub, wtOpMu: &sync.Map{}}
}

// NewBeadServiceWithRepo constructs a BeadService with an explicit repo name.
// publishOnWrite should be true in remote mode (where no watcher runs).
func NewBeadServiceWithRepo(backend store.Backend, cli CLIRunner, pub Publisher, repo string, publishOnWrite bool) *BeadService {
	return &BeadService{backend: backend, cli: cli, repo: repo, publish: pub, publishOnWrite: publishOnWrite, wtOpMu: &sync.Map{}}
}

// WithOrchestrator returns a new BeadService with an orchestrator dispatcher
// attached. The orchestrator is used by Dispatch when present.
func (svc *BeadService) WithOrchestrator(o OrchestratorDispatcher) *BeadService {
	svc2 := *svc
	svc2.orchestrator = o
	return &svc2
}

// WithScheduler returns a new BeadService with a scheduler manager attached.
// The manager is used by SetCapacity and SchedulerSnapshot when present.
func (svc *BeadService) WithScheduler(s SchedulerManager) *BeadService {
	svc2 := *svc
	svc2.scheduler = s
	return &svc2
}

// SetCapacity changes the scheduler's maximum concurrency at runtime.
// Returns *ServiceError{Code: CodeInvalidCapacity} when n ≤ 0, or when the
// scheduler is not configured (scheduler == nil).
func (svc *BeadService) SetCapacity(n int) error {
	if svc.scheduler == nil {
		return &ServiceError{Code: CodeInvalidCapacity, Message: "scheduler not configured"}
	}
	if err := svc.scheduler.SetCapacity(n); err != nil {
		// If the adapter already returned a *ServiceError, pass it through
		// without re-wrapping (guards against double-wrap from the orchestrator
		// adapter path — tri-review fix #12).
		var se *ServiceError
		if errors.As(err, &se) {
			return se
		}
		return &ServiceError{Code: CodeInvalidCapacity, Message: err.Error()}
	}
	return nil
}

// SchedulerSnapshot returns the current scheduler state. If the scheduler is
// not configured, it returns a zero-value snapshot (capacity 0, no active runs).
func (svc *BeadService) SchedulerSnapshot() SchedulerSnapshot {
	if svc.scheduler == nil {
		return SchedulerSnapshot{}
	}
	return svc.scheduler.SchedulerSnapshot()
}

// WithAttacher returns a new BeadService with a session attacher attached.
// The attacher is used by GetAttach/SendKeys when present.
func (svc *BeadService) WithAttacher(a SessionAttacher) *BeadService {
	svc2 := *svc
	svc2.attacher = a
	return &svc2
}

// WithWorktreeAccessor returns a new BeadService with a worktree accessor
// attached. The accessor is used by Worktree and Diff when present.
func (svc *BeadService) WithWorktreeAccessor(a WorktreeAccessor) *BeadService {
	svc2 := *svc
	svc2.wtAccessor = a
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
//
// Returns a DispatchResult carrying the active bead plus idempotency flags
// (Joined=true when joining an existing in-flight run; Queued=true when waiting).
func (svc *BeadService) Dispatch(ctx context.Context, id string, input DispatchInput) (DispatchResult, error) {
	if !input.Agent.Valid() {
		return DispatchResult{}, &ServiceError{Code: CodeInvalidRequest, Message: "invalid agent"}
	}
	if !input.Mode.Valid() {
		return DispatchResult{}, &ServiceError{Code: CodeInvalidRequest, Message: "invalid mode"}
	}
	if input.PermissionMode != "" && !input.PermissionMode.Valid() {
		return DispatchResult{}, &ServiceError{Code: CodeInvalidRequest, Message: "invalid permissionMode"}
	}
	// An explicit chain override is validated fail-fast, the same as the
	// scalar fields above: FR-012a requires per-step PermissionMode to never
	// be silently defaulted, so a step missing (or with an invalid)
	// PermissionMode is a 400, not a fallback to DefaultPermissionMode. An
	// empty/nil input.Chain (the M2 single-step default) is unaffected.
	for i, step := range input.Chain {
		if step.Name == "" {
			return DispatchResult{}, &ServiceError{Code: CodeInvalidRequest, Message: fmt.Sprintf("chain[%d]: name is required", i)}
		}
		if !step.PermissionMode.Valid() {
			return DispatchResult{}, &ServiceError{Code: CodeInvalidRequest, Message: fmt.Sprintf("chain[%d]: invalid or missing permissionMode", i)}
		}
	}

	// If an orchestrator is available, delegate real dispatch to it.
	if svc.orchestrator != nil {
		// Get the bead first to obtain title/desc for the prompt.
		bead, err := svc.GetBead(ctx, id)
		if err != nil {
			return DispatchResult{}, err
		}
		req := OrchestratorDispatchRequest{
			BeadID:         id,
			BeadTitle:      bead.Title,
			BeadDesc:       bead.Desc,
			Agent:          input.Agent,
			Mode:           input.Mode,
			PermissionMode: input.PermissionMode,
			Chain:          input.Chain,
			Skills:         svc.resolveBeadSkills(ctx, id, bead),
		}
		orchResult, orchErr := svc.orchestrator.Dispatch(ctx, req)
		if orchErr != nil {
			return DispatchResult{}, wrapOrchestratorError(orchErr)
		}
		if orchResult.Bead == nil {
			// Defensive: a zero-value result with no error would hand the HTTP
			// layer a 200 with a null bead. Treat a missing bead with no error
			// as an internal fault.
			return DispatchResult{}, &ServiceError{Code: CodeInternal, Message: "orchestrator returned no bead"}
		}

		// Idempotent join: the bead is already in-flight. Skip the bd-claim
		// and WS publish (the run transition already happened on the first
		// dispatch) and return directly so the handler can render 200+joined.
		if orchResult.Joined {
			return DispatchResult{
				Bead:   orchResult.Bead,
				Joined: true,
				Queued: orchResult.Queued,
			}, nil
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
			// Detach from the request context's cancellation: the orchestrator
			// run has already launched, so a client disconnect (which cancels
			// ctx) must not abort this bd claim and leave the store showing the
			// pre-dispatch column while the agent runs. context.WithoutCancel
			// drops the deadline too, so re-apply an explicit one — otherwise a
			// hung bd subprocess could block the request indefinitely.
			claimCtx, claimCancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
			iss, err := svc.cli.Dispatch(claimCtx, id)
			claimCancel()
			if err != nil {
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
		// claim above failed/was unavailable (orchResult.Bead.Column is still
		// authoritative for "the run is active" either way).
		// Return the same merged bead from the HTTP path so the response
		// matches the WS frame (clients otherwise saw a stub here while a
		// concurrent socket-listener saw the full bead).
		// orchResult.Bead is guaranteed non-nil here (the nil-bead case returned above).
		merged := *bead
		merged.Column = orchResult.Bead.Column
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
		return DispatchResult{Bead: &merged, Queued: orchResult.Queued}, nil
	}

	// Legacy stub path: just move the bead to running via bd CLI.
	if err := svc.requireCLI(); err != nil {
		return DispatchResult{}, err
	}

	iss, err := svc.cli.Dispatch(ctx, id)
	if err != nil {
		return DispatchResult{}, wrapCLIError(err)
	}
	b := IssueToBead(&iss, svc.repo)
	svc.publishWrite(ws.EventBeadUpdated, &b)
	return DispatchResult{Bead: &b}, nil
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

// WorktreeDTO is the response body for GET /api/v1/beads/{id}/worktree.
type WorktreeDTO struct {
	BeadID string          `json:"beadID"`
	VCS    string          `json:"vcs"`
	Clean  bool            `json:"clean"`
	Files  []wt.FileChange `json:"files"`
}

// WorktreeAccessor is the interface BeadService uses to query and mutate the
// VCS backend. The real implementation wraps the orchestrator's wt.Backend.
// Defined here (not in orchestrator) to avoid import cycles.
type WorktreeAccessor interface {
	// WorktreeStatus returns the status of the per-bead worktree.
	// Returns ErrWorktreeNotFound when no worktree exists.
	WorktreeStatus(ctx context.Context, beadID string) (wt.WorktreeStatus, error)
	// DiffSummary returns the list of changed files in the per-bead worktree.
	DiffSummary(ctx context.Context, beadID string) ([]wt.FileChange, error)
	// Diff returns a streaming unified diff. path must already pass safeRelPath.
	// Empty path = whole worktree.
	Diff(ctx context.Context, beadID, path string) (io.ReadCloser, error)
	// DefaultVCS returns the VCS label (e.g. "git") for the DTO vcs field.
	DefaultVCS() string

	// Finalize seals the current working-copy changes with message.
	// Returns (true, nil) when a commit was created; (false, nil) for a no-op
	// (clean worktree). A no-change worktree succeeds as a no-op (idempotent).
	Finalize(ctx context.Context, beadID, message string) (bool, error)
	// Push pushes branch muster/<beadID> to remote (resolved via wt.ResolveRemote —
	// empty defaults to "origin").
	Push(ctx context.Context, beadID, remote string) error
	// Remove deregisters and deletes the per-bead worktree directory.
	Remove(ctx context.Context, beadID string) error

	// BeadRunState returns the current run state for the given bead.
	// Returns an empty string when no run record exists (never dispatched or
	// already evicted). The service M1 guard uses this to refuse Finalize/Remove
	// when a run is still active (StepActive or StepPending).
	BeadRunState(beadID string) core.StepStatus
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
	// No attacher wired in: the attach/send (tmux session) feature isn't
	// available in this configuration. Use a dedicated code rather than
	// BD_CLI_MISSING (which wrongly points the client at the bd CLI) or
	// BD_CLI_UNAVAILABLE/503 (which wrongly implies a transient outage). Maps
	// to 501 Not Implemented.
	return &ServiceError{Code: CodeAttachUnavailable, Message: "send not available: this muster instance has no tmux session transport"}
}

// mapWorktreeReadError classifies a wt.Backend read error into a ServiceError.
// Backend-unavailable conditions — the VCS binary vanished after startup
// (exec.ErrNotFound) or the backend reports the selected VCS is unavailable
// (wt.ErrVCSUnavailable) — map to VCS_UNAVAILABLE (412) per the API contract,
// not INTERNAL (500). op names the operation for the error message.
func mapWorktreeReadError(err error, id, op string) *ServiceError {
	switch {
	case errors.Is(err, wt.ErrWorktreeNotFound):
		return &ServiceError{Code: CodeWorktreeNotFound, Message: "no worktree for bead " + id}
	case errors.Is(err, wt.ErrVCSUnavailable), errors.Is(err, exec.ErrNotFound):
		return &ServiceError{Code: CodeVCSUnavailable, Message: op + ": vcs backend unavailable: " + err.Error()}
	case errors.Is(err, wt.ErrWorktreeDirty):
		return &ServiceError{Code: CodeWorktreeDirty, Message: op + ": " + err.Error()}
	default:
		return &ServiceError{Code: CodeInternal, Message: op + " failed: " + err.Error()}
	}
}

// Worktree returns the worktree file-change summary for a bead.
// Returns WORKTREE_NOT_FOUND when no worktree exists (bead was never dispatched
// or the worktree directory was removed). Returns VCS_UNAVAILABLE when the
// WorktreeAccessor is not wired in or the VCS backend is unavailable.
func (svc *BeadService) Worktree(ctx context.Context, id string) (*WorktreeDTO, error) {
	if svc.wtAccessor == nil {
		return nil, &ServiceError{Code: CodeVCSUnavailable, Message: "worktree access not configured"}
	}
	// Verify the bead exists.
	if _, err := svc.GetBead(ctx, id); err != nil {
		return nil, err
	}
	st, err := svc.wtAccessor.WorktreeStatus(ctx, id)
	if err != nil {
		return nil, mapWorktreeReadError(err, id, "worktree status")
	}
	if !st.Exists {
		return nil, &ServiceError{Code: CodeWorktreeNotFound, Message: "no worktree for bead " + id}
	}

	files, err := svc.wtAccessor.DiffSummary(ctx, id)
	if err != nil {
		return nil, mapWorktreeReadError(err, id, "diff summary")
	}
	if files == nil {
		files = []wt.FileChange{} // return [] not null
	}

	return &WorktreeDTO{
		BeadID: id,
		VCS:    svc.wtAccessor.DefaultVCS(),
		Clean:  st.Clean,
		Files:  files,
	}, nil
}

// Diff returns a streaming unified diff for the bead's worktree.
// path must already have been validated by the caller (e.g. via safeRelPath).
// Empty path = whole worktree. Returns WORKTREE_NOT_FOUND / VCS_UNAVAILABLE
// as appropriate. The caller must close the returned ReadCloser.
func (svc *BeadService) Diff(ctx context.Context, id, path string) (io.ReadCloser, error) {
	if svc.wtAccessor == nil {
		return nil, &ServiceError{Code: CodeVCSUnavailable, Message: "worktree access not configured"}
	}
	// Verify the bead exists.
	if _, err := svc.GetBead(ctx, id); err != nil {
		return nil, err
	}
	rc, err := svc.wtAccessor.Diff(ctx, id, path)
	if err != nil {
		return nil, mapWorktreeReadError(err, id, "diff")
	}
	return rc, nil
}

// ── Write-side worktree methods (M4 US2) ─────────────────────────────────────

// lockWtOp acquires the per-bead worktree-operation mutex for id and returns a
// release function. Callers must defer the returned function:
//
//	unlock := svc.lockWtOp(id)
//	defer unlock()
//
// This serializes Finalize and Remove for the same bead to close the TOCTOU
// window between checkRunNotActive (which reads run state) and the actual
// backend call. Push is intentionally excluded — it is permitted at any run
// state and does not mutate the worktree's commit graph.
func (svc *BeadService) lockWtOp(id string) func() {
	v, _ := svc.wtOpMu.LoadOrStore(id, &sync.Mutex{})
	mu := v.(*sync.Mutex)
	mu.Lock()
	return mu.Unlock
}

// checkRunNotActive returns CodeRunAlreadyActive when the bead's current run is
// in a non-terminal state (StepActive or StepPending). Push is permitted at any
// run state (the agent may have finished but the worktree is still dirty).
// Finalize and Remove must wait for a terminal state to avoid racing the agent.
//
// Note: checkRunNotActive does not hold the wtOpMu lock itself; callers
// (FinalizeWorktree, RemoveWorktree) must call lockWtOp before checkRunNotActive
// and hold the lock across the subsequent backend call to close the TOCTOU
// window. A narrow residual window still exists between the agent updating its
// run state and muster observing it, which is accepted as a known limitation.
func (svc *BeadService) checkRunNotActive(id string) error {
	if svc.wtAccessor == nil {
		return nil // no accessor = no run state; caller handles the nil accessor check
	}
	state := svc.wtAccessor.BeadRunState(id)
	if state == core.StepActive || state == core.StepPending {
		return &ServiceError{
			Code:    CodeRunAlreadyActive,
			Message: "cannot finalize/remove worktree while a run is active (state=" + string(state) + ")",
		}
	}
	return nil
}

// FinalizeWorktree seals the per-bead worktree's working-copy changes with
// message. A no-change worktree succeeds as a no-op (idempotent).
// Returns (true, nil) when a commit was created; (false, nil) for a no-op.
//
// M1 guard: returns (false, CodeRunAlreadyActive) when the bead's run is
// StepActive or StepPending. Terminal states (StepDone, StepFailed, or absent) are allowed.
// The per-bead wtOpMu lock is held from the guard check through the backend
// call to close the TOCTOU window between reading run state and acting on it.
func (svc *BeadService) FinalizeWorktree(ctx context.Context, id, message string) (bool, error) {
	if svc.wtAccessor == nil {
		return false, &ServiceError{Code: CodeVCSUnavailable, Message: "worktree access not configured"}
	}
	// Reject NUL bytes in the commit message: git commit -m treats NUL as a
	// message terminator, producing a silently truncated commit. Do NOT use
	// validateField here — that rejects newlines, and multiline commit messages
	// are legitimate.
	if strings.ContainsRune(message, 0) {
		return false, &ServiceError{Code: CodeInvalidRequest, Message: "message must not contain NUL bytes"}
	}
	// Verify the bead exists.
	if _, err := svc.GetBead(ctx, id); err != nil {
		return false, err
	}
	// Acquire per-bead lock before the run-state guard to close the TOCTOU
	// window (tri-review 5): checkRunNotActive + Finalize are executed atomically
	// with respect to concurrent RemoveWorktree for the same bead.
	unlock := svc.lockWtOp(id)
	defer unlock()
	// M1 run-state guard (held under lock).
	if err := svc.checkRunNotActive(id); err != nil {
		return false, err
	}
	committed, err := svc.wtAccessor.Finalize(ctx, id, message)
	if err != nil {
		return false, mapWorktreeReadError(err, id, "finalize")
	}
	if svc.publish != nil {
		svc.publish(ws.Frame{Type: ws.EventWorktreeFinalized, BeadID: id, Committed: &committed})
	}
	return committed, nil
}

// PushWorktree pushes branch muster/<beadID> to remote (resolved via
// wt.ResolveRemote — empty defaults to "origin").
// Push is permitted regardless of the current run state (the run may be finished
// but the branch not yet pushed). An invalid remote name returns CodeInvalidRequest
// immediately, before the backend is called. Push failures map to CodeInternal.
func (svc *BeadService) PushWorktree(ctx context.Context, id, remote string) error {
	if svc.wtAccessor == nil {
		return &ServiceError{Code: CodeVCSUnavailable, Message: "worktree access not configured"}
	}
	// Validate and resolve the remote name early — before the bead lookup and
	// the backend call — so an invalid remote is rejected with a clear 400.
	resolvedRemote, err := wt.ResolveRemote(remote)
	if err != nil {
		return &ServiceError{Code: CodeInvalidRequest, Message: "invalid remote name: " + err.Error()}
	}
	// Verify the bead exists.
	if _, err := svc.GetBead(ctx, id); err != nil {
		return err
	}
	// Pass resolvedRemote (not raw remote) so the backend, WS frame, and HTTP
	// response all use the same value. The backend's own ResolveRemote is an
	// idempotent revalidation (defense-in-depth for direct backend callers).
	if err := svc.wtAccessor.Push(ctx, id, resolvedRemote); err != nil {
		return mapWorktreeReadError(err, id, "push")
	}
	if svc.publish != nil {
		svc.publish(ws.Frame{Type: ws.EventWorktreePushed, BeadID: id, Branch: wt.BranchName(id), Remote: resolvedRemote})
	}
	return nil
}

// RemoveWorktree deregisters and deletes the per-bead worktree directory.
//
// M1 guard: same as FinalizeWorktree — returns CodeRunAlreadyActive when the
// bead's run is StepActive or StepPending.
// The per-bead wtOpMu lock is held from the guard check through the backend
// call, mirroring FinalizeWorktree's TOCTOU fix (tri-review 5).
func (svc *BeadService) RemoveWorktree(ctx context.Context, id string) error {
	if svc.wtAccessor == nil {
		return &ServiceError{Code: CodeVCSUnavailable, Message: "worktree access not configured"}
	}
	// Verify the bead exists.
	if _, err := svc.GetBead(ctx, id); err != nil {
		return err
	}
	// Acquire per-bead lock before the run-state guard (tri-review 5).
	unlock := svc.lockWtOp(id)
	defer unlock()
	// M1 run-state guard (held under lock).
	if err := svc.checkRunNotActive(id); err != nil {
		return err
	}
	if err := svc.wtAccessor.Remove(ctx, id); err != nil {
		return mapWorktreeReadError(err, id, "remove")
	}
	if svc.publish != nil {
		svc.publish(ws.Frame{Type: ws.EventWorktreeRemoved, BeadID: id})
	}
	return nil
}

// StepAdvancer is the interface BeadService uses to advance or loop back the
// step pointer for a live multi-step run. The real implementation is
// *orchestrator.Orchestrator (wrapped in service_adapter.go); tests may
// substitute a fake. Defined here (not in orchestrator) to avoid import cycles.
type StepAdvancer interface {
	// Advance moves the step pointer forward by 1 for the run keyed by beadID.
	// Returns (newStepIdx, chainLen, nil) on success, or (0, 0, *ServiceError)
	// with Code==CodeStepOutOfRange when already at the last step or no chain.
	Advance(ctx context.Context, beadID string) (stepIdx, chainLen int, err error)
	// LoopBack moves the step pointer back to toIdx for the run keyed by beadID.
	// Returns (toIdx, chainLen, nil) on success, or (0, 0, *ServiceError) with
	// Code==CodeStepOutOfRange when toIdx is out of range.
	LoopBack(ctx context.Context, beadID string, toIdx int) (stepIdx, chainLen int, err error)
}

// WithStepAdvancer returns a new BeadService with a step advancer attached.
// The advancer is used by AdvanceStep and LoopBackStep when present.
func (svc *BeadService) WithStepAdvancer(a StepAdvancer) *BeadService {
	svc2 := *svc
	svc2.stepAdvancer = a
	return &svc2
}

// AdvanceStep moves the chain step pointer forward by 1 for the given bead.
// Returns (newStepIdx, chainLen) on success; maps ErrStepOutOfRange → CodeStepOutOfRange.
func (svc *BeadService) AdvanceStep(ctx context.Context, beadID string) (stepIdx, chainLen int, err error) {
	// Verify the bead exists before touching the orchestrator.
	if _, err := svc.GetBead(ctx, beadID); err != nil {
		return 0, 0, err
	}
	if svc.stepAdvancer == nil {
		return 0, 0, &ServiceError{Code: CodeAttachUnavailable, Message: "advance not available: no multi-step dispatcher configured"}
	}
	return svc.stepAdvancer.Advance(ctx, beadID)
}

// LoopBackStep moves the chain step pointer back to toIdx for the given bead.
// Returns (toIdx, chainLen) on success; maps ErrStepOutOfRange → CodeStepOutOfRange.
func (svc *BeadService) LoopBackStep(ctx context.Context, beadID string, toIdx int) (stepIdx, chainLen int, err error) {
	// Verify the bead exists before touching the orchestrator.
	if _, err := svc.GetBead(ctx, beadID); err != nil {
		return 0, 0, err
	}
	if svc.stepAdvancer == nil {
		return 0, 0, &ServiceError{Code: CodeAttachUnavailable, Message: "loopback not available: no multi-step dispatcher configured"}
	}
	return svc.stepAdvancer.LoopBack(ctx, beadID, toIdx)
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
