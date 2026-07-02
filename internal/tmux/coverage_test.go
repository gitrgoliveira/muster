package tmux

import (
	"testing"
)

// TestNewRealManager_WithExplicitBin verifies explicit bin path is used.
func TestNewRealManager_WithExplicitBin(t *testing.T) {
	m := NewRealManager("/usr/bin/tmux")
	if m == nil {
		t.Error("NewRealManager should not return nil")
	}
}

// TestBuildEnvArgv verifies the env-wrapping behavior.
func TestBuildEnvArgv_NoEnv(t *testing.T) {
	argv := []string{"sh", "-c", "echo hello"}
	got := buildEnvArgv(nil, argv)
	if len(got) != len(argv) {
		t.Errorf("no-env: want len %d got %d", len(argv), len(got))
	}
}

func TestBuildEnvArgv_WithEnv(t *testing.T) {
	env := []string{"FOO=bar", "BAZ=qux"}
	argv := []string{"sh", "-c", "echo $FOO"}
	got := buildEnvArgv(env, argv)
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
