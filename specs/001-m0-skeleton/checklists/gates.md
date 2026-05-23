# Quality Gates & Test Requirements Checklist: M0 — Skeleton

**Purpose**: Validate that the test, coverage, build, and pre-merge requirements are 
specified well enough that an implementer can mechanically tell whether the work is "done".
**Created**: 2026-05-22
**Feature**: [plan.md](../plan.md)

> This checklist tests the **requirements about tests and gates**, not the tests themselves.

## Test Coverage Requirements

- [ ] CHK001 Are coverage targets quantified per package? Currently spec says `core ≥80%, api ≥70%`; plan says `core 80%, services 80%, store 80%, ws 75%, render/middleware 90%, api handlers 70%`. Pick one normative source. [Conflict, spec.md §SC-006..SC-007 vs plan.md]
- [ ] CHK002 Is the coverage-measurement method specified (line vs branch vs statement; `go test -cover` vs `go test -coverprofile`)? [Clarity, Gap]
- [ ] CHK003 Are coverage targets specified as a **gate** (CI fails below threshold) or an **aspiration** (reported but non-blocking)? [Clarity, Gap]
- [ ] CHK004 Is the requirement that `go test -race ./...` MUST pass in CI declared anywhere, or is it implicit in plan.md? [Clarity, plan.md]
- [ ] CHK005 Are testing-dependency version pins (testify v1.10.0) treated as requirements or as plan-level choices? [Clarity, plan.md vs spec.md §FR-020]
- [ ] CHK006 Are there requirements about test runtime budget (e.g., "full suite completes in <30 s") so flaky/slow tests are surfaced? [Gap]
- [ ] CHK007 Is there a requirement that every error-matrix row in `contracts/rest-api.md` has a corresponding negative test, or is it only a recommendation? [Clarity, plan.md Phase 8]
- [ ] CHK008 Is there a requirement that every FR has at least one acceptance test, or is the link FR→test left implicit? [Traceability, Gap]
- [ ] CHK009 Are integration tests differentiated from unit tests in the spec (separate target? separate tag?)? [Gap]

## Test Strategy Quality

- [ ] CHK010 Is the "tests-first per layer" rule a normative process requirement (PR rejected if a commit adds production code without a prior failing test) or an informal preference? [Clarity, plan.md §Implementation Phases]
- [ ] CHK011 Is the choice of `httptest.NewServer` vs in-process router testing pinned, or left to the implementer per endpoint? [Clarity, plan.md]
- [ ] CHK012 Are mock vs real dependencies specified per layer? (e.g., services tests use a stub `Store`; api tests use real store + real services). [Completeness, plan.md Phase 4 / Phase 8]
- [ ] CHK013 Is the WS test methodology specified (`coder/websocket.Dial` against an `httptest.NewServer`) such that two implementers would write tests identically? [Clarity, plan.md Phase 5]
- [ ] CHK014 Are deterministic-ID injection requirements specified for `TestCreate_IDCollisionRetry`, so the test is not flaky? [Completeness, plan.md Phase 3]

## Code Quality Gates

- [ ] CHK015 Are formatting requirements specified (`gofmt`, `goimports`, both)? [Gap]
- [ ] CHK016 Are linting requirements specified (`go vet`, `staticcheck`, `golangci-lint` with which linters enabled)? [Gap]
- [ ] CHK017 Are doc-comment requirements specified for exported identifiers (godoc style, mandatory or aspirational)? [Gap]
- [ ] CHK018 Is the build-clean requirement specified (zero warnings from `go build`, `go vet`)? [Gap]
- [ ] CHK019 Is `go.sum` hygiene specified (no missing or unused entries; `go mod tidy` produces no diff)? [Gap]
- [ ] CHK020 Are licence/header requirements for new files specified? [Gap]

## Build / Embed Requirements

- [ ] CHK021 Is the requirement that `ui/` MUST be populated before `go build` documented as a gate (`make build` runs `make ui-copy` first), or left to developer discipline? [Clarity, plan.md §Phase 1 / quickstart.md]
- [ ] CHK022 Is the `//go:generate cp -r prototype/ ui/` directive's behaviour cross-platform (does `cp -r` work on every supported build host)? Or do we need a Go program to do the copy? [Edge Case, plan.md §Phase 1.3]
- [ ] CHK023 Is the `.gitkeep`-in-`ui/` strategy specified well enough to survive a fresh clone? [Completeness, plan.md §Phase 1]
- [ ] CHK024 Is the prototype-to-ui mapping pinned (which files in `prototype/` MUST be copied — all of them? Just a subset? Are there files in `prototype/` that should NOT be embedded, like `debug_test.html`)? [Completeness, Gap]
- [ ] CHK025 Is the build's reproducibility specified (e.g., `go build -trimpath`, `-buildvcs=false`) or left as default? [Gap]

## Pre-Merge Requirements

- [ ] CHK026 Is the pre-merge checklist defined anywhere normative? (Build clean, `go test -race`, coverage gate, smoke test all pass — is this enforced or aspirational?) [Gap]
- [ ] CHK027 Is the smoke-test recipe in `quickstart.md` considered a normative acceptance test, or merely user documentation? [Clarity, quickstart.md]
- [ ] CHK028 Are required PR-description sections specified (e.g., "test plan", "screenshots if UI changed")? [Gap]
- [ ] CHK029 Are commit-message conventions specified (Conventional Commits, sign-off, etc.)? [Gap]
- [ ] CHK030 Is the merge strategy specified (squash vs merge-commit vs rebase)? [Gap]

## CI / Tooling Requirements

- [ ] CHK031 Are CI matrix requirements specified (which Go versions, which platforms)? Currently the spec says "Go 1.26+"; is that the build min, the CI min, or both? [Clarity, spec.md §Assumptions]
- [ ] CHK032 Are required CI checks listed (e.g., must-pass status checks before merge)? [Gap]
- [ ] CHK033 Is coverage reporting tooling specified (`go tool cover`, codecov, or none)? [Gap]
- [ ] CHK034 Are security checks specified (e.g., `govulncheck`, `gosec`)? [Gap]
- [ ] CHK035 Is dependency-update policy specified (Dependabot, manual, none)? [Gap]

## Performance Gates

- [ ] CHK036 Are SC-001/002/004/005 timing budgets enforced anywhere (benchmark in CI? smoke timing? manual check?), or are they observational? [Clarity, spec.md §SC-001..SC-005]
- [ ] CHK037 Is a `Benchmark*` requirement declared for any package (e.g., store list, WS broadcast fan-out)? [Gap]
- [ ] CHK038 Are memory regression bounds specified for the in-memory store? [Gap]

## Observability of the Test Suite Itself

- [ ] CHK039 Are test names required to follow a convention (`TestXxx_Yyy` per layer × scenario), so failures are self-describing in CI output? [Gap]
- [ ] CHK040 Are flake-detection / retry requirements specified, or are flaky tests forbidden? [Gap]
- [ ] CHK041 Is there a requirement that race-detector failures fail the build (vs being reported as warnings)? [Gap]

## Traceability

- [ ] CHK042 Is there a single source-of-truth document mapping FR → SC → test → file, so gate failures point at concrete missing tests? [Traceability, Gap]
- [ ] CHK043 Are the test-name tables in plan.md (Phases 2–10) pinned as **the** test inventory, or can implementers add/remove tests without amending plan.md? [Clarity, plan.md §Implementation Phases]
