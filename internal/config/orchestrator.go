package config

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// RepoMap maps bead-ID prefixes (e.g. "mp") to absolute repo paths.
// Populated from repeatable --repo prefix=path flags and MUSTER_REPO env.
type RepoMap map[string]string

// ParseRepoFlag parses a single "prefix=path" string into the RepoMap.
// Returns an error if the format is invalid.
func ParseRepoFlag(m RepoMap, val string) error {
	idx := strings.IndexByte(val, '=')
	if idx <= 0 {
		return fmt.Errorf("invalid --repo value %q: expected prefix=path", val)
	}
	prefix := val[:idx]
	path := val[idx+1:]
	if path == "" {
		return fmt.Errorf("invalid --repo value %q: path is empty", val)
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("--repo %q: cannot resolve path: %w", val, err)
	}
	m[prefix] = abs
	return nil
}

// ParseRepoEnv parses the MUSTER_REPO environment variable (comma-separated
// "prefix=path" pairs) into the RepoMap.
func ParseRepoEnv(m RepoMap, envVal string) error {
	if envVal == "" {
		return nil
	}
	for _, entry := range strings.Split(envVal, ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		if err := ParseRepoFlag(m, entry); err != nil {
			return fmt.Errorf("MUSTER_REPO: %w", err)
		}
	}
	return nil
}

// DefaultWorktreesDir returns the platform-appropriate default worktrees
// directory when --worktrees-dir is not specified.
func DefaultWorktreesDir() string {
	// Use os.TempDir() as the base so tests don't need special perms.
	tmp := os.TempDir()
	if runtime.GOOS == "darwin" || runtime.GOOS == "linux" {
		// Prefer a stable path under the user's home if available.
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, ".muster", "worktrees")
		}
	}
	return filepath.Join(tmp, "muster", "worktrees")
}
