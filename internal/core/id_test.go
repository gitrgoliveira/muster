package core

import (
	"regexp"
	"testing"
)

var beadIDPattern = regexp.MustCompile(`^bd-[0-9a-f]{4}$`)

func TestNewBeadID_Format(t *testing.T) {
	for i := 0; i < 1000; i++ {
		id := NewBeadID()
		if !beadIDPattern.MatchString(id) {
			t.Errorf("iteration %d: id %q does not match pattern ^bd-[0-9a-f]{4}$", i, id)
		}
	}
}

func TestNewBeadID_Uniqueness(t *testing.T) {
	const n = 10000
	seen := make(map[string]struct{}, n)
	collisions := 0
	for i := 0; i < n; i++ {
		id := NewBeadID()
		if _, exists := seen[id]; exists {
			collisions++
		}
		seen[id] = struct{}{}
	}
	// 4-hex-char space (65536 values): birthday paradox gives ~7.3% collision at 10k samples.
	// Store layer retries on collision; this test verifies the ID is not deterministic/constant.
	rate := float64(collisions) / float64(n)
	if rate >= 0.15 {
		t.Errorf("collision rate %.4f (%d/%d) exceeds 15%% threshold — ID generation may be broken", rate, collisions, n)
	}
}
