package tmux

import (
	"fmt"
	"strconv"
	"strings"
)

const sessionPrefix = "muster/"

// SessionName returns the canonical tmux session name for a bead run.
// Format: "muster/<beadID>/<stepIdx>/<loop>"
// Slash-containing names are accepted by tmux (verified in research spike).
func SessionName(beadID string, stepIdx, loop int) string {
	return fmt.Sprintf("%s%s/%d/%d", sessionPrefix, beadID, stepIdx, loop)
}

// ParseSessionName parses a tmux session name produced by SessionName.
//
// It enforces the structural shape only: the muster/ prefix must be present,
// there must be at least three slash-separated segments, and the trailing two
// (stepIdx, loop) must parse as integers. It does NOT reject values SessionName
// would never emit — negative or non-canonical integers (parsed via
// strconv.Atoi) and bead IDs that wouldn't pass core.ValidBeadID both parse
// successfully. Callers needing those guarantees validate separately: recovery
// runs the bead ID through core.ValidBeadID and rejects/kills sessions whose
// step or loop index isn't the M2-supported 0. Returns an error only when the
// structural shape above is violated.
func ParseSessionName(name string) (beadID string, stepIdx, loop int, err error) {
	if !strings.HasPrefix(name, sessionPrefix) {
		return "", 0, 0, fmt.Errorf("session name %q does not start with %q", name, sessionPrefix)
	}
	rest := name[len(sessionPrefix):]
	// rest = "<beadID>/<stepIdx>/<loop>". A valid M2 bead ID never contains a
	// slash (core.ValidBeadID forbids it), but we still peel the two trailing
	// integers off the RIGHT and treat everything before them as the bead ID.
	// That way a malformed/stray session name carrying extra slashes is captured
	// whole rather than silently truncated, so the caller's core.ValidBeadID
	// check rejects it (recovery kills such a session) instead of adopting a
	// bogus, partial ID.
	parts := strings.Split(rest, "/")
	if len(parts) < 3 {
		return "", 0, 0, fmt.Errorf("session name %q: expected muster/<bead>/<step>/<loop>", name)
	}
	// loop is the last part, stepIdx is the second-to-last, beadID is everything before.
	loopStr := parts[len(parts)-1]
	stepStr := parts[len(parts)-2]
	beadParts := parts[:len(parts)-2]

	loop, err = strconv.Atoi(loopStr)
	if err != nil {
		return "", 0, 0, fmt.Errorf("session name %q: invalid loop %q: %w", name, loopStr, err)
	}
	stepIdx, err = strconv.Atoi(stepStr)
	if err != nil {
		return "", 0, 0, fmt.Errorf("session name %q: invalid stepIdx %q: %w", name, stepStr, err)
	}
	beadID = strings.Join(beadParts, "/")
	if beadID == "" {
		return "", 0, 0, fmt.Errorf("session name %q: beadID is empty", name)
	}
	return beadID, stepIdx, loop, nil
}

// IsMusterSession returns true if the session name starts with "muster/".
func IsMusterSession(name string) bool {
	return strings.HasPrefix(name, sessionPrefix)
}
