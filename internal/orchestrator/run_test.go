package orchestrator

import (
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
	_, err := o.resolvePermMode("")
	if err != ErrNoPermissionMode {
		t.Errorf("want ErrNoPermissionMode got %v", err)
	}

	// Request wins.
	pm, err := o.resolvePermMode(core.PermAcceptEdits)
	if err != nil {
		t.Fatal(err)
	}
	if pm != core.PermAcceptEdits {
		t.Errorf("want acceptEdits got %q", pm)
	}

	// Default used when request is empty.
	o.defaultPermMode = core.PermDontAsk
	pm, err = o.resolvePermMode("")
	if err != nil {
		t.Fatal(err)
	}
	if pm != core.PermDontAsk {
		t.Errorf("want dontAsk got %q", pm)
	}

	// Invalid mode returns error.
	_, err = o.resolvePermMode("invalid-mode")
	if err == nil {
		t.Error("want error for invalid permission mode")
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
