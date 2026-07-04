package orchestrator_test

import (
	"testing"

	"github.com/gitrgoliveira/muster/internal/orchestrator"
)

// ── T010: QuotaUsage — compile-only type definition check ────────────────────

// TestQuotaUsage_ZeroValueSane verifies that QuotaUsage has sane zero values.
// Known=false means "no usage data" — the correct initial state before US5
// wires the on-disk reader. All numeric fields are zero.
func TestQuotaUsage_ZeroValueSane(t *testing.T) {
	var q orchestrator.QuotaUsage

	if q.Known {
		t.Error("QuotaUsage.Known zero value: want false")
	}
	if q.InputTokens != 0 {
		t.Errorf("QuotaUsage.InputTokens zero value: want 0, got %d", q.InputTokens)
	}
	if q.OutputTokens != 0 {
		t.Errorf("QuotaUsage.OutputTokens zero value: want 0, got %d", q.OutputTokens)
	}
	if q.CostUSD != 0 {
		t.Errorf("QuotaUsage.CostUSD zero value: want 0, got %f", q.CostUSD)
	}

	// Exercise all fields to confirm they compile.
	_ = orchestrator.QuotaUsage{
		Known:        true,
		InputTokens:  1000,
		OutputTokens: 500,
		CostUSD:      0.05,
	}
}
