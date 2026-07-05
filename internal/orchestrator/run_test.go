package orchestrator

import (
	"errors"
	"strings"
	"testing"

	"github.com/gitrgoliveira/muster/internal/core"
)

// TestGetAttach_ReservationWindow verifies GetAttach rejects a run that is
// StepActive but has not yet been assigned a tmux Session name — the window
// between Dispatch registering the reservation and the tmux session actually
// spawning. Without this check, RealManager.Attach never errors (it just
// string-concatenates the session name), so an empty Session would report
// Available: true with a bogus "tmux attach -t " command.
func TestGetAttach_ReservationWindow(t *testing.T) {
	o := New(Config{RepoMap: RepoMap{"mp": "/tmp/repo"}})

	run := &Run{BeadID: "mp-starting", State: core.StepActive, Session: ""}
	o.mu.Lock()
	o.registerRun(run)
	o.mu.Unlock()

	resp, err := o.GetAttach("mp-starting", 0)
	if err != nil {
		t.Fatalf("GetAttach: %v", err)
	}
	if resp.Available {
		t.Errorf("Available should be false while Session is empty, got command %q", resp.Command)
	}
}

// TestSendKeys_ReservationWindow is the SendKeys analogue of
// TestGetAttach_ReservationWindow: a StepActive run with no Session yet must
// be rejected with a clear error rather than forwarding an empty session name
// to the transport.
func TestSendKeys_ReservationWindow(t *testing.T) {
	o := New(Config{RepoMap: RepoMap{"mp": "/tmp/repo"}})

	run := &Run{BeadID: "mp-starting", State: core.StepActive, Session: ""}
	o.mu.Lock()
	o.registerRun(run)
	o.mu.Unlock()

	err := o.SendKeys("mp-starting", 0, "y\n")
	if err == nil {
		t.Fatal("SendKeys should reject a run with no Session yet")
	}
}

func TestRunRegistry_OneActivePerBead(t *testing.T) {
	o := New(Config{
		Adapters:  nil,
		Transport: nil,
		RepoMap:   RepoMap{"mp": "/tmp/repo"},
	})

	// Insert a run.
	run := &Run{
		BeadID: "mp-abc",
		State:  core.StepActive,
	}
	o.mu.Lock()
	o.registerRun(run)
	o.mu.Unlock()

	got := o.GetRun("mp-abc")
	if got == nil {
		t.Fatal("GetRun returned nil")
	}
	if got.BeadID != "mp-abc" {
		t.Errorf("BeadID want mp-abc got %q", got.BeadID)
	}

	// RunCount should be 1.
	if c := o.RunCount(); c != 1 {
		t.Errorf("RunCount want 1 got %d", c)
	}

	// Remove and verify gone.
	o.removeRun("mp-abc")
	if got := o.GetRun("mp-abc"); got != nil {
		t.Error("GetRun should return nil after removeRun")
	}
}

func TestResolvePermMode(t *testing.T) {
	o := New(Config{RepoMap: RepoMap{}})

	// No default, no request → error.
	_, err := o.resolvePermMode(core.ModeAgent, "")
	if err != ErrNoPermissionMode {
		t.Errorf("want ErrNoPermissionMode got %v", err)
	}

	// Request wins.
	pm, err := o.resolvePermMode(core.ModeAgent, core.PermAcceptEdits)
	if err != nil {
		t.Fatal(err)
	}
	if pm != core.PermAcceptEdits {
		t.Errorf("want acceptEdits got %q", pm)
	}

	// Default used when request is empty.
	o.defaultPermMode = core.PermDontAsk
	pm, err = o.resolvePermMode(core.ModeAgent, "")
	if err != nil {
		t.Fatal(err)
	}
	if pm != core.PermDontAsk {
		t.Errorf("want dontAsk got %q", pm)
	}

	// Invalid mode returns error.
	_, err = o.resolvePermMode(core.ModeAgent, "invalid-mode")
	if err == nil {
		t.Error("want error for invalid permission mode")
	}
}

// TestResolvePermMode_Plan verifies that core.ModePlan always resolves to
// core.PermPlan regardless of the requested/default permission mode — the
// claude adapter's Modes() table hardcodes "--permission-mode plan" for plan
// mode, discarding whatever value would otherwise be resolved. Without this
// carve-out, plan-mode dispatch would spuriously require a permissionMode
// (even with no default configured) that's never actually used.
func TestResolvePermMode_Plan(t *testing.T) {
	o := New(Config{RepoMap: RepoMap{}})

	// No default, no request, plan mode → still resolves (no ErrNoPermissionMode).
	pm, err := o.resolvePermMode(core.ModePlan, "")
	if err != nil {
		t.Fatalf("plan mode should not require permissionMode: %v", err)
	}
	if pm != core.PermPlan {
		t.Errorf("want PermPlan got %q", pm)
	}

	// Explicit "plan" request in plan mode is accepted.
	pm, err = o.resolvePermMode(core.ModePlan, core.PermPlan)
	if err != nil {
		t.Fatal(err)
	}
	if pm != core.PermPlan {
		t.Errorf("want PermPlan got %q", pm)
	}

	// A default permission mode configured for agent-mode dispatches must not
	// leak into plan mode's resolution.
	o.defaultPermMode = core.PermBypassPermissions
	pm, err = o.resolvePermMode(core.ModePlan, "")
	if err != nil {
		t.Fatal(err)
	}
	if pm != core.PermPlan {
		t.Errorf("want PermPlan got %q", pm)
	}

	// A conflicting explicit permissionMode in plan mode is rejected — it
	// would silently be ignored downstream, which is worse than an error.
	if _, err := o.resolvePermMode(core.ModePlan, core.PermAcceptEdits); err == nil {
		t.Error("want error for non-plan permissionMode requested with plan mode")
	}

	// Symmetric case: PermPlan requested with a NON-plan mode must be rejected
	// — accepting it would run plan mode while the request stays labelled agent.
	if _, err := o.resolvePermMode(core.ModeAgent, core.PermPlan); err == nil {
		t.Error("want error for permissionMode=plan requested with agent mode")
	}

	// And the same guard on the DEFAULT path: a configured default of "plan"
	// must not be silently applied to an agent-mode dispatch that omits it.
	o.defaultPermMode = core.PermPlan
	if _, err := o.resolvePermMode(core.ModeAgent, ""); err == nil {
		t.Error("want error for defaultPermMode=plan applied to agent mode")
	}
	// Plan-mode dispatch is unaffected — plan mode ignores the default entirely.
	if _, err := o.resolvePermMode(core.ModePlan, ""); err != nil {
		t.Errorf("plan mode with plan default should still resolve: %v", err)
	}
}

// ── Fix 1 (tri-review 6): launching sentinel — Advance/LoopBack blocked in launch window ──

// TestAdvance_LaunchingSentinel verifies that Advance is rejected when a run is
// in the launch window: State=StepActive, pendingAdvance=true (the launching
// sentinel set by admitOrEnqueue), cancel=nil. Without Fix 1, the nil cancel
// would be silently no-oped and Advance would return nil, dropping the advance.
func TestAdvance_LaunchingSentinel(t *testing.T) {
	o := New(Config{RepoMap: RepoMap{"mp": "/tmp/repo"}})

	chain := StepChain{
		{Name: "plan", PermissionMode: core.PermPlan},
		{Name: "build", PermissionMode: core.PermAcceptEdits},
	}
	// Simulate the mid-launch window: StepActive + pendingAdvance=true + cancel=nil.
	// This is the state set by admitOrEnqueue (Fix 1 target, tri-review 6).
	run := &Run{
		BeadID:         "mp-sentinel",
		State:          core.StepActive,
		pendingAdvance: true, // launching sentinel: blocks Advance/LoopBack until doLaunch arms cancel
		Chain:          &chain,
		// cancel is nil — doLaunch has not run yet
	}
	o.mu.Lock()
	o.registerRun(run)
	o.mu.Unlock()

	err := o.Advance("mp-sentinel")
	if !errors.Is(err, ErrStepOutOfRange) {
		t.Errorf("Advance during launch window: want ErrStepOutOfRange, got %v", err)
	}
	if err != nil && !strings.Contains(err.Error(), "transition in progress") {
		t.Errorf("error message want 'transition in progress', got %q", err.Error())
	}
}

// TestLoopBack_LaunchingSentinel is the LoopBack twin of TestAdvance_LaunchingSentinel.
func TestLoopBack_LaunchingSentinel(t *testing.T) {
	o := New(Config{RepoMap: RepoMap{"mp": "/tmp/repo"}})

	chain := StepChain{
		{Name: "plan", PermissionMode: core.PermPlan},
		{Name: "build", PermissionMode: core.PermAcceptEdits},
	}
	// Run at step 1 with the launching sentinel set (tri-review 6).
	run := &Run{
		BeadID:         "mp-lbsentinel",
		State:          core.StepActive,
		StepIdx:        1, // at step 1 so LoopBack(0) would otherwise be valid
		pendingAdvance: true,
		Chain:          &chain,
	}
	o.mu.Lock()
	o.registerRun(run)
	o.mu.Unlock()

	err := o.LoopBack("mp-lbsentinel", 0)
	if !errors.Is(err, ErrStepOutOfRange) {
		t.Errorf("LoopBack during launch window: want ErrStepOutOfRange, got %v", err)
	}
	if err != nil && !strings.Contains(err.Error(), "transition in progress") {
		t.Errorf("error message want 'transition in progress', got %q", err.Error())
	}
}

func TestRepoMap_Resolve(t *testing.T) {
	m := RepoMap{"mp": "/repos/mp", "bd": "/repos/bd"}

	path, err := m.Resolve("mp-abc")
	if err != nil {
		t.Fatal(err)
	}
	if path != "/repos/mp" {
		t.Errorf("want /repos/mp got %q", path)
	}

	_, err = m.Resolve("unknown-xyz")
	if err != ErrUnmappedPrefix {
		t.Errorf("want ErrUnmappedPrefix got %v", err)
	}
}

func TestPrefixOf(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"mp-abc", "mp"},
		{"bd-0001", "bd"},
		{"nohyphen", "nohyphen"},
		{"multi-part-id", "multi"},
	}
	for _, tt := range tests {
		if got := prefixOf(tt.input); got != tt.want {
			t.Errorf("prefixOf(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
