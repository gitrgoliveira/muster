# Research: M3 — Worktrees (`wt.Backend`)

**Spike date**: 2026-07-03 · **Tools**: `jj` 0.42.0, `git` 2.55.0 (macOS/homebrew)

Per Constitution ("Verify assumptions empirically before building"), the jj and git
worktree/diff contracts below were pinned against the real binaries in a scratch repo,
not trusted from prose. Findings feed the Phase-3 plan.

---

## 1. jj-native detection ("jj source repos only")

The M3 decision is **jj source repos only** — muster does NOT run `jj git init` to
colocate a plain git repo. It must therefore detect whether a source repo is already
jj-native.

- **`jj root`** (and `jj status`) run inside a plain git repo exit **non-zero** with:
  `Error: There is no jj repo in "."` plus a hint to run `jj git init`.
- The same commands in a jj (or jj-colocated) repo succeed and print the workspace root.

**Contract**: the jj backend's availability-for-this-repo probe is `jj root` (exit 0 =
jj-native). A `vcs=jj` dispatch against a repo where `jj root` fails is refused (FR-004a)
— no auto-colocate, no git fallback.

> Note: `jj git init --colocate` *does* work to colocate an existing git repo (verified),
> but is deliberately **out of scope** — kept in reserve for a future "auto-colocate"
> option, not used in M3.

## 2. Per-bead isolated working copy

| Backend | Create command | Result |
|---|---|---|
| git | `git worktree add -b muster/<beadID> <path>` (existing M2 `worktree.Ensure`) | linked worktree, dedicated branch |
| jj  | `jj workspace add <path>` (run from the source repo) | named workspace, own working-copy commit `@` |

- `jj workspace add <path>` prints `Created workspace in "<path>"` and registers it;
  `jj workspace list` shows `default` + the new workspace, each with its own `@` commit.
- The jj workspace name defaults to the path basename. muster should pass an explicit
  per-bead path (e.g. `<worktreesDir>/<beadID>`), mirroring the git layout, so the two
  backends share the same on-disk directory convention.
- **Teardown (M4, not M3)**: `jj workspace forget <name>` deregisters it. Verified it
  drops from `jj workspace list`. (git equivalent: `git worktree remove`.)

## 3. Change summary (`DiffSummary` → `[]FileChange`)

Both backends emit a compact `<kind> <path>` summary that maps directly onto `FileChange`:

| Backend | Command | Output shape | Kind letters |
|---|---|---|---|
| git | `git diff --name-status HEAD` | `M\ta.txt` / `D\tb.txt` / `A\tc.txt` / `R100\told\tnew` | M A D R C (T) |
| jj  | `jj diff --summary`           | `M a.txt` / `D b.txt` / `A c.txt` | M A D R C |

- git uses **TAB** separators and rename appends a similarity score (`R100`); jj uses a
  **space** separator. Parsers differ per backend — do not share the tokenizer blindly.
- Map both to `changeKind ∈ {added, modified, deleted, renamed, copied}`; treat any
  unknown leading letter defensively.

### ⚠️ git untracked-files gotcha (the key spike finding)

The M2 worktree holds the agent's edits as **uncommitted working-tree** changes, and the
agent typically creates **new (untracked)** files. Plain `git diff HEAD` and
`git diff --name-status HEAD` **silently omit untracked files**:

```
$ git diff --name-status HEAD      # c.txt (new) is MISSING
M  a.txt
D  b.txt
$ git status --porcelain           # c.txt shows as ??
 M a.txt
 D b.txt
?? c.txt
```

Two ways to include new files:

1. **`git add -AN` (intent-to-add) then `git diff HEAD`** — untracked files appear as
   additions in both summary and unified diff. **Downside: mutates the index** (leaves
   intent-to-add marks), which could race a still-running agent that inspects/uses the
   index. Verified it works.
2. **Non-mutating (recommended)**: build the summary from `git status --porcelain=v1 -z`
   (includes `??` untracked), and build the unified diff as `git diff HEAD` **plus**
   `git diff --no-index -- /dev/null <file>` appended per untracked file. No index writes.

**Plan decision to record**: the git backend uses the **non-mutating** path (option 2) so
diff reads never perturb the agent's worktree/index. jj has no equivalent problem — it
auto-snapshots the working copy (see §5), so `jj diff` already reflects new files.

## 4. Unified diff (`Diff` → stream)

- **`jj diff --git [<path>]`** produces **byte-compatible git-format unified diff**
  (`diff --git a/… b/…`, `index …`, `new file mode …`, `@@ … @@`) — verified identical in
  shape to `git diff`. So both backends can serve the same media type; a client cannot
  tell git from jj by the diff bytes.
- **`git diff HEAD [-- <path>]`** for the git backend (plus the untracked handling in §3).
- Per-file scoping works on both: `jj diff --git <path>` and `git diff HEAD -- <path>`.
- **Streaming**: both are ordinary stdout streams — pipe the child's stdout straight to
  the HTTP response (FR-008), never buffer the whole diff.
- **`?path=` safety (FR-007)**: validate the path is relative, `filepath.IsLocal`, and
  resolves inside the worktree BEFORE passing to the VCS. Do not rely on the VCS to reject
  traversal — pass the validated, worktree-relative path only.

## 5. jj working-copy auto-snapshot (behavioral note)

Every `jj` command **auto-snapshots** the working copy into the `@` commit (the `@` hash
changes as files are edited). Consequences:

- `jj diff` (default `@` vs its parent `@-`) always reflects live on-disk state, including
  newly-created files with no explicit `add`. This is why jj sidesteps the git untracked
  gotcha.
- Reads are benign but not side-effect-free: they advance the jj op log. Acceptable for a
  read endpoint; note it so it isn't mistaken for corruption.

## 6. Identity / config independence

- `jj workspace add` warns *"Name and email not configured … can't be pushed"* but
  `add`/`status`/`diff`/`summary` all **succeed without identity**. Only `push` (M4) needs
  it. M3 (read + create) requires **no** jj user config → muster sets none (Constitution:
  no credential/identity handling).
- The spike ran with `JJ_CONFIG=/dev/null` to prove no ambient user config is required.
  Tests should pin `JJ_CONFIG` (and `GIT_CONFIG_*`) to hermetic values so CI is
  reproducible regardless of the developer's global config.

## 7. Testing strategy (Constitution IV)

- **Fakes-on-`$PATH`**: argv-recording fake `jj` (mirroring the M2 fake-`tmux`/fake-`claude`
  pattern) for unit tests of command construction and output parsing (feed canned
  `diff --summary` / `--git` fixtures).
- **Real-binary integration test**: gated on `jj` presence, skipped when absent (exactly
  the M2 real-`tmux` pattern). git already has a real-repo test helper
  (`internal/worktree/testhelp_test.go`) to extend.
- Pin `JJ_CONFIG`/git identity env in tests for hermeticity.

## 8. Pinned command contracts (summary)

| Operation | git | jj |
|---|---|---|
| Detect backend usable for repo | `git rev-parse --git-dir` | `jj root` (exit 0 = jj-native) |
| Create per-bead worktree | `git worktree add -b muster/<id> <path>` | `jj workspace add <path>` |
| Change summary | `git status --porcelain=v1 -z` (incl. untracked) | `jj diff --summary` |
| Unified diff (all) | `git diff HEAD` + `--no-index` per untracked | `jj diff --git` |
| Unified diff (one file) | `git diff HEAD -- <path>` | `jj diff --git <path>` |
| Teardown (M4) | `git worktree remove` | `jj workspace forget <name>` |

## 9. Open items carried to Plan

- Confirm the on-disk layout: reuse `<worktreesDir>/<beadID>` for both backends; the jj
  workspace name = beadID.
- Decide the git summary source of truth: `git status --porcelain` (chosen, non-mutating)
  vs `git diff --name-status HEAD` + separate untracked scan — the former is one command.
- `WorktreeStatus.clean/ahead/behind`: git via `git status --porcelain` + `rev-list
  --count`; jj via `jj status` / `jj log`. Ahead/behind is lower priority (US4) — may be
  reported as best-effort/omitted in M3.
