package store_test

import (
	"testing"

	"github.com/gitrgoliveira/muster/internal/core"
	"github.com/gitrgoliveira/muster/internal/store"
)

func TestSeedBeads_Count(t *testing.T) {
	beads := store.SeedBeads()
	if len(beads) != 14 {
		t.Errorf("expected 14 beads, got %d", len(beads))
	}
}

func TestSeedProviders_Count(t *testing.T) {
	providers := store.SeedProviders()
	if len(providers) != 4 {
		t.Errorf("expected 4 providers, got %d", len(providers))
	}
}

func TestSeedCapacity_Count(t *testing.T) {
	capacity := store.SeedCapacity()
	if len(capacity) != 4 {
		t.Errorf("expected 4 capacity entries, got %d", len(capacity))
	}
}

func TestSeedDolt_NonZero(t *testing.T) {
	d := store.SeedDolt()
	if d.Branch == "" {
		t.Error("SeedDolt().Branch must not be empty")
	}
	if d.Status == "" {
		t.Error("SeedDolt().Status must not be empty")
	}
}

func TestSeedBeads_HaveValidEnums(t *testing.T) {
	for _, b := range store.SeedBeads() {
		if !b.Column.Valid() {
			t.Errorf("bead %s: invalid Column %q", b.ID, b.Column)
		}
		if !b.Type.Valid() {
			t.Errorf("bead %s: invalid Type %q", b.ID, b.Type)
		}
		if !b.Priority.Valid() {
			t.Errorf("bead %s: invalid Priority %d", b.ID, b.Priority)
		}
	}
}

func TestSeedBeads_DerivedFields(t *testing.T) {
	for _, b := range store.SeedBeads() {
		wantEstimate := core.DeriveEstimate(b.TokensBudget)
		if b.Estimate != wantEstimate {
			t.Errorf("bead %s: Estimate = %q, want %q (budget=%d)", b.ID, b.Estimate, wantEstimate, b.TokensBudget)
		}

		wantAssignee := core.DeriveAssignee(b.Steps)
		if b.Assignee != wantAssignee {
			t.Errorf("bead %s: Assignee = %q, want %q", b.ID, b.Assignee, wantAssignee)
		}

		wantComments := core.DeriveCommentCount(b.History, b.Reviewer)
		if b.Comments != wantComments {
			t.Errorf("bead %s: Comments = %d, want %d", b.ID, b.Comments, wantComments)
		}
	}
}
