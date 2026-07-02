package config

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
)

// repoPrefixPattern is the accepted form of a --repo/MUSTER_REPO prefix. It
// mirrors the prefix half of a bead ID (core.ValidBeadID: lowercase alpha
// before the first hyphen), so a typo like "Mp" or "mp-foo" fails at startup
// rather than silently never matching any bead at dispatch time.
var repoPrefixPattern = regexp.MustCompile(`^[a-z]+$`)

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
	// Trim surrounding whitespace so entries like "mp = /path" or a
	// comma-separated MUSTER_REPO list with spaces after commas work.
	prefix := strings.TrimSpace(val[:idx])
	if !repoPrefixPattern.MatchString(prefix) {
		return fmt.Errorf("invalid --repo value %q: prefix %q must be lowercase letters only (it is matched against bead-ID prefixes)", val, prefix)
	}
	path := strings.TrimSpace(val[idx+1:])
	if path == "" {
		return fmt.Errorf("invalid --repo value %q: path is empty", val)
	}
	// Expand a leading "~" (followed by a path separator, or bare "~") to the
	// user's home dir. A shell won't expand it in `--repo mp=~/x` (the ~ isn't
	// at the start of the word), and MUSTER_REPO never passes through a shell,
	// so a literal "~/..." would otherwise be resolved relative to the cwd by
	// filepath.Abs and silently mis-map the repo. Accept both "/" and the OS
	// separator so Windows "~\repos" works too. ("~otheruser" is intentionally
	// not expanded — it's uncommon and not portably resolvable.)
	if path == "~" || (len(path) > 1 && path[0] == '~' && (path[1] == '/' || path[1] == os.PathSeparator)) {
		home, herr := os.UserHomeDir()
		if herr != nil {
			return fmt.Errorf("--repo %q: cannot expand ~: %w", val, herr)
		}
		// Strip the "~" and any leading separator(s) so we join a clean relative
		// remainder onto home. Bare "~" -> home.
		rest := strings.TrimLeft(path[1:], `/\`)
		path = filepath.Join(home, rest)
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
//
// Behavior: on darwin/linux, prefer a stable path under the user's home
// (`~/.muster/worktrees`); fall back to `<os.TempDir()>/muster/worktrees`
// only when the home directory is unavailable or on other platforms. The
// temp-dir fallback also keeps tests usable on hosts where the test runner
// lacks a real $HOME.
func DefaultWorktreesDir() string {
	if runtime.GOOS == "darwin" || runtime.GOOS == "linux" {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, ".muster", "worktrees")
		}
	}
	return filepath.Join(os.TempDir(), "muster", "worktrees")
}
