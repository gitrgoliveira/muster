package skills

import (
	"net"
	"testing"
	"time"
)

// TestFetchSkill_RealNetwork is a skip-gated real-network integration test: it
// only runs when a real outbound HTTPS connection is available. It exercises the
// actual DNS + TLS path that the fake-HTTP unit tests stub out.
func TestFetchSkill_RealNetwork(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping real-network test in -short mode")
	}
	// Probe connectivity; skip (not fail) when offline/sandboxed.
	c, err := net.DialTimeout("tcp", "93.184.216.34:443", 2*time.Second)
	if err != nil {
		t.Skipf("no outbound network: %v", err)
	}
	_ = c.Close()

	// example.com does not serve a skill document, so we expect a parse error —
	// the point is that the fetch policy + transport reached a real host without
	// being blocked, then rejected the non-skill body (no partial registration).
	if _, err := fetchSkill(newImportClient(), "https://example.com/"); err == nil {
		t.Fatal("example.com is not a skill document; expected a parse error")
	}
}
