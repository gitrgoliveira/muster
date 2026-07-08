package config

import (
	"os"
	"path/filepath"
	"runtime"
)

// MusterDirEnv is the environment variable that overrides the resolved muster
// operating-config directory (constitution, imported skills, primed memories).
const MusterDirEnv = "MUSTER_DIR"

// ResolveMusterDir resolves the base directory for muster's own operating
// config — the constitution (`<dir>/constitution.md`), imported skills
// (`<dir>/skills/`), and primed-memory snapshots (`<dir>/primed/`). This is
// muster's own state, not beads issue state (Constitution II): local files
// only, never Dolt, never through `bd`.
//
// Precedence: the explicit flag value (--muster-dir) > the MUSTER_DIR env var >
// the platform default `~/.muster` (mirroring DefaultWorktreesDir, whose
// `~/.muster/worktrees` establishes the `~/.muster` precedent). The temp-dir
// fallback keeps tests usable on hosts without a real $HOME.
func ResolveMusterDir(flagVal string) string {
	if flagVal != "" {
		return filepath.Clean(flagVal)
	}
	if env := os.Getenv(MusterDirEnv); env != "" {
		return filepath.Clean(env)
	}
	if runtime.GOOS == "darwin" || runtime.GOOS == "linux" {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, ".muster")
		}
	}
	return filepath.Join(os.TempDir(), "muster")
}
