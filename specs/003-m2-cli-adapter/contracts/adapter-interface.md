# Contract: `Adapter` interface (`internal/adapter`)

The stable seam M5 extends (gemini/codex/opencode). M2 implements only `claude`.

```go
type Adapter interface {
    ID() core.AgentID
    Detect(ctx context.Context) (DetectResult, error)
    Modes() []Mode
    Invoke(ctx context.Context, req InvokeReq) (Spec, error)
    Login(ctx context.Context) (LoginFlow, error)
    QuotaSource() QuotaSource
}
```

- **`Detect`** is side-effect-free and fast (probe binary + auth). Used at startup and by `orchestrator/status`. Errors only on unexpected failures; "not installed"/"not logged in" are `DetectResult` fields, not errors.
- **`Invoke`** returns a transport-agnostic `Spec{Argv, Env, Cwd}` — it does **not** spawn anything. The orchestrator hands the `Spec` to the tmux/fallback transport. This keeps adapters free of process/tmux concerns (so the same `claude` adapter would work under a future non-tmux transport).
- **`Login`** may return `ErrNotSupported` (claude M2 is detect-only). Adapters MUST NOT store credentials.
- **`QuotaSource`** is advisory metadata; M2 `claude` returns `QuotaNone` (quota is M4).

## RunEvent (transport → orchestrator)

The transport yields a stream the orchestrator consumes. For the M2 terminal transport this is raw output + lifecycle:

```go
type RunEvent struct {
    Kind     RunEventKind // Output | Opened | Closed
    Data     []byte       // Output: raw pane bytes
    ExitCode int          // Closed
}
```

Invariants:
- `Output` events preserve byte order (sequence-numbered downstream as `runlog.line`).
- Exactly one `Closed` per run; carries the exit code (FR-013).
- Cancellation: the orchestrator's `context.Context` cancel → transport kills the session → `Closed{cancelled}`.

## Registry

`adapter.Registry` maps `core.AgentID → Adapter`. M2 registers `claude` only. Dispatch to an unregistered agent → `501 ADAPTER_NOT_FOUND`; a registered agent whose binary isn't installed → `501 ADAPTER_NOT_INSTALLED` (FR-002, US5).
