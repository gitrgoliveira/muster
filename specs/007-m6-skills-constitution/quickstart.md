# Quickstart: M6 — Skills & Constitution

**Feature**: `specs/007-m6-skills-constitution` | **Phase**: 1 | Verifies: US1–US5, SC-001..008

A reviewer walkthrough exercising the milestone end-to-end. Assumes `make build` → `bin/muster`, a beads dir, and a `--repo` mapping for the dispatched bead's prefix.

## 0. Build & run

```bash
make build
bin/muster serve --beads-dir /path/to/.beads --repo mp=/path/to/repo
# constitution + imported skills live under the resolved <musterDir> (default ~/.muster)
```

## 1. Constitution round-trip (US2 · SC-002)

```bash
# fresh install: empty default, version 0 (NOT 404)
curl -s localhost:7766/api/v1/constitution
# → {"markdown":"","version":0,"updatedAt":null}

curl -s -X PUT localhost:7766/api/v1/constitution \
  -H 'content-type: application/json' \
  -d '{"markdown":"# Muster agents\n- Always run tests with -race.\n"}'
# → {"markdown":"# Muster agents...","version":1,...}   + WS constitution.changed{version:1}

curl -s localhost:7766/api/v1/constitution   # version persists across restart (reload from disk)
```

## 2. Skills: browse built-ins, import, delete (US3 · SC-003)

```bash
curl -s localhost:7766/api/v1/skills            # full built-in catalog (even with zero imports)
curl -s localhost:7766/api/v1/skills/categories # distinct categories

# import from URL (https only; http allowed for loopback); persisted to <musterDir>/skills/<id>.md
curl -s -X POST localhost:7766/api/v1/skills \
  -H 'content-type: application/json' -d '{"url":"https://example.com/skills/repo-grep.md"}'
# appears in GET /skills, survives restart

curl -s -X DELETE localhost:7766/api/v1/skills/<imported-id>   # 204
curl -s -X DELETE localhost:7766/api/v1/skills/<builtin-id>    # 403 SKILL_READONLY (never silent)
```

## 3. Skill selection via `bd` labels (US4 · option b)

```bash
# tag the bead with reserved skill: labels using the authoritative writer.
# NOTE: `bd label add [issue-id...] [label]` takes ONE label (the last arg), so
# add each separately:
bd -C /path/to/.beads label add <beadID> skill:repo-grep
bd -C /path/to/.beads label add <beadID> skill:run-tests
# muster reads them via `bd label list <id> --json` at dispatch → Bead.Skills =
# [repo-grep, run-tests]; a per-dispatch Step.Skills set unions on top (additive).
```

## 4. Assembly replaces the placeholder (US1 · SC-001) — byte-verifiable

```bash
curl -s -X POST localhost:7766/api/v1/beads/<beadID>/dispatch \
  -H 'content-type: application/json' -d '{"agent":"claude","mode":"plan"}'

# inspect the written prompt (no agent execution needed to verify shape)
cat <worktree>/.muster-prompt-0.txt
# asserts: <system role="muster"> header = constitution markdown (+ version);
#          "Skills loaded:" lists repo-grep + run-tests with PromptStub first lines;
#          "Bead <id>: <title>" + "Acceptance criteria:" <desc>;  <user> = resolved step prompt
```

Two-step chain (US1 AS3): dispatch a `plan→build` chain; after step 0 completes, `.muster-prompt-1.txt` for `build` contains a one-line summary of step 0 drawn from its runlog.

## 5. Warnings never block (US4 · SC-004/005)

```bash
bd -C /path/to/.beads label add <beadID> skill:does-not-exist   # unknown skill
# dispatch → succeeds; WS runlog.warning "skill \"does-not-exist\" not found; skipped"
# a skill whose mcpServers name is absent from the agent's MCP config → warning, dispatch proceeds
```

## 6. Memories via `bd` (US5 · SC-006)

```bash
curl -s -X POST localhost:7766/api/v1/memories -d '{"value":"always run tests with -race"}'   # bd remember
curl -s "localhost:7766/api/v1/memories?q=race"                                                # bd memories race
curl -s -X DELETE localhost:7766/api/v1/memories/<key>                                          # bd forget
curl -s -X POST localhost:7766/api/v1/memories/prime -d '{"beadID":"<beadID>"}'                 # next dispatch gets "Primed memories"
# if bd is missing/errors on any of these → typed error, NOT an empty-list success
```

## 7. Quality gates (SC-007/008)

```bash
make test        # go test -race ./...  — M0–M5 suites green; new M6 packages covered
make cover-check # per-package gates incl. new M6 packages in the thresholds map
make lint
```

Additive-surface check (SC-008): no M0–M5 route/shape/WS-event changed — only `/constitution`, `/skills*`, `/memories*`, `constitution.changed`, `runlog.warning` added.
