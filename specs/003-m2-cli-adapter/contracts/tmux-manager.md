# Contract: tmux Transport (`internal/tmux`)

**Primitives verified end-to-end in the 2026-05-29 spike.** Minimum tmux **3.2** (host had 3.6b).

## Socket

muster spawns on the **default tmux socket** (no `-L`), so the user's plain `tmux attach -t muster/<bead>/0/0` connects. (The spike used `-L` only to avoid polluting the user's server.)

## Session naming

`muster/<bead-id>/<step-idx>/<loop-count>` — e.g. `muster/mp-abc/0/0`. M2 always uses step `0`, loop `0`. **Slash-containing names are accepted** (verified). The `muster/` prefix is the discovery filter for restart recovery.

## Primitives → tmux commands

| Manager method | tmux invocation |
|---|---|
| `Detect` | `tmux -V` → parse, require ≥ 3.2 |
| `Spawn(name,cwd,env,argv)` | **race-free 3-step**: `tmux new-session -d -s <name> -x 220 -y 50` (holder shell, no command) → `set-option -t <name> remain-on-exit on` → `respawn-pane -k -t <name> <wrapper>`. Setting remain-on-exit *before* the command runs ensures a fast-failing agent's pane (and its `pane_dead_status`) survives. |
| `Pipe(name)` | `tmux pipe-pane -t <name> 'cat >> <fifo>'` → reader side (raw bytes incl. ANSI). **No `-o`**: `-o` means "only if no pipe is already open", which on restart-recovery would leave a stale pipe from a previous muster process attached and starve the new FIFO of a writer, so we always (re)open the pipe. |
| `Capture(name,esc)` | `tmux capture-pane -p [-e] -S - -t <name>` (full scrollback; `-e` keeps escapes) |
| `Send(name,keys)` | `tmux send-keys -t <name> -l <keys>` — `-l` sends the keys as **literal** text so payloads that happen to match tmux key names (e.g. `Enter`, `C-c`) are delivered verbatim rather than interpreted. |
| `Attach(name)` | returns the string `tmux attach -t <name>` (the client runs it) |
| `DeadStatus(name)` | `tmux display-message -p -t <name> '#{pane_dead} #{pane_dead_status}'` |
| `Kill(name)` | `tmux kill-session -t <name>` |
| `List()` | `tmux list-sessions -F '#{session_name}'` → filter `muster/` → parse names |

## Lifecycle

1. **Spawn** with `remain-on-exit on` so the pane survives process exit (to read the code).
2. **Pipe** the pane to a reader; orchestrator fans bytes to `runlog.line` + persists nothing.
3. On **pane death** (poll `#{pane_dead}` or a `pane-died` hook): read `#{pane_dead_status}` → exit code → emit `tmux.session.closed` + step done/failed → **Kill**.
4. **Restart recovery**: `List()` enumerates `muster/*`; for each, re-`Pipe` and resume; sessions with no live bead/step are killed after a grace period.

## Fallback (tmux absent)

`Detect` fails → orchestrator uses the direct-exec transport: `exec.Command` the wrapper, pipe stdout/stderr live, exit from `cmd.Wait()`. `Attach`/`Send`/`Capture` return `ErrAttachUnavailable` with reason `tmux not installed`; status reports `tmuxAvailable:false`. **No catch-up of earlier output** in fallback (live-from-connect only).

## Raw-output note

`pipe-pane` emits raw terminal control sequences (verified: `[?2004l` bracketed-paste, echoed input). muster does **not** strip them; the UI renders via a terminal emulator (plan decision D1). `capture-pane -e` preserves escapes for faithful catch-up rendering.
