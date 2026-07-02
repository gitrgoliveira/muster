package core

import (
	"regexp"
	"strings"

	"github.com/google/uuid"
)

// NewBeadID generates "bd-XXXX" where XXXX is the first 4 hex chars (lowercase)
// of a UUIDv4. Callers must check store-level uniqueness and retry on collision.
func NewBeadID() string {
	return "bd-" + strings.ToLower(uuid.NewString()[:4])
}

// validBeadIDPattern is the canonical, allow-list format for a bead ID: a
// lowercase alpha prefix followed by one or more hyphen-separated alphanumeric
// segments.
//
// The tail is one-OR-MORE hyphen segments (the `+`), not exactly one — that
// breadth is deliberate, not cosmetic: a single segment like "mp-kbj" is valid,
// but real bd IDs also include molecule beads whose IDs carry an extra segment
// (e.g. "mp-mol-4gl"), so a single-hyphen-only pattern (^[a-z]+-[0-9a-z]+$)
// would reject legitimate, bd-generated IDs. Examples that MUST pass: mp-kbj,
// bd-0000, muster-xyz, mp-mol-4gl, mp-e2e-01. Examples that MUST fail:
// "notanid" (no hyphen), "bad..id" (dots), "mp-" (empty segment), "MP-abc"
// (uppercase).
var validBeadIDPattern = regexp.MustCompile(`^[a-z]+(-[0-9a-z]+)+$`)

// ValidBeadID reports whether id matches the canonical bead-ID format. This is
// the single source of truth shared by every layer that must trust a bead ID —
// the HTTP handler's request-path allow-list and the orchestrator's
// recovery-time validation of user-controllable tmux session names — so those
// checks can never drift apart.
func ValidBeadID(id string) bool {
	return validBeadIDPattern.MatchString(id)
}
