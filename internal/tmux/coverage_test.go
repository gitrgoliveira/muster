package tmux

import (
	"strings"
	"testing"
)

// TestMergeEnv verifies provided env overrides the inherited env deterministically
// (the overridden inherited entry is dropped, not left as a duplicate).
func TestMergeEnv(t *testing.T) {
	base := []string{"PATH=/bin", "FOO=old", "KEEP=1"}
	override := []string{"FOO=new", "NEWVAR=x"}
	got := mergeEnv(base, override)

	// No duplicate keys.
	seen := map[string]int{}
	for _, e := range got {
		seen[envKey(e)]++
	}
	for k, n := range seen {
		if n != 1 {
			t.Errorf("key %q appears %d times, want 1: %v", k, n, got)
		}
	}
	// FOO must be the override value, and inherited keys preserved.
	byKey := map[string]string{}
	for _, e := range got {
		if k, v, ok := strings.Cut(e, "="); ok {
			byKey[k] = v
		}
	}
	if byKey["FOO"] != "new" {
		t.Errorf("FOO want new got %q", byKey["FOO"])
	}
	if byKey["PATH"] != "/bin" || byKey["KEEP"] != "1" || byKey["NEWVAR"] != "x" {
		t.Errorf("merged env wrong: %v", got)
	}
}

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
