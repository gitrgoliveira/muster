# Data Model: M2 — First CLI Adapter

**Phase 1 output.** Go-leaning shapes (this is a Go service). Existing M0 enums (`core.AgentID`, `core.Mode`, `core.Column`, `core.StepStatus`) are reused; new types are flagged **NEW**.

## New core enum

```go
// internal/core/enums.go (NEW)
type PermissionMode string

const (
    PermDefault          PermissionMode = "default"
    PermAcceptEdits      PermissionMode = "acceptEdits"
    PermDontAsk          PermissionMode = "dontAsk"
    PermBypassPermissions PermissionMode = "bypassPermissions"
    PermPlan             PermissionMode = "plan"
    PermAuto             PermissionMode = "auto"
)

func (p PermissionMode) Valid() bool { /* allow-list above */ }
```

Mirrors the documented `claude --permission-mode` choice set (spike-verified). Used by FR-021. **Distinct from `core.Mode`** (`plan`/`agent`/…): `Mode` = invocation profile, `PermissionMode` = autonomy.

## Adapter abstraction (`internal/adapter`)

```go
type Adapter interface {
    ID() core.AgentID
    Detect(ctx context.Context) (DetectResult, error)         // installed? version? loggedIn?
    Modes() []Mode                                            // supported invocation profiles
    Invoke(ctx context.Context, req InvokeReq) (Spec, error)  // -> argv/env/cwd to run (transport spawns it)
    Login(ctx context.Context) (LoginFlow, error)             // claude: returns ErrNotSupported (detect-only)
    QuotaSource() QuotaSource                                  // claude M2: None
}

type DetectResult struct {
    Installed bool
    Version   string
    LoggedIn  bool   // from `claude auth status --json`.loggedIn
}

type Mode struct {
    ID   core.Mode          // e.g. ModePlan, ModeAgent
    Args func(pm core.PermissionMode) []string // e.g. plan -> ["--permission-mode","plan"]
}

type InvokeReq struct {
    Bead           core.Bead
    Mode           core.Mode
    PermissionMode core.PermissionMode  // user-supplied (FR-021); never defaulted by muster
    Worktree       string               // cwd
    PromptFile     string               // path to assembled prompt inside the worktree
}

// Spec is the resolved, transport-agnostic launch description.
type Spec struct {
    Argv []string  // e.g. ["claude","--permission-mode","acceptEdits"]
    Env  []string
    Cwd  string
    // PromptDelivery: stdin from PromptFile (wrapper cats it) — see claude-adapter.md
}

type QuotaSource int
const ( QuotaNone QuotaSource = iota; QuotaCLIOutput; QuotaAPIHeaders )
```

`claude` adapter (`internal/adapter/claude`): `Detect` shells `claude auth status --json`; `Modes` returns `plan→--permission-mode plan` and `agent→--permission-mode <pm>`; `Login` returns `ErrNotSupported`; `QuotaSource` returns `QuotaNone`.

## Transport (`internal/tmux`)

```go
type Session struct {
    Name      string    // "muster/<bead>/<step>/<loop>"
    BeadID    string
    StepIdx   int
    Loop      int
    Pane      string
    StartedAt time.Time
}

type Manager interface {
    Detect() (version string, err error)                       // tmux >= 3.2
    Spawn(name, cwd string, env, argv []string) (*Session, error) // default socket; remain-on-exit on
    Pipe(name string) (io.ReadCloser, error)                   // pipe-pane -> reader (raw bytes)
    Capture(name string, withEscapes bool) (string, error)     // capture-pane [-e] -p -S -
    Send(name, keys string) error                              // send-keys
    Attach(name string) (cmd string, err error)                // returns "tmux attach -t <name>"
    DeadStatus(name string) (code int, dead bool, err error)   // #{pane_dead_status}
    Kill(name string) error
    List() ([]Session, error)                                  // muster/ prefix only
}
```

A `fallback` implementation (`internal/tmux/fallback.go`) satisfies the same orchestrator needs via `exec.Command` when `Detect` fails: live pipe from stdout/stderr, exit from `Wait()`, but `Attach`/`Send`/`Capture` return `ErrAttachUnavailable`.

## Worktree (`internal/worktree`)

```go
type Worktree struct { BeadID, Path, Branch, RepoPath string }

// Create or reuse <worktreesDir>/<beadID> on branch muster/<beadID> from repoPath.
func Ensure(worktreesDir, repoPath, beadID string) (Worktree, error)
```
`git worktree add -b muster/<beadID> <path>` (reuse if the path/branch already exist). Errors if `repoPath` is not a git repo.

## Run (in-memory, `internal/orchestrator`)

```go
type Run struct {
    BeadID         string
    StepIdx        int            // always 0 in M2
    Loop           int            // always 0 in M2
    Agent          core.AgentID
    Mode           core.Mode
    PermissionMode core.PermissionMode
    Worktree       string
    Session        string         // tmux session name ("" in fallback)
    State          core.StepStatus // running | done | failed | cancelled
    ExitCode       int
    StartedAt, EndedAt time.Time
}
```
Registry: `map[beadID]*Run` guarded by `sync.RWMutex`. One active run per bead → second dispatch = `409`. Not persisted (rebuilt on restart from `tmux List()`).

## RepoMapping (`internal/orchestrator/repomap.go` + `internal/config`)

```go
type RepoMap map[string]string // prefix (e.g. "mp") -> absolute repo path

func (m RepoMap) Resolve(beadID string) (repoPath string, err error) // prefix before first '-'
```
Parsed from repeatable `--repo prefix=path` / `MUSTER_REPO`. Unmapped prefix → dispatch `422`.

## Extensions to existing types

**`services.DispatchInput`** (add field):
```go
type DispatchInput struct {
    Agent          core.AgentID
    Mode           core.Mode
    PermissionMode core.PermissionMode // NEW (FR-021); validated/allow-listed; resolved from
                                       // request or --default-permission-mode; error if neither
}
```

**`ws.Frame`** (add fields; all `omitempty`):
```go
// runlog.line
BeadID  string `json:"beadID,omitempty"`
StepIdx *int   `json:"stepIdx,omitempty"`  // *int: M1 frames leave it nil; M2 sets it (so the valid value 0 isn't dropped by omitempty)
Seq     uint64 `json:"seq,omitempty"`
Data    string `json:"data,omitempty"`     // base64-encoded raw pane bytes (terminal output is not guaranteed UTF-8)
// tmux.session.opened / closed
Session string `json:"session,omitempty"`
ExitCode *int  `json:"exitCode,omitempty"` // on closed
```
New `EventType`s: `EventRunlogLine="runlog.line"`, `EventTmuxOpened="tmux.session.opened"`, `EventTmuxClosed="tmux.session.closed"`.

**`health.OrchestratorStatusResponse`** (add fields):
```go
TmuxAvailable bool          `json:"tmuxAvailable"`
TmuxVersion   string        `json:"tmuxVersion,omitempty"`
RunningCount  int           `json:"runningCount"`
Adapters      []AdapterInfo `json:"adapters,omitempty"` // {id, installed, version, loggedIn}
```

## State transitions (bead column / step)

```
dispatch(claude) ──> step running, bead column = running   (FR-002, FR-013)
   agent exit 0  ──> step done,    bead column unchanged; outcome recorded via an appended note + bead.updated
   agent exit ≠0 ──> step failed,  bead column unchanged; outcome recorded via an appended note + bead.updated
   timeout/cancel──> step cancelled, session killed
```
`core.ColReview` exists as a column constant but M2 does not persist a distinct review state — completing a run cannot move the bead to `review` (no dedicated write path), so completion is recorded as a note on the bead plus a `bead.updated` broadcast, leaving the column unchanged. A persisted "review" state is tracked as a follow-up (FR-013 completion distinguishable from `in_progress`), not part of M2.
