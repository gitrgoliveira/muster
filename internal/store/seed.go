package store

import (
	"time"

	"github.com/gitrgoliveira/muster/internal/core"
)

// SeedIssues returns 4 prototype issues for use in tests and development.
// IDs use the mp-* prefix to match realistic beads issue IDs.
func SeedIssues() []Issue {
	now := time.Now().UTC()
	started := now.Add(-2 * time.Hour)
	closed := now.Add(-1 * time.Hour)
	return []Issue{
		{
			ID:          "mp-aaa",
			Title:       "Implement feature alpha",
			Description: "First issue in the backlog, open status.",
			Status:      "open",
			Priority:    2,
			IssueType:   "feature",
			Owner:       "alice",
			CreatedAt:   now.Add(-24 * time.Hour),
			UpdatedAt:   now,
		},
		{
			ID:          "mp-bbb",
			Title:       "Fix bug beta",
			Description: "In-progress bug fix.",
			Status:      "in_progress",
			Priority:    1,
			IssueType:   "bug",
			Assignee:    "claude",
			Owner:       "bob",
			CreatedAt:   now.Add(-12 * time.Hour),
			UpdatedAt:   now,
			StartedAt:   &started,
		},
		{
			ID:          "mp-ccc",
			Title:       "Closed chore gamma",
			Description: "Completed maintenance task.",
			Status:      "closed",
			Priority:    3,
			IssueType:   "chore",
			Owner:       "alice",
			CreatedAt:   now.Add(-48 * time.Hour),
			UpdatedAt:   now,
			ClosedAt:    &closed,
		},
		{
			ID:          "mp-ddd",
			Title:       "Backlog task delta",
			Description: "Another open issue.",
			Status:      "open",
			Priority:    0,
			IssueType:   "task",
			Owner:       "carol",
			CreatedAt:   now.Add(-6 * time.Hour),
			UpdatedAt:   now,
		},
	}
}

// SeedBeads returns the 14 prototype beads transcribed from prototype/data.jsx.
func SeedBeads() []core.Bead {
	beads := []core.Bead{
		bdA1f2(),
		bdC411(),
		bd7c0d(),
		bd9aa1(),
		bd8b44(),
		bd3e80(),
		bdD091(),
		bdB210(),
		bd4f12(),
		bd2d55(),
		bd7e21(),
		bd5e91(),
		bd6a02(),
		bd4a11(),
	}
	// Set derived fields.
	for i := range beads {
		b := &beads[i]
		b.Estimate = core.DeriveEstimate(b.TokensBudget)
		b.Assignee = core.DeriveAssignee(b.Steps)
		b.Comments = core.DeriveCommentCount(b.History, b.Reviewer)
	}
	return beads
}

func bdA1f2() core.Bead {
	history := []core.HistoryEvent{
		{At: "Mon 08:55", Kind: core.EvOpened, Actor: "you@yours.dev"},
		{At: "Mon 09:02", Kind: core.EvScheduled, Actor: "you@yours.dev"},
		{At: "Mon 09:14", Kind: core.EvClaimed, Actor: "dispatcher", Agent: core.AgentClaude},
		{At: "Mon 09:14", Kind: core.EvStarted, Actor: "claude", Note: "plan step"},
		{At: "Mon 09:21", Kind: core.EvComment, Actor: "claude", Note: "3 acceptance tests drafted"},
		{At: "Mon 09:34", Kind: core.EvStarted, Actor: "claude", Note: "build step"},
		{At: "Mon 10:02", Kind: core.EvDiscovered, Actor: "claude", Note: "spawned bd-3e80 (flaky checkout)"},
	}
	return core.Bead{
		ID:       "bd-a1f2",
		Title:    "Refactor auth middleware for OAuth refresh tokens",
		Desc:     "Existing middleware swallows 401s when the access token is expired. Need a refresh dance with concurrency-safe locking, plus session-scoped revocation.",
		Type:     core.TypeFeature,
		Column:   core.ColRunning,
		Priority: 0,
		Labels:   []string{"oauth", "security", "middleware"},
		VCS:      core.VCSJJ,
		Branch:   "jj/bd-a1f2-oauth-refresh",
		Ready:    true,
		Repo:     "main",
		Formula:  "speckit-flow",
		Skills:   []string{"repo-grep", "run-tests", "beads-memory"},
		Steps: []core.Step{
			{Agent: core.AgentClaude, Mode: core.ModePlan, Skills: []string{"speckit"}, Status: core.StepDone, Note: "Spec ratified, 3 acceptance tests"},
			{Agent: core.AgentClaude, Mode: core.ModePlan, Skills: []string{}, Status: core.StepDone, Note: "6 atomic steps planned"},
			{Agent: core.AgentClaude, Mode: core.ModeBuild, Skills: []string{}, Status: core.StepActive, Note: "Implementing token refresh queue"},
			{Agent: core.AgentGemini, Mode: core.ModeReview, Skills: []string{}, Status: core.StepPending},
			{Agent: core.AgentClaude, Mode: core.ModeAgent, Skills: []string{}, Status: core.StepPending},
		},
		SubBeads: []core.SubBead{
			{ID: "bd-a1f2.1", Title: "Token refresh queue with singleflight lock", Status: core.StepDone, Agent: core.AgentClaude},
			{ID: "bd-a1f2.2", Title: "Session-scoped revocation cascade", Status: core.StepActive, Agent: core.AgentClaude},
			{ID: "bd-a1f2.3", Title: "Migration script for legacy session tokens", Status: core.StepPending, Agent: core.AgentClaude},
			{ID: "bd-a1f2.4", Title: "Backfill audit log for revoked sessions", Status: core.StepPending, Agent: core.AgentGemini},
		},
		Gates:   []core.Gate{},
		History: history,
		Acceptance: []core.Acceptance{
			{Text: "Concurrent 401s dedupe via singleflight refresh", Done: true},
			{Text: "Revoked refresh tokens return 401 immediately", Done: true},
			{Text: "Session-scoped revocation cascades to all devices", Done: false},
			{Text: "Migration script handles legacy session tokens", Done: false},
		},
		TokensUsed:   184320,
		TokensBudget: 250000,
		NowPlaying:   &core.NowPlaying{Action: "editing src/auth/revoke.ts", Since: 38, Kind: core.NPKTool},
		Reviewer:     nil,
		CreatedAt:    "Mon 09:14",
		OpenedAt:     "Mon 09:14",
		LastActivity: "Mon 10:02",
		Log: []core.LogEntry{
			{T: "09:14:02", Kind: core.LogSystem, Msg: "Task claimed by Claude Code · worktree wt-a1f2"},
			{T: "09:14:11", Kind: core.LogTool, Msg: "read_file src/auth/middleware.ts"},
			{T: "09:14:14", Kind: core.LogTool, Msg: "read_file src/auth/tokens.ts"},
			{T: "09:14:22", Kind: core.LogThought, Msg: "The refresh path needs a singleflight lock so concurrent 401s don't each spawn a refresh."},
			{T: "09:14:31", Kind: core.LogTool, Msg: "edit_file src/auth/refresh-queue.ts (new)"},
			{T: "09:14:48", Kind: core.LogTool, Msg: "edit_file src/auth/middleware.ts"},
			{T: "09:15:02", Kind: core.LogTool, Msg: "shell: pnpm test auth"},
			{T: "09:15:31", Kind: core.LogOutput, Msg: "  ✓ refresh queue dedupes concurrent 401s (412ms)"},
			{T: "09:15:31", Kind: core.LogOutput, Msg: "  ✓ revoked refresh tokens 401 immediately (88ms)"},
			{T: "09:15:31", Kind: core.LogOutput, Msg: "  ✗ session-scoped revocation cascades (timeout)"},
			{T: "09:15:34", Kind: core.LogThought, Msg: "Cascade test times out — revoke is async, need to await flush."},
			{T: "09:15:39", Kind: core.LogTool, Msg: "edit_file src/auth/revoke.ts"},
		},
		Files: []core.FileChange{
			{Path: "src/auth/middleware.ts", Status: core.FileModified, Adds: 24, Dels: 11},
			{Path: "src/auth/refresh-queue.ts", Status: core.FileAdded, Adds: 87, Dels: 0},
			{Path: "src/auth/revoke.ts", Status: core.FileModified, Adds: 9, Dels: 2},
			{Path: "src/auth/tokens.ts", Status: core.FileModified, Adds: 3, Dels: 3},
			{Path: "test/auth/refresh.spec.ts", Status: core.FileAdded, Adds: 64, Dels: 0},
		},
		DiffPreview: "@@ src/auth/middleware.ts @@\n-  if (res.status === 401) {\n-    await refresh(token);\n-    return retry(req);\n-  }\n+  if (res.status === 401) {\n+    const fresh = await refreshQueue.dedupe(token.sub, () => refresh(token));\n+    if (!fresh) throw new SessionRevoked(token.sub);\n+    return retry(req, fresh);\n+  }",
		Blocks:      []string{"bd-c411"},
		BlockedBy:   []string{},
	}
}

func bdC411() core.Bead {
	history := []core.HistoryEvent{
		{At: "Mon 10:55", Kind: core.EvOpened, Actor: "you@yours.dev"},
		{At: "Mon 10:58", Kind: core.EvBlocked, Actor: "dispatcher", Note: "waits on bd-a1f2"},
		{At: "Mon 11:02", Kind: core.EvClaimed, Actor: "dispatcher", Agent: core.AgentGemini},
		{At: "Mon 11:02", Kind: core.EvStarted, Actor: "gemini", Note: "agent step · walking bd graph"},
	}
	return core.Bead{
		ID:       "bd-c411",
		Title:    "Wire Beads dependency graph into changelog generator",
		Desc:     "Generate CHANGELOG entries by walking the bead graph between two tags. Group by epic, surface blocked-by chains.",
		Type:     core.TypeFeature,
		Column:   core.ColRunning,
		Priority: 1,
		Labels:   []string{"changelog", "release"},
		VCS:      core.VCSJJ,
		Branch:   "jj/bd-c411-changelog",
		Ready:    false,
		Repo:     "main",
		Formula:  "changelog-gen",
		Skills:   []string{"repo-grep", "beads-memory"},
		Steps: []core.Step{
			{Agent: core.AgentClaude, Mode: core.ModePlan, Skills: []string{"speckit"}, Status: core.StepDone},
			{Agent: core.AgentGemini, Mode: core.ModePlan, Skills: []string{}, Status: core.StepDone},
			{Agent: core.AgentGemini, Mode: core.ModeAgent, Skills: []string{}, Status: core.StepActive, Note: "Walking bd graph from v0.4.0..HEAD"},
			{Agent: core.AgentClaude, Mode: core.ModeReview, Skills: []string{}, Status: core.StepPending},
			{Agent: core.AgentClaude, Mode: core.ModeAgent, Skills: []string{}, Status: core.StepPending},
		},
		SubBeads: []core.SubBead{
			{ID: "bd-c411.1", Title: "Walk bd graph topology, group by epic", Status: core.StepActive, Agent: core.AgentGemini},
			{ID: "bd-c411.2", Title: "Markdown renderer with epic headings", Status: core.StepPending, Agent: core.AgentGemini},
		},
		Gates:   []core.Gate{},
		History: history,
		Acceptance: []core.Acceptance{
			{Text: "Walks bd graph from tag→tag", Done: false},
			{Text: "Groups entries by epic root, topo-sort within", Done: false},
			{Text: "Markdown output passes shellcheck on CI", Done: false},
		},
		TokensUsed:   42110,
		TokensBudget: 250000,
		NowPlaying:   &core.NowPlaying{Action: "walking bd graph v0.4.0..HEAD · 127 beads", Since: 502, Kind: core.NPKThought},
		Reviewer:     nil,
		CreatedAt:    "Mon 11:02",
		OpenedAt:     "Mon 11:02",
		LastActivity: "Mon 11:02",
		Log: []core.LogEntry{
			{T: "11:02:14", Kind: core.LogSystem, Msg: "Task claimed by Gemini CLI · worktree wt-c411"},
			{T: "11:02:33", Kind: core.LogTool, Msg: "shell: bd export --since v0.4.0 --json"},
			{T: "11:02:41", Kind: core.LogOutput, Msg: "127 beads · 14 epics · 8 chains"},
			{T: "11:02:55", Kind: core.LogThought, Msg: "Group by epic root, then topo-sort within each group."},
			{T: "11:03:08", Kind: core.LogTool, Msg: "edit_file scripts/changelog.ts"},
		},
		Files: []core.FileChange{
			{Path: "scripts/changelog.ts", Status: core.FileModified, Adds: 134, Dels: 42},
			{Path: "scripts/__tests__/changelog.spec.ts", Status: core.FileAdded, Adds: 89, Dels: 0},
		},
		Blocks:    []string{},
		BlockedBy: []string{"bd-a1f2"},
	}
}

func bd7c0d() core.Bead {
	history := []core.HistoryEvent{
		{At: "Mon 08:24", Kind: core.EvOpened, Actor: "you@yours.dev"},
		{At: "Mon 08:30", Kind: core.EvScheduled, Actor: "you@yours.dev"},
	}
	return core.Bead{
		ID:       "bd-7c0d",
		Title:    "Migrate legacy invoice schema → v3",
		Desc:     "Backfill v3 fields from v2 rows, dual-write window, then cutover.",
		Type:     core.TypeEpic,
		Column:   core.ColScheduled,
		Priority: 0,
		Labels:   []string{"migration", "billing"},
		VCS:      core.VCSJJ,
		Branch:   "",
		Ready:    true,
		Repo:     "backend-repo",
		Formula:  "migrate-v3",
		Skills:   []string{"sql", "run-tests", "sentry"},
		Steps: []core.Step{
			{Agent: core.AgentClaude, Mode: core.ModePlan, Skills: []string{"speckit"}, Status: core.StepDone},
			{Agent: core.AgentGemini, Mode: core.ModePlan, Skills: []string{"openspec"}, Status: core.StepPending},
			{Agent: core.AgentClaude, Mode: core.ModePlan, Skills: []string{}, Status: core.StepPending},
			{Agent: core.AgentClaude, Mode: core.ModeBuild, Skills: []string{}, Status: core.StepPending},
			{Agent: core.AgentCodex, Mode: core.ModeReview, Skills: []string{}, Status: core.StepPending},
			{Agent: core.AgentClaude, Mode: core.ModeAgent, Skills: []string{}, Status: core.StepPending},
		},
		SubBeads: []core.SubBead{},
		Gates: []core.Gate{
			{Kind: core.GateHuman, Label: "@alice approves dual-write plan", Status: core.GateWaiting},
		},
		History: history,
		Acceptance: []core.Acceptance{
			{Text: "Dual-write window plan reviewed by @alice", Done: false},
			{Text: "Backfill job is resumable from any chunk", Done: false},
			{Text: "Cutover requires zero downtime", Done: false},
		},
		TokensUsed:   0,
		TokensBudget: 400000,
		Reviewer:     nil,
		CreatedAt:    "Mon 08:30",
		OpenedAt:     "Mon 08:30",
		LastActivity: "Mon 08:30",
		Log:          []core.LogEntry{},
		Files:        []core.FileChange{},
		Blocks:       []string{"bd-9aa1"},
		BlockedBy:    []string{},
	}
}

func bd9aa1() core.Bead {
	history := []core.HistoryEvent{
		{At: "Mon 10:38", Kind: core.EvOpened, Actor: "you@yours.dev"},
		{At: "Mon 10:40", Kind: core.EvBlocked, Actor: "dispatcher", Note: "waits on bd-7c0d"},
		{At: "Mon 10:45", Kind: core.EvScheduled, Actor: "you@yours.dev"},
	}
	return core.Bead{
		ID:       "bd-9aa1",
		Title:    "Add full-text search to /docs route",
		Desc:     "Postgres tsvector + GIN index. Snippets with <mark> highlighting.",
		Type:     core.TypeFeature,
		Column:   core.ColScheduled,
		Priority: 2,
		Labels:   []string{"search", "docs"},
		VCS:      core.VCSJJ,
		Branch:   "",
		Ready:    false,
		Repo:     "frontend-repo",
		Skills:   []string{"sql", "browser", "run-tests"},
		Steps: []core.Step{
			{Agent: core.AgentOpenCode, Mode: core.ModePlan, Skills: []string{}, Status: core.StepPending},
			{Agent: core.AgentOpenCode, Mode: core.ModeBuild, Skills: []string{}, Status: core.StepPending},
			{Agent: core.AgentGemini, Mode: core.ModeReview, Skills: []string{}, Status: core.StepPending},
		},
		SubBeads:     []core.SubBead{},
		Gates:        []core.Gate{},
		History:      history,
		Acceptance:   []core.Acceptance{},
		TokensUsed:   0,
		TokensBudget: 200000,
		Reviewer:     nil,
		CreatedAt:    "Mon 10:45",
		OpenedAt:     "Mon 10:45",
		LastActivity: "Mon 10:45",
		Log:          []core.LogEntry{},
		Files:        []core.FileChange{},
		Blocks:       []string{},
		BlockedBy:    []string{"bd-7c0d"},
	}
}

func bd8b44() core.Bead {
	history := []core.HistoryEvent{
		{At: "Sun 23:40", Kind: core.EvOpened, Actor: "you@yours.dev"},
		{At: "Sun 23:55", Kind: core.EvClaimed, Actor: "dispatcher", Agent: core.AgentClaude},
		{At: "Sun 23:55", Kind: core.EvStarted, Actor: "claude"},
		{At: "Mon 02:11", Kind: core.EvFailed, Actor: "claude", Note: "token budget exhausted at 92%"},
		{At: "Mon 02:11", Kind: core.EvRequeued, Actor: "dispatcher"},
		{At: "Mon 06:30", Kind: core.EvSplit, Actor: "dispatcher", Note: "auto-split into 3 sub-beads"},
	}
	return core.Bead{
		ID:       "bd-8b44",
		Title:    "Token budget exhausted on schema migration",
		Desc:     "Returned to queue at 92% of budget. Likely needs to be split into 3 sub-beads.",
		Type:     core.TypeTask,
		Column:   core.ColScheduled,
		Priority: 1,
		Labels:   []string{"migration", "requeue"},
		VCS:      core.VCSJJ,
		Branch:   "jj/bd-8b44-stuck",
		Ready:    true,
		Repo:     "billing-repo",
		Skills:   []string{},
		Steps: []core.Step{
			{Agent: core.AgentClaude, Mode: core.ModePlan, Skills: []string{}, Status: core.StepDone},
			{Agent: core.AgentClaude, Mode: core.ModeAgent, Skills: []string{}, Status: core.StepFailed, Note: "Token budget exhausted at 92%"},
		},
		SubBeads: []core.SubBead{
			{ID: "bd-8b44.1", Title: "Split: dual-write window setup", Status: core.StepPending, Agent: core.AgentClaude, AutoSplit: true},
			{ID: "bd-8b44.2", Title: "Split: backfill v3 fields from v2 rows", Status: core.StepPending, Agent: core.AgentClaude, AutoSplit: true},
			{ID: "bd-8b44.3", Title: "Split: cutover + drop v2 columns", Status: core.StepPending, Agent: core.AgentClaude, AutoSplit: true},
		},
		Gates:        []core.Gate{},
		History:      history,
		Acceptance:   []core.Acceptance{},
		TokensUsed:   184000,
		TokensBudget: 200000,
		Requeued:     true,
		Reviewer:     nil,
		CreatedAt:    "Sun 23:40",
		OpenedAt:     "Sun 23:40",
		LastActivity: "Mon 06:30",
		Log:          []core.LogEntry{},
		Files:        []core.FileChange{},
		Blocks:       []string{},
		BlockedBy:    []string{},
	}
}

func bd3e80() core.Bead {
	reviewer := &core.Reviewer{Agent: core.AgentClaude, Comments: 2}
	history := []core.HistoryEvent{
		{At: "Sun 22:11", Kind: core.EvOpened, Actor: "claude", Note: "auto-created by bd-a1f2"},
		{At: "Sun 22:14", Kind: core.EvClaimed, Actor: "dispatcher", Agent: core.AgentCodex},
		{At: "Sun 22:14", Kind: core.EvStarted, Actor: "codex"},
		{At: "Mon 07:48", Kind: core.EvReview, Actor: "claude"},
		{At: "Mon 09:15", Kind: core.EvComment, Actor: "claude", Note: "requested deterministic clock test"},
		{At: "Mon 09:42", Kind: core.EvComment, Actor: "claude", Note: "second iteration looks good"},
	}
	return core.Bead{
		ID:       "bd-3e80",
		Title:    "Fix flaky test in payments/checkout.spec.ts",
		Desc:     "Race between stripe webhook stub and order-finalize. Add deterministic clock.",
		Type:     core.TypeBug,
		Column:   core.ColReview,
		Priority: 1,
		Labels:   []string{"flake", "payments", "test"},
		VCS:      core.VCSJJ,
		Branch:   "jj/bd-3e80-flaky-checkout",
		Ready:    true,
		Repo:     "main",
		Formula:  "bug-triage",
		Skills:   []string{"run-tests", "sentry"},
		Steps: []core.Step{
			{Agent: core.AgentCodex, Mode: core.ModePlan, Skills: []string{}, Status: core.StepDone},
			{Agent: core.AgentCodex, Mode: core.ModeApply, Skills: []string{}, Status: core.StepDone},
			{Agent: core.AgentClaude, Mode: core.ModeReview, Skills: []string{}, Status: core.StepActive, Note: "2 comments, awaiting human"},
			{Agent: core.AgentCodex, Mode: core.ModeApply, Skills: []string{}, Status: core.StepPending},
		},
		SubBeads: []core.SubBead{},
		Gates: []core.Gate{
			{Kind: core.GateHuman, Label: "2 review comments awaiting reply", Status: core.GateWaiting},
		},
		History: history,
		Acceptance: []core.Acceptance{
			{Text: "Test passes 1000 iterations without flake", Done: true},
			{Text: "Deterministic clock helper extracted", Done: true},
		},
		TokensUsed:   58440,
		TokensBudget: 150000,
		Reviewer:     reviewer,
		CreatedAt:    "Sun 22:11",
		OpenedAt:     "Sun 22:11",
		LastActivity: "Mon 09:42",
		Log:          []core.LogEntry{},
		Files: []core.FileChange{
			{Path: "test/payments/checkout.spec.ts", Status: core.FileModified, Adds: 22, Dels: 31},
			{Path: "test/helpers/clock.ts", Status: core.FileAdded, Adds: 41, Dels: 0},
		},
		Blocks:    []string{},
		BlockedBy: []string{},
	}
}

func bdD091() core.Bead {
	reviewer := &core.Reviewer{Agent: core.AgentGemini, Comments: 0}
	history := []core.HistoryEvent{
		{At: "Sun 17:30", Kind: core.EvOpened, Actor: "you@yours.dev"},
		{At: "Sun 18:02", Kind: core.EvClaimed, Actor: "dispatcher", Agent: core.AgentOpenCode},
		{At: "Sun 18:02", Kind: core.EvStarted, Actor: "opencode"},
		{At: "Sun 20:14", Kind: core.EvReview, Actor: "gemini", Note: "no comments"},
	}
	return core.Bead{
		ID:       "bd-d091",
		Title:    "Surface stale beads in dashboard",
		Desc:     "Beads with no status change in >7 days surface a \"stale\" pill in the backlog header.",
		Type:     core.TypeChore,
		Column:   core.ColReview,
		Priority: 3,
		Labels:   []string{"dashboard", "hygiene"},
		VCS:      core.VCSGit,
		Branch:   "jj/bd-d091-stale-beads",
		Ready:    true,
		Repo:     "main",
		Skills:   []string{},
		Steps: []core.Step{
			{Agent: core.AgentOpenCode, Mode: core.ModePlan, Skills: []string{}, Status: core.StepDone},
			{Agent: core.AgentOpenCode, Mode: core.ModeAgent, Skills: []string{}, Status: core.StepDone},
			{Agent: core.AgentGemini, Mode: core.ModeReview, Skills: []string{}, Status: core.StepActive, Note: "no comments — autoclose in 4h"},
		},
		SubBeads:     []core.SubBead{},
		Gates:        []core.Gate{},
		History:      history,
		Acceptance:   []core.Acceptance{},
		TokensUsed:   22410,
		TokensBudget: 80000,
		Reviewer:     reviewer,
		CreatedAt:    "Sun 18:02",
		OpenedAt:     "Sun 18:02",
		LastActivity: "Sun 20:14",
		Log:          []core.LogEntry{},
		Files:        []core.FileChange{},
		Blocks:       []string{},
		BlockedBy:    []string{},
	}
}

func bdB210() core.Bead {
	history := []core.HistoryEvent{
		{At: "Mon 07:55", Kind: core.EvOpened, Actor: "you@yours.dev"},
	}
	return core.Bead{
		ID:       "bd-b210",
		Title:    "Implement audit log for admin actions",
		Desc:     "Append-only audit table, RLS by tenant, surface in /admin/audit.",
		Type:     core.TypeFeature,
		Column:   core.ColBacklog,
		Priority: 2,
		Labels:   []string{"admin", "security"},
		VCS:      core.VCSJJ,
		Branch:   "",
		Ready:    true,
		Repo:     "main",
		Skills:   []string{},
		Steps: []core.Step{
			{Agent: core.AgentClaude, Mode: core.ModePlan, Skills: []string{"speckit"}, Status: core.StepPending},
			{Agent: core.AgentClaude, Mode: core.ModePlan, Skills: []string{}, Status: core.StepPending},
			{Agent: core.AgentClaude, Mode: core.ModeBuild, Skills: []string{}, Status: core.StepPending},
		},
		SubBeads:     []core.SubBead{},
		Gates:        []core.Gate{},
		History:      history,
		Acceptance:   []core.Acceptance{},
		TokensUsed:   0,
		TokensBudget: 300000,
		Reviewer:     nil,
		CreatedAt:    "Mon 07:55",
		OpenedAt:     "Mon 07:55",
		LastActivity: "Mon 07:55",
		Log:          []core.LogEntry{},
		Files:        []core.FileChange{},
		Blocks:       []string{},
		BlockedBy:    []string{},
	}
}

func bd4f12() core.Bead {
	history := []core.HistoryEvent{
		{At: "Fri 16:20", Kind: core.EvOpened, Actor: "you@yours.dev"},
	}
	return core.Bead{
		ID:       "bd-4f12",
		Title:    "Upgrade Tailwind 3 → 4 across packages",
		Desc:     "Codemod arbitrary values, replace removed plugins, regenerate tokens.",
		Type:     core.TypeChore,
		Column:   core.ColBacklog,
		Priority: 3,
		Labels:   []string{"deps", "design-system"},
		VCS:      core.VCSGit,
		Branch:   "",
		Ready:    true,
		Repo:     "frontend-repo",
		Skills:   []string{},
		Steps: []core.Step{
			{Agent: core.AgentGemini, Mode: core.ModePlan, Skills: []string{}, Status: core.StepPending},
			{Agent: core.AgentGemini, Mode: core.ModeAgent, Skills: []string{}, Status: core.StepPending},
			{Agent: core.AgentClaude, Mode: core.ModeReview, Skills: []string{}, Status: core.StepPending},
		},
		SubBeads:     []core.SubBead{},
		Gates:        []core.Gate{},
		History:      history,
		Acceptance:   []core.Acceptance{},
		TokensUsed:   0,
		TokensBudget: 200000,
		Reviewer:     nil,
		CreatedAt:    "Fri 16:20",
		OpenedAt:     "Fri 16:20",
		LastActivity: "Fri 16:20",
		Log:          []core.LogEntry{},
		Files:        []core.FileChange{},
		Blocks:       []string{},
		BlockedBy:    []string{},
	}
}

func bd2d55() core.Bead {
	history := []core.HistoryEvent{
		{At: "Fri 14:02", Kind: core.EvOpened, Actor: "you@yours.dev"},
	}
	return core.Bead{
		ID:       "bd-2d55",
		Title:    "Mobile share sheet for article reader",
		Desc:     "Native share on iOS/Android via Web Share API, fallback popover on desktop.",
		Type:     core.TypeFeature,
		Column:   core.ColBacklog,
		Priority: 3,
		Labels:   []string{"mobile", "reader"},
		VCS:      core.VCSGit,
		Branch:   "",
		Ready:    true,
		Repo:     "frontend-repo",
		Skills:   []string{"browser", "figma-read", "run-tests"},
		Steps: []core.Step{
			{Agent: core.AgentOpenCode, Mode: core.ModeAgent, Skills: []string{}, Status: core.StepPending},
			{Agent: core.AgentClaude, Mode: core.ModeReview, Skills: []string{}, Status: core.StepPending},
		},
		SubBeads:     []core.SubBead{},
		Gates:        []core.Gate{},
		History:      history,
		Acceptance:   []core.Acceptance{},
		TokensUsed:   0,
		TokensBudget: 100000,
		Reviewer:     nil,
		CreatedAt:    "Fri 14:02",
		OpenedAt:     "Fri 14:02",
		LastActivity: "Fri 14:02",
		Log:          []core.LogEntry{},
		Files:        []core.FileChange{},
		Blocks:       []string{},
		BlockedBy:    []string{},
	}
}

func bd7e21() core.Bead {
	history := []core.HistoryEvent{
		{At: "Mon 06:00", Kind: core.EvOpened, Actor: "gemini", Note: "discovered while running bd-c411"},
		{At: "Mon 06:11", Kind: core.EvBlocked, Actor: "dispatcher", Note: "waits on bd-7c0d"},
	}
	return core.Bead{
		ID:       "bd-7e21",
		Title:    "Reproduce intermittent 502 from /api/feed under load",
		Desc:     "Caller reports spiky 502s at ~2k RPM. Likely connection pool exhaustion.",
		Type:     core.TypeBug,
		Column:   core.ColBacklog,
		Priority: 1,
		Labels:   []string{"perf", "feed"},
		VCS:      core.VCSJJ,
		Branch:   "",
		Ready:    false,
		Repo:     "backend-repo",
		Skills:   []string{"sentry", "run-tests"},
		Steps: []core.Step{
			{Agent: core.AgentClaude, Mode: core.ModePlan, Skills: []string{}, Status: core.StepPending},
			{Agent: core.AgentClaude, Mode: core.ModeAgent, Skills: []string{}, Status: core.StepPending},
		},
		SubBeads:     []core.SubBead{},
		Gates:        []core.Gate{},
		History:      history,
		Acceptance:   []core.Acceptance{},
		TokensUsed:   0,
		TokensBudget: 150000,
		Reviewer:     nil,
		CreatedAt:    "Mon 06:11",
		OpenedAt:     "Mon 06:11",
		LastActivity: "Mon 06:11",
		Log:          []core.LogEntry{},
		Files:        []core.FileChange{},
		Blocks:       []string{},
		BlockedBy:    []string{"bd-7c0d"},
	}
}

func bd5e91() core.Bead {
	history := []core.HistoryEvent{
		{At: "Thu 13:30", Kind: core.EvOpened, Actor: "you@yours.dev"},
		{At: "Thu 13:42", Kind: core.EvClaimed, Actor: "dispatcher", Agent: core.AgentClaude},
		{At: "Thu 13:42", Kind: core.EvStarted, Actor: "claude"},
		{At: "Thu 15:58", Kind: core.EvReview, Actor: "codex"},
		{At: "Thu 17:42", Kind: core.EvClosed, Actor: "you@yours.dev", Note: "approved"},
	}
	return core.Bead{
		ID:       "bd-5e91",
		Title:    "Stream embeddings to Pinecone in batches",
		Desc:     "Replace blocking upserts with a 200-row stream, retry with jitter.",
		Type:     core.TypeTask,
		Column:   core.ColDone,
		Priority: 1,
		Labels:   []string{"pinecone", "embeddings"},
		VCS:      core.VCSJJ,
		Branch:   "jj/bd-5e91-pinecone-stream",
		Ready:    true,
		Repo:     "main",
		Skills:   []string{"run-tests"},
		Steps: []core.Step{
			{Agent: core.AgentClaude, Mode: core.ModePlan, Skills: []string{"speckit"}, Status: core.StepDone},
			{Agent: core.AgentClaude, Mode: core.ModeAgent, Skills: []string{}, Status: core.StepDone},
			{Agent: core.AgentCodex, Mode: core.ModeReview, Skills: []string{}, Status: core.StepDone},
			{Agent: core.AgentClaude, Mode: core.ModeAgent, Skills: []string{}, Status: core.StepDone},
		},
		SubBeads:     []core.SubBead{},
		Gates:        []core.Gate{},
		History:      history,
		Acceptance:   []core.Acceptance{},
		TokensUsed:   91220,
		TokensBudget: 150000,
		Reviewer:     nil,
		CreatedAt:    "Thu 13:30",
		OpenedAt:     "Thu 13:30",
		ClosedAt:     "Thu 17:42",
		LastActivity: "Thu 17:42",
		Log:          []core.LogEntry{},
		Files:        []core.FileChange{},
		Blocks:       []string{},
		BlockedBy:    []string{},
	}
}

func bd6a02() core.Bead {
	history := []core.HistoryEvent{
		{At: "Thu 09:10", Kind: core.EvOpened, Actor: "you@yours.dev"},
		{At: "Thu 09:18", Kind: core.EvClaimed, Actor: "dispatcher", Agent: core.AgentGemini},
		{At: "Thu 09:18", Kind: core.EvStarted, Actor: "gemini"},
		{At: "Thu 11:34", Kind: core.EvReview, Actor: "claude"},
		{At: "Thu 12:08", Kind: core.EvClosed, Actor: "you@yours.dev", Note: "shipped"},
	}
	return core.Bead{
		ID:       "bd-6a02",
		Title:    "Deprecate /v1 API surface",
		Desc:     "Add Sunset headers, log callers, prep migration doc.",
		Type:     core.TypeChore,
		Column:   core.ColDone,
		Priority: 2,
		Labels:   []string{"api", "sunset"},
		VCS:      core.VCSJJ,
		Branch:   "jj/bd-6a02-v1-sunset",
		Ready:    true,
		Repo:     "backend-repo",
		Skills:   []string{},
		Steps: []core.Step{
			{Agent: core.AgentGemini, Mode: core.ModePlan, Skills: []string{}, Status: core.StepDone},
			{Agent: core.AgentGemini, Mode: core.ModeAgent, Skills: []string{}, Status: core.StepDone},
			{Agent: core.AgentClaude, Mode: core.ModeReview, Skills: []string{}, Status: core.StepDone},
		},
		SubBeads:     []core.SubBead{},
		Gates:        []core.Gate{},
		History:      history,
		Acceptance:   []core.Acceptance{},
		TokensUsed:   34990,
		TokensBudget: 120000,
		Reviewer:     nil,
		CreatedAt:    "Thu 09:10",
		OpenedAt:     "Thu 09:10",
		ClosedAt:     "Thu 12:08",
		LastActivity: "Thu 12:08",
		Log:          []core.LogEntry{},
		Files:        []core.FileChange{},
		Blocks:       []string{},
		BlockedBy:    []string{},
	}
}

func bd4a11() core.Bead {
	history := []core.HistoryEvent{
		{At: "Wed 09:33", Kind: core.EvOpened, Actor: "you@yours.dev"},
		{At: "Wed 09:40", Kind: core.EvClaimed, Actor: "dispatcher", Agent: core.AgentCodex},
		{At: "Wed 09:40", Kind: core.EvStarted, Actor: "codex"},
		{At: "Wed 14:52", Kind: core.EvReview, Actor: "claude"},
		{At: "Wed 16:00", Kind: core.EvClosed, Actor: "you@yours.dev", Note: "approved"},
	}
	return core.Bead{
		ID:       "bd-4a11",
		Title:    "Strip PII from outbound webhooks",
		Desc:     "Redact email / phone / address before posting to subscriber endpoints.",
		Type:     core.TypeTask,
		Column:   core.ColDone,
		Priority: 1,
		Labels:   []string{"privacy", "webhooks"},
		VCS:      core.VCSJJ,
		Branch:   "jj/bd-4a11-webhook-pii",
		Ready:    true,
		Repo:     "backend-repo",
		Skills:   []string{},
		Steps: []core.Step{
			{Agent: core.AgentCodex, Mode: core.ModePlan, Skills: []string{}, Status: core.StepDone},
			{Agent: core.AgentCodex, Mode: core.ModeApply, Skills: []string{}, Status: core.StepDone},
			{Agent: core.AgentClaude, Mode: core.ModeReview, Skills: []string{}, Status: core.StepDone},
			{Agent: core.AgentCodex, Mode: core.ModeApply, Skills: []string{}, Status: core.StepDone},
		},
		SubBeads:     []core.SubBead{},
		Gates:        []core.Gate{},
		History:      history,
		Acceptance:   []core.Acceptance{},
		TokensUsed:   18220,
		TokensBudget: 80000,
		Reviewer:     nil,
		CreatedAt:    "Wed 09:33",
		OpenedAt:     "Wed 09:33",
		ClosedAt:     "Wed 16:00",
		LastActivity: "Wed 16:00",
		Log:          []core.LogEntry{},
		Files:        []core.FileChange{},
		Blocks:       []string{},
		BlockedBy:    []string{},
	}
}

// SeedProviders returns the 4 providers from the AGENTS array in prototype/data.jsx.
func SeedProviders() []core.Provider {
	return []core.Provider{
		{
			ID:       "claude",
			Name:     "Claude Code",
			Mono:     "CC",
			Color:    "#D97757",
			Parallel: 3,
			Kind:     "cli",
			Plan:     "Claude Max",
		},
		{
			ID:       "gemini",
			Name:     "Gemini CLI",
			Mono:     "GM",
			Color:    "#3B82F6",
			Parallel: 2,
			Kind:     "cli",
			Plan:     "Gemini Code Assist",
		},
		{
			ID:       "opencode",
			Name:     "OpenCode",
			Mono:     "OC",
			Color:    "#10B981",
			Parallel: 2,
			Kind:     "sdk",
			Plan:     "BYO model · routes to local Ollama",
		},
		{
			ID:       "codex",
			Name:     "Codex",
			Mono:     "CX",
			Color:    "#8B5CF6",
			Parallel: 1,
			Kind:     "cli",
			Plan:     "ChatGPT Pro",
		},
	}
}

// SeedCapacity returns the 4 capacity entries from prototype/data.jsx.
func SeedCapacity() []core.Capacity {
	return []core.Capacity{
		{Agent: core.AgentClaude, Running: 2, Queued: 3, Limit: 3},
		{Agent: core.AgentGemini, Running: 1, Queued: 2, Limit: 2},
		{Agent: core.AgentOpenCode, Running: 0, Queued: 1, Limit: 2},
		{Agent: core.AgentCodex, Running: 0, Queued: 0, Limit: 1},
	}
}

// SeedDolt returns the Dolt sync state from prototype/data.jsx.
func SeedDolt() core.DoltStatus {
	return core.DoltStatus{
		Branch:   "main",
		Remote:   "origin",
		Ahead:    0,
		Behind:   0,
		LastSync: "2m ago",
		Status:   "clean",
		Server:   "running",
		Port:     3306,
		Writers:  4,
	}
}
