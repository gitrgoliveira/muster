package orchestrator

import "strings"

// prefixOf extracts the prefix from a bead ID (everything before the first '-').
// For "mp-abc" the prefix is "mp"; for "bd-0001" it is "bd".
func prefixOf(beadID string) string {
	// strings.Cut returns the whole string as `prefix` when '-' is absent,
	// which is exactly the desired fallback.
	prefix, _, _ := strings.Cut(beadID, "-")
	return prefix
}
