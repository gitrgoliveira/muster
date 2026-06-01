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
// Returns an error if the name does not match the expected format.
func ParseSessionName(name string) (beadID string, stepIdx, loop int, err error) {
	if !strings.HasPrefix(name, sessionPrefix) {
		return "", 0, 0, fmt.Errorf("session name %q does not start with %q", name, sessionPrefix)
	}
	rest := name[len(sessionPrefix):]
	// rest = "<beadID>/<stepIdx>/<loop>". The beadID may itself contain
	// slashes (any leading segments before the trailing stepIdx/loop pair are
	// rejoined into beadID), so split from the right to peel off the two
	// trailing integers and treat everything left of them as the bead ID.
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
