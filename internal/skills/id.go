package skills

import (
	"fmt"
	"regexp"
	"strings"
)

// maxIDLen bounds a skill id defensively (ids become filenames and label
// suffixes).
const maxIDLen = 128

// idPattern is the sole accepted shape for a skill id: starts alphanumeric,
// then lowercase alphanumerics plus `.`, `_`, `-`. No path separators, no
// leading dot, no uppercase. Because ids flow into `<musterDir>/skills/<id>.md`
// and `skill:<id>` labels, this is a security gate, not a style choice.
var idPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)

// ValidateID is the single gate every skill id must pass before it is used in a
// filesystem path or label resolution. It rejects the empty string, path
// separators, `.`/`..` traversal, and anything that could escape
// <musterDir>/skills/. Returns nil for a safe id, or a descriptive error.
//
// A caller resolving a `skill:<id>` label treats a non-nil error as
// "ignore-with-warning"; a caller importing a skill treats it as a typed
// SKILL_INVALID_ID error. Neither may proceed to touch the filesystem with an
// invalid id.
func ValidateID(id string) error {
	switch {
	case id == "":
		return fmt.Errorf("skill id is empty: %w", ErrInvalidID)
	case len(id) > maxIDLen:
		return fmt.Errorf("skill id %q exceeds %d chars: %w", id, maxIDLen, ErrInvalidID)
	case id == "." || id == "..":
		return fmt.Errorf("skill id %q is a path traversal: %w", id, ErrInvalidID)
	case strings.Contains(id, ".."):
		return fmt.Errorf("skill id %q contains '..': %w", id, ErrInvalidID)
	case strings.ContainsAny(id, `/\`):
		return fmt.Errorf("skill id %q contains a path separator: %w", id, ErrInvalidID)
	case !idPattern.MatchString(id):
		return fmt.Errorf("skill id %q must match %s: %w", id, idPattern.String(), ErrInvalidID)
	}
	return nil
}
