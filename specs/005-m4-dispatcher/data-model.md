# Data Model — M4 Dispatcher

All types are **in-memory orchestrator run state** unless noted (Constitution II — no new authoritative durable issue state; nothing here becomes a beads column). Field names are indicative; final Go naming follows existing conventions.

## Scheduler (new, `internal/orchestrator/scheduler.go`)

| Field | Type | Notes |
|---|---|---|
| `capacity` | `int` | max concurrent active runs; `>0` invariant; runtime-mutable via `SetCapacity` |
| `active` | `map[string]struct{}` / count | bead IDs currently running; size ≤ `capacity` |
| `waiting` | `[]*Run` | FIFO queue of pending (state `StepPending`) runs awaiting a slot |

Methods: `admitOrEnqueue(run) (queued bool)`, `onRunEnd() *Run` (pop head to admit), `SetCapacity(n) error` (n>0; drain-not-kill), `Snapshot() (capacity, active int, waiting []string)`. All under the orchestrator's existing `mu`.

**Invariants:** `len(active) ≤ capacity` always; a bead appears in **at most one** of active/waiting (idempotency); lowering capacity never mutates `active` (drain semantics).

## Run (extended — existing `internal/orchestrator/orchestrator.go`)

Existing fields retained (`BeadID`, `StepIdx`, `Loop`, `Agent`, `Mode`, `PermissionMode`, `Worktree`, `Session`, `Pane`, `State`, `ExitCode`, `StartedAt`, `EndedAt`, `cancel`). Added:

| Field | Type | Notes |
|---|---|---|
| `Waiting` | `bool` (derived) | true while in the scheduler `waiting` queue (state `StepPending`); false once admitted (`StepActive`) |
| `Chain` | `*StepChain` | the resolved step chain for this bead (nil ⇒ single implicit step 0, M2 behavior) |
| `Quota` | `QuotaUsage` | best-effort usage captured at run end; `Known:false` until/unless populated |

`StepIdx` (already present, always 0 in M2) becomes the live **step pointer**.

**State transitions (per run):** `StepPending` (queued) → `StepActive` (admitted/launched) → `StepDone` | `StepFailed` (cancel/timeout fold into `StepFailed`, as M2). A multi-step chain advancing produces a **new** active run for the next `StepIdx` over the same worktree; loop-back sets `StepIdx` to an earlier value and starts a new run.

## StepChain / StepProfile (new, `internal/orchestrator/steps.go`)

```
StepChain  = ordered []StepProfile          // e.g. [plan, build, review]
StepProfile {
  Name           string                     // "plan" | "build" | "review" | ...
  PermissionMode core.PermissionMode         // per-step; NEVER silently defaulted (FR-012a)
  PromptRef      string                      // reference to the assembled prompt for this step
}
```

Resolution order for a dispatch: `DispatchRequest.Chain` (explicit) → configured **default chain** → nil (single implicit step 0). Chain length and per-step state are observable via status.

**Validation:** advance target = `StepIdx+1` must be `< len(chain)`; loop-back target must be `≥0` and `< StepIdx`; out-of-range ⇒ `ErrStepOutOfRange` (typed → HTTP 400/409, never silent clamp).

## DispatchResult (new / extended service return)

| Field | Type | Notes |
|---|---|---|
| `Bead` | `*core.Bead` | the active/queued bead (as M2) |
| `Joined` | `bool` | true when an in-flight duplicate joined the existing run (idempotency, FR-018) → HTTP 200 |
| `Queued` | `bool` | true when accepted but waiting for a slot (at capacity) |

## QuotaUsage (new, `internal/orchestrator/quota.go`)

| Field | Type | Notes |
|---|---|---|
| `Known` | `bool` | false ⇒ unknown/unavailable (missing or unparseable record); never fails a run |
| `InputTokens` | `int64` | best-effort, from claude on-disk session record (path/format spike-pinned) |
| `OutputTokens` | `int64` | " |
| `CostUSD` | `float64` | " |

Exposed on run status (additive fields) and via the `run.quota` WS event. `claude` adapter `QuotaSource()` → `QuotaCLIOutput`.

## Config (new, `internal/config/dispatcher.go`)

| Knob | Source | Default | Validation |
|---|---|---|---|
| max concurrent dispatches | `--max-concurrent-dispatches` / `MUSTER_MAX_CONCURRENT_DISPATCHES` | `4` | integer `>0`, else typed error → fail fast at startup |
| push remote name | `--push-remote` / `MUSTER_PUSH_REMOTE` | `origin` | non-empty token |

## Beads impact

**None.** No new column, no new authoritative issue state. Bead-visible status changes (if any) continue to flow through `bd` (`store/bdshell`). Scheduler queue, step pointer, quota, and capacity are all disposable muster-side state (Constitution II).
