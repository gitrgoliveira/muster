package tmux

import (
	"errors"
	"fmt"
	"testing"
)

// TestNewRealManager_WithExplicitBin verifies explicit bin path is used.
func TestNewRealManager_WithExplicitBin(t *testing.T) {
	m := NewRealManager("/usr/bin/tmux")
	if m == nil {
		t.Error("NewRealManager should not return nil")
	}
}

// TestBuildShellCmd verifies the env-wrapping behavior.
func TestBuildShellCmd_NoEnv(t *testing.T) {
	argv := []string{"sh", "-c", "echo hello"}
	got := buildShellCmd(nil, argv)
	if len(got) != len(argv) {
		t.Errorf("no-env: want len %d got %d", len(argv), len(got))
	}
}

func TestBuildShellCmd_WithEnv(t *testing.T) {
	env := []string{"FOO=bar", "BAZ=qux"}
	argv := []string{"sh", "-c", "echo $FOO"}
	got := buildShellCmd(env, argv)
	if got[0] != "env" {
		t.Errorf("first arg want env got %q", got[0])
	}
	if got[1] != "FOO=bar" || got[2] != "BAZ=qux" {
		t.Errorf("env args wrong: %v", got[1:3])
	}
	if got[3] != "sh" {
		t.Errorf("cmd[0] want sh got %q", got[3])
	}
}

// TestIsWrappedAttachUnavailable tests wrapping detection.
func TestIsWrappedAttachUnavailable_Wrapped(t *testing.T) {
	wrapped := fmt.Errorf("outer: %w", ErrAttachUnavailable)
	if !IsAttachUnavailable(wrapped) {
		t.Error("should detect ErrAttachUnavailable wrapped in another error")
	}
}

func TestIsWrappedAttachUnavailable_OtherError(t *testing.T) {
	other := errors.New("some other error")
	if IsAttachUnavailable(other) {
		t.Error("should not detect ErrAttachUnavailable in unrelated error")
	}
}

func TestIsWrappedAttachUnavailable_Nil(t *testing.T) {
	if IsAttachUnavailable(nil) {
		t.Error("should not detect ErrAttachUnavailable in nil")
	}
}
