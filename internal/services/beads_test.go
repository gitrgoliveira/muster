package services

import (
	"context"
	"errors"
	"testing"

	"github.com/gitrgoliveira/muster/internal/core"
	"github.com/gitrgoliveira/muster/internal/store"
	"github.com/gitrgoliveira/muster/internal/store/bdshell"
	"github.com/gitrgoliveira/muster/internal/ws"
)

func TestWrapCLIError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		wantCode string
	}{
		{"exit 1 → validation", &bdshell.CLIError{ExitCode: 1, Stderr: "bad input"}, CodeCLIValidation},
		{"exit 2 → not found", &bdshell.CLIError{ExitCode: 2, Stderr: "missing"}, CodeNotFound},
		{"exit 3 → unavailable", &bdshell.CLIError{ExitCode: 3, Stderr: "down"}, CodeCLIUnavailable},
		{"exit 99 → internal", &bdshell.CLIError{ExitCode: 99, Stderr: "unknown"}, CodeInternal},
		{"deadline exceeded → timeout", context.DeadlineExceeded, CodeCLITimeout},
		{"generic error → internal", errors.New("oops"), CodeInternal},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := wrapCLIError(tc.err)
			if got.Code != tc.wantCode {
				t.Errorf("wrapCLIError(%v) code = %q, want %q", tc.err, got.Code, tc.wantCode)
			}
		})
	}
}

func TestPatch_RejectsUnsupportedFields(t *testing.T) {
	backend := store.NewMemoryBackend(nil)
	svc := NewBeadService(backend, nil, func(_ ws.Frame) {})

	tests := []struct {
		name  string
		input PatchBeadInput
		want  string
	}{
		{
			"labels rejected",
			PatchBeadInput{Labels: &[]string{"foo"}},
			"labels patch not supported",
		},
		{
			"ready rejected",
			PatchBeadInput{Ready: boolPtr(true)},
			"ready patch not supported",
		},
		{
			"tokensBudget rejected",
			PatchBeadInput{TokensBudget: intPtr(100)},
			"tokensBudget patch not supported",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := svc.Patch(context.Background(), "mp-aaa", tc.input)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			se, ok := err.(*ServiceError)
			if !ok {
				t.Fatalf("expected ServiceError, got %T", err)
			}
			if se.Code != CodeInvalidRequest {
				t.Errorf("code = %q, want %q", se.Code, CodeInvalidRequest)
			}
			if !contains(se.Message, tc.want) {
				t.Errorf("message %q does not contain %q", se.Message, tc.want)
			}
		})
	}
}

func TestColumnToStatuses_BacklogIncludesScheduled(t *testing.T) {
	statuses := columnToStatuses("backlog")
	found := false
	for _, s := range statuses {
		if s == "scheduled" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("columnToStatuses(\"backlog\") = %v, want to include \"scheduled\"", statuses)
	}
}

func TestColumnToStatuses_RoundTrip(t *testing.T) {
	tests := []struct {
		status string
		column core.Column
	}{
		{"open", core.ColBacklog},
		{"scheduled", core.ColBacklog},
		{"in_progress", core.ColRunning},
		{"in_review", core.ColReview},
		{"closed", core.ColDone},
		{"cancelled", core.ColDone},
	}
	for _, tc := range tests {
		t.Run(tc.status, func(t *testing.T) {
			col := statusToColumn(tc.status)
			if col != tc.column {
				t.Errorf("statusToColumn(%q) = %q, want %q", tc.status, col, tc.column)
			}
			statuses := columnToStatuses(string(tc.column))
			found := false
			for _, s := range statuses {
				if s == tc.status {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("columnToStatuses(%q) = %v, does not include %q", tc.column, statuses, tc.status)
			}
		})
	}
}

func boolPtr(b bool) *bool { return &b }
func intPtr(i int) *int    { return &i }

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 || indexOf(s, substr) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
