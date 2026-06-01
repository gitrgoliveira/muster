package tmux_test

import (
	"errors"
	"io"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/gitrgoliveira/muster/internal/tmux"
)

func TestFallbackManager_Spawn_And_Pipe(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell commands not tested on Windows")
	}

	f := tmux.NewFallbackManager()
	sess, err := f.Spawn("muster/mp-fb/0/0", t.TempDir(), nil, []string{"sh", "-c", "echo hello"})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	if sess.Name != "muster/mp-fb/0/0" {
		t.Errorf("Session.Name want muster/mp-fb/0/0 got %q", sess.Name)
	}

	pipe, err := f.Pipe("muster/mp-fb/0/0")
	if err != nil {
		t.Fatalf("Pipe: %v", err)
	}
	defer pipe.Close()

	// Read output.
	var buf strings.Builder
	readDone := make(chan struct{})
	go func() {
		defer close(readDone)
		io.Copy(&buf, pipe) //nolint:errcheck
	}()

	select {
	case <-readDone:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout reading from pipe")
	}
	if !strings.Contains(buf.String(), "hello") {
		t.Errorf("pipe output want 'hello', got %q", buf.String())
	}
}

func TestFallbackManager_DeadStatus_AfterExit(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell commands not tested on Windows")
	}

	f := tmux.NewFallbackManager()
	_, err := f.Spawn("muster/mp-dead/0/0", t.TempDir(), nil, []string{"sh", "-c", "exit 42"})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}

	// Wait for process to exit.
	deadline := time.Now().Add(5 * time.Second)
	var code int
	var dead bool
	for time.Now().Before(deadline) {
		code, dead, err = f.DeadStatus("muster/mp-dead/0/0")
		if err != nil {
			t.Fatalf("DeadStatus error: %v", err)
		}
		if dead {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !dead {
		t.Fatal("process should have exited")
	}
	if code != 42 {
		t.Errorf("exit code want 42 got %d", code)
	}
}

func TestFallbackManager_DeadStatus_UnknownSession(t *testing.T) {
	// An unknown session must report dead with a non-zero code so the
	// orchestrator does not mark the run as a successful (exit-0) completion.
	// Mirrors RealManager.DeadStatus's "no numeric exit status" sentinel.
	f := tmux.NewFallbackManager()
	code, dead, err := f.DeadStatus("muster/never-spawned/0/0")
	if err != nil {
		t.Fatalf("DeadStatus error: %v", err)
	}
	if !dead {
		t.Error("unknown session should be reported as dead")
	}
	if code == 0 {
		t.Errorf("unknown session must NOT report exit code 0 (looks like success); got %d", code)
	}
}

func TestFallbackManager_Attach_ReturnsUnavailable(t *testing.T) {
	f := tmux.NewFallbackManager()
	_, err := f.Attach("muster/mp-test/0/0")
	if !errors.Is(err, tmux.ErrAttachUnavailable) {
		t.Errorf("Attach want ErrAttachUnavailable got %v", err)
	}
}

func TestFallbackManager_Send_ReturnsUnavailable(t *testing.T) {
	f := tmux.NewFallbackManager()
	err := f.Send("muster/mp-test/0/0", "y\n")
	if !errors.Is(err, tmux.ErrAttachUnavailable) {
		t.Errorf("Send want ErrAttachUnavailable got %v", err)
	}
}

func TestFallbackManager_Capture_ReturnsUnavailable(t *testing.T) {
	f := tmux.NewFallbackManager()
	_, err := f.Capture("muster/mp-test/0/0", false)
	if err != tmux.ErrAttachUnavailable {
		t.Errorf("Capture want ErrAttachUnavailable got %v", err)
	}
}

func TestFallbackManager_List_Empty(t *testing.T) {
	f := tmux.NewFallbackManager()
	sessions, err := f.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(sessions) != 0 {
		t.Errorf("List want empty, got %v", sessions)
	}
}

func TestFallbackManager_Detect_Error(t *testing.T) {
	f := tmux.NewFallbackManager()
	_, err := f.Detect()
	if err == nil {
		t.Error("Detect should return error for fallback manager")
	}
}

func TestFallbackManager_Kill_NoSession(t *testing.T) {
	f := tmux.NewFallbackManager()
	// Kill a non-existent session should not error.
	err := f.Kill("muster/mp-nonexistent/0/0")
	if err != nil {
		t.Errorf("Kill nonexistent: want nil got %v", err)
	}
}

func TestFallbackManager_Kill_ExistingSession(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell commands not tested on Windows")
	}
	f := tmux.NewFallbackManager()
	// Spawn a long-running process.
	_, err := f.Spawn("muster/mp-kill/0/0", t.TempDir(), nil, []string{"sh", "-c", "sleep 30"})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	// Kill it.
	err = f.Kill("muster/mp-kill/0/0")
	if err != nil {
		t.Errorf("Kill running session: %v", err)
	}
}

func TestIsAttachUnavailable_True(t *testing.T) {
	if !tmux.IsAttachUnavailable(tmux.ErrAttachUnavailable) {
		t.Error("IsAttachUnavailable(ErrAttachUnavailable) should be true")
	}
}

func TestIsAttachUnavailable_False(t *testing.T) {
	if tmux.IsAttachUnavailable(nil) {
		t.Error("IsAttachUnavailable(nil) should be false")
	}
}
