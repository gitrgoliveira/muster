package wt

import (
	"errors"
	"path/filepath"
	"strings"
)

// SafeRelPath validates that path is safe for use as a worktree-relative file
// path in HTTP query parameters (FR-007). It is the sole validator the /diff
// handler applies. Validation is format-only: it rejects absolute paths,
// non-local paths (containing ".." or reserved names), and paths beginning with
// "-" (which a VCS could read as an option), and returns the cleaned form. It
// does NOT resolve symlinks or walk the filesystem — the git/jj backends pass
// the path after a "--" terminator and skip untracked symlinks
// (untrackedDiffSafe) so a request can't read or escape outside the worktree.
//
// An empty path is allowed (means "whole worktree").
func SafeRelPath(path string) (string, error) {
	if path == "" {
		return "", nil
	}
	if filepath.IsAbs(path) {
		return "", errors.New("wt: path must be relative, not absolute")
	}
	if !filepath.IsLocal(path) {
		return "", errors.New("wt: path is not local (contains '..' or absolute component)")
	}
	cleaned := filepath.Clean(path)
	// Reject a leading "-": the VCS backends pass the path as an argv element, so
	// "--help"/"-x" could be read as an option (argument injection). git uses a
	// "--" terminator and jj now does too, but reject here as defense in depth.
	if strings.HasPrefix(cleaned, "-") {
		return "", errors.New("wt: path must not start with '-'")
	}
	return cleaned, nil
}

// safeRelPath validates that path is a safe worktree-relative path: not
// absolute, not containing "..", not escaping the worktree boundary.
//
// It accepts an empty path (meaning "whole worktree") and returns it as-is.
// For non-empty paths it:
//   - Rejects absolute paths (filepath.IsAbs).
//   - Rejects paths that are not local (filepath.IsLocal) — catches "..",
//     empty components, and OS-specific reserved names. A "." / leading "./"
//     is normalized away (allowed), not rejected.
//   - Rejects paths that resolve (via filepath.Join + filepath.Clean) outside
//     worktree after evaluation.
//
// Returns the cleaned worktree-relative path on success, or a descriptive
// error on rejection. The returned path is suitable for passing to git/jj.
func safeRelPath(worktree, path string) (string, error) {
	if path == "" {
		return "", nil
	}

	// Reject absolute paths immediately.
	if filepath.IsAbs(path) {
		return "", errors.New("wt: path must be relative, not absolute")
	}

	// filepath.IsLocal rejects "..", paths with a leading "/" (not possible
	// after IsAbs check but defensive), empty strings, and OS-reserved names on
	// Windows ("CON", etc.). It does NOT traverse — it inspects the string only.
	if !filepath.IsLocal(path) {
		return "", errors.New("wt: path is not local (contains '..' or absolute component)")
	}

	// Resolve the full path and confirm it stays inside worktree.
	// filepath.Clean eliminates redundant separators, "." components, etc.
	full := filepath.Clean(filepath.Join(worktree, path))

	// The cleaned worktree dir (no trailing slash).
	base := filepath.Clean(worktree)

	// The resolved path must equal base or be a child of base. Use HasPrefix
	// with the OS separator appended to base to prevent a prefix like
	// "/tmp/foo" matching "/tmp/foobar/...".
	if full != base && !strings.HasPrefix(full, base+string(filepath.Separator)) {
		return "", errors.New("wt: path escapes the worktree boundary")
	}

	// Return the worktree-relative cleaned path.
	rel, err := filepath.Rel(base, full)
	if err != nil {
		// Shouldn't happen after the boundary check above, but guard anyway.
		return "", errors.New("wt: cannot relativize path")
	}
	return rel, nil
}
