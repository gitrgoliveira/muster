# Quickstart — M4 Dispatcher

End-to-end operator walkthrough exercising all five slices. Assumes a running server (`bin/muster serve --beads-dir ... --repo mp=/path/to/repo`) bound to `127.0.0.1:7766`, `git` present, and (for the jj half) a jj-native source repo. Capacity default is **4**.

## 1. Capacity-gated scheduling (US1)

```bash
# Start with capacity 1 to see queueing clearly.
curl -XPUT localhost:7766/api/v1/orchestrator/capacity -d '{"capacity":1}'

# Dispatch three beads quickly.
for id in mp-a mp-b mp-c; do
  curl -XPOST localhost:7766/api/v1/beads/$id/dispatch
done

# Exactly one runs; the other two are queued.
curl localhost:7766/api/v1/orchestrator/status
# → { "capacity":1, "activeCount":1, "waiting":["mp-b","mp-c"], ... }
```
WS clients see `dispatch.admitted` for `mp-a` and `dispatch.queued` for `mp-b`,`mp-c`. When `mp-a` finishes, `mp-b` is admitted automatically (`dispatch.admitted`) with no new request. Raise capacity live:
```bash
curl -XPUT localhost:7766/api/v1/orchestrator/capacity -d '{"capacity":3}'   # mp-b, mp-c admitted
```

## 2. Idempotent dispatch (US4)

```bash
curl -XPOST localhost:7766/api/v1/beads/mp-a/dispatch      # starts a run
curl -XPOST localhost:7766/api/v1/beads/mp-a/dispatch      # 200 { "joined": true, ... } — no second agent
```
Racing two identical dispatches yields exactly one run (both observe the same run). After `mp-a` completes, a fresh dispatch starts a new run (idempotency covers in-flight duplicates only).

## 3. Multi-step plan→build→review chain (US3)

```bash
# Dispatch with an explicit chain (or rely on the configured default chain).
curl -XPOST localhost:7766/api/v1/beads/mp-a/dispatch -d '{
  "chain":[
    {"name":"plan","permissionMode":"plan"},
    {"name":"build","permissionMode":"acceptEdits"},
    {"name":"review","permissionMode":"plan"}
  ]}'

# Attach to the current step (idx>0 now works).
curl localhost:7766/api/v1/beads/mp-a/steps/0/attach     # plan
curl -XPOST localhost:7766/api/v1/beads/mp-a/steps/advance   # → step.advanced, stepIdx:1 (build)
curl localhost:7766/api/v1/beads/mp-a/steps/1/attach     # build (was rejected in M2)
curl -XPOST localhost:7766/api/v1/beads/mp-a/steps/advance   # → stepIdx:2 (review)

# Review decides more build work is needed → loop back to step 1.
curl -XPOST localhost:7766/api/v1/beads/mp-a/steps/loopback -d '{"toIdx":1}'   # → step.loopedback, stepIdx:1
```
Each transition runs that step's profile (its own permission mode) as a fresh agent over the same worktree.

## 4. Finalize & push the work product (US2)

```bash
# After the agent has changed files (visible via the M3 diff endpoint):
curl localhost:7766/api/v1/beads/mp-a/diff                       # M3 read-side

curl -XPOST localhost:7766/api/v1/beads/mp-a/worktree/finalize -d '{"message":"mp-a: implement X"}'
# → { "committed": true, ... }   (a no-change worktree → { "committed": false } no-op success)

curl -XPOST localhost:7766/api/v1/beads/mp-a/worktree/push
# → { "pushed": true, "branch":"muster/mp-a", "remote":"origin" }

# When done with the worktree:
curl -XDELETE localhost:7766/api/v1/beads/mp-a/worktree          # → { "removed": true }
```
WS: `worktree.finalized`, `worktree.pushed`, `worktree.removed`. Works identically against a jj-native repo (jj backend). A missing remote or absent VCS binary yields an explicit error, never a silent success.

## 5. Quota (US5, best-effort)

```bash
curl localhost:7766/api/v1/orchestrator/status
# → per-run: "quota": { "known": true, "inputTokens":..., "outputTokens":..., "costUSD":... }
#   (a run with no parseable usage → "quota": { "known": false } and the run still succeeds)
```
WS clients also receive `run.quota` at run end.

## Verification

```bash
make test          # go test -race ./...  (fakes for git/jj/claude; real-binary tests skip if absent)
make cover-check   # per-package gates, incl. any new M4 package
make lint
```
All M0–M3 suites remain green (SC-007/SC-008); the only migrated tests are the two duplicate-dispatch cases, now asserting the idempotent 200+join contract.
