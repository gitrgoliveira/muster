# Quickstart: M3 — Worktrees & Diff exposure

Prereqs: `make build`, `git` (required), `bd` (for write endpoints), optionally `jj` ≥ 0.42
and `tmux` (for live runlog). Serves loopback only.

## 1. Serve with a git repo mapped

```bash
bin/muster serve \
  --beads-dir /path/to/.beads \
  --repo mp=/path/to/git-repo \
  --worktrees-dir ~/.muster/worktrees \
  --default-vcs git            # NEW (M3): git | jj, default git
```

Check backend availability:

```bash
curl -s localhost:7766/api/v1/orchestrator/status | jq '{vcs, worktreeCount}'
# { "vcs": { "defaultVCS": "git", "git": {"available": true}, "jj": {"available": true, "version": "0.42.0"} },
#   "worktreeCount": 0 }
```

## 2. Dispatch a bead, then inspect its worktree

```bash
curl -sX POST localhost:7766/api/v1/beads/mp-abc/dispatch \
  -H 'Content-Type: application/json' \
  -d '{"agent":"claude","mode":"agent","permissionMode":"acceptEdits"}'
# 202 running  (agent edits files inside the per-bead worktree on branch muster/mp-abc)
```

**File list** (change summary — includes newly-created/untracked files):

```bash
curl -s localhost:7766/api/v1/beads/mp-abc/worktree | jq
# { "beadID":"mp-abc","vcs":"git","clean":false,
#   "files":[ {"path":"internal/foo.go","kind":"added"},
#             {"path":"README.md","kind":"modified"} ] }
```

**Whole diff** (git-format unified diff, streamed):

```bash
curl -s localhost:7766/api/v1/beads/mp-abc/diff
# diff --git a/internal/foo.go b/internal/foo.go
# new file mode 100644
# ...
```

**Single-file diff**:

```bash
curl -s 'localhost:7766/api/v1/beads/mp-abc/diff?path=README.md'
# diff --git a/README.md b/README.md ...
```

**Path safety** — traversal is rejected:

```bash
curl -s -o /dev/null -w '%{http_code}\n' 'localhost:7766/api/v1/beads/mp-abc/diff?path=../../etc/passwd'
# 400   (INVALID_PATH — never discloses files outside the worktree)
```

**No worktree yet** (bead never dispatched):

```bash
curl -s -o /dev/null -w '%{http_code}\n' localhost:7766/api/v1/beads/mp-neverrun/worktree
# 404   (WORKTREE_NOT_FOUND)
```

## 3. Using the jj backend

jj requires a **jj-native source repo** (M3 does not colocate a plain git repo):

```bash
# The mapped repo must already be a jj repo (jj root succeeds inside it).
bin/muster serve --beads-dir /path/to/.beads \
  --repo mp=/path/to/jj-repo --default-vcs jj
```

The `/worktree` and `/diff` responses are identical in shape to git (jj diffs are emitted
in git format). If `--default-vcs jj` is set but the mapped repo is not jj-native, dispatch
is refused:

```bash
curl -sX POST localhost:7766/api/v1/beads/mp-abc/dispatch -d '{"agent":"claude","mode":"agent","permissionMode":"acceptEdits"}'
# 412  VCS_UNAVAILABLE   (jj source repo required; no silent fallback to git)
```

## 4. Automated verification

```bash
make test          # unit + fake-jj/git; real-jj & real-git integration run if binaries present
go test ./internal/wt/...            # the new abstraction
go test -race ./...                  # race-clean gate (Constitution IV)
```

`make test` never requires `jj`: fakes cover command-construction and parsing; the
real-`jj` integration test skips automatically when `jj` isn't on `$PATH` (same pattern as
M2's real-`tmux` tests).
