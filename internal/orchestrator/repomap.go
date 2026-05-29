package orchestrator

import "strings"

// prefixOf extracts the prefix from a bead ID (everything before the first '-').
// For "mp-abc" the prefix is "mp"; for "bd-0001" it is "bd".
func prefixOf(beadID string) string {
	idx := strings.IndexByte(beadID, '-')
	if idx < 0 {
		return beadID
	}
	return beadID[:idx]
}
