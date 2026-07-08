package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveMusterDir_FlagWins(t *testing.T) {
	t.Setenv(MusterDirEnv, "/env/should/lose")
	got := ResolveMusterDir("/flag/wins")
	if got != "/flag/wins" {
		t.Fatalf("flag should win: got %q", got)
	}
}

func TestResolveMusterDir_FlagCleaned(t *testing.T) {
	if got := ResolveMusterDir("/a/b/../c"); got != "/a/c" {
		t.Fatalf("flag not cleaned: got %q", got)
	}
}

func TestResolveMusterDir_EnvWhenNoFlag(t *testing.T) {
	t.Setenv(MusterDirEnv, "/env/dir")
	if got := ResolveMusterDir(""); got != "/env/dir" {
		t.Fatalf("env should be used: got %q", got)
	}
}

func TestResolveMusterDir_DefaultHome(t *testing.T) {
	// Ensure no env override leaks in.
	if err := os.Unsetenv(MusterDirEnv); err != nil {
		t.Fatal(err)
	}
	got := ResolveMusterDir("")
	if home, err := os.UserHomeDir(); err == nil {
		want := filepath.Join(home, ".muster")
		if got != want {
			t.Fatalf("default: got %q want %q", got, want)
		}
	} else if got == "" {
		t.Fatal("expected a non-empty fallback dir")
	}
}
