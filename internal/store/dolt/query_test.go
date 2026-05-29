package dolt

import (
	"strings"
	"testing"

	"github.com/gitrgoliveira/muster/internal/store"
)

func TestBuildListQuery_NoFilter(t *testing.T) {
	q, args := buildListQuery(store.Filter{})
	if !strings.HasPrefix(q, "\nSELECT") {
		t.Errorf("query should start with SELECT, got %q", q[:20])
	}
	if strings.Contains(q, "WHERE") {
		t.Errorf("empty filter should have no WHERE clause")
	}
	if len(args) != 0 {
		t.Errorf("want 0 args, got %d", len(args))
	}
}

func TestBuildListQuery_StatusFilter(t *testing.T) {
	q, args := buildListQuery(store.Filter{Status: []string{"open", "in_progress"}})
	if !strings.Contains(q, "WHERE") {
		t.Errorf("status filter should produce WHERE clause")
	}
	if !strings.Contains(q, "status IN") {
		t.Errorf("status filter should produce IN clause")
	}
	if len(args) != 2 {
		t.Errorf("want 2 args, got %d", len(args))
	}
	if args[0] != "open" || args[1] != "in_progress" {
		t.Errorf("unexpected args: %v", args)
	}
}

func TestBuildListQuery_IDsFilter(t *testing.T) {
	q, args := buildListQuery(store.Filter{IDs: []string{"mp-aaa", "mp-bbb"}})
	if !strings.Contains(q, "id IN") {
		t.Errorf("IDs filter should produce id IN clause, got %q", q)
	}
	if len(args) != 2 {
		t.Errorf("want 2 args, got %d", len(args))
	}
}

func TestBuildListQuery_StatusAndIDs(t *testing.T) {
	q, args := buildListQuery(store.Filter{
		Status: []string{"open"},
		IDs:    []string{"mp-aaa"},
	})
	if !strings.Contains(q, "status IN") || !strings.Contains(q, "id IN") {
		t.Errorf("combined filter should have both clauses: %q", q)
	}
	if !strings.Contains(q, "AND") {
		t.Errorf("multiple clauses should be joined with AND")
	}
	if len(args) != 2 {
		t.Errorf("want 2 args, got %d", len(args))
	}
}

func TestBuildListQuery_Limit(t *testing.T) {
	q, _ := buildListQuery(store.Filter{Limit: 10})
	if !strings.Contains(q, "LIMIT 10") {
		t.Errorf("want LIMIT 10 in query, got %q", q)
	}
}

func TestBuildListQuery_NoLimitWhenZero(t *testing.T) {
	q, _ := buildListQuery(store.Filter{Limit: 0})
	if strings.Contains(q, "LIMIT") {
		t.Errorf("zero limit should not add LIMIT clause")
	}
}

func TestBuildListQuery_Placeholders(t *testing.T) {
	f := store.Filter{Status: []string{"a", "b", "c"}}
	q, args := buildListQuery(f)
	// Should have exactly 3 placeholders.
	count := strings.Count(q, "?")
	if count != 3 {
		t.Errorf("want 3 placeholders, got %d in %q", count, q)
	}
	if len(args) != 3 {
		t.Errorf("want 3 args, got %d", len(args))
	}
}
