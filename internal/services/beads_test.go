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

func TestWrapOrchestratorError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		wantCode string
	}{
		{"already active → 409", errors.New("run already active for bead"), CodeRunAlreadyActive},
		{"unmapped prefix → 422", errors.New("bead prefix has no repo mapping"), CodeUnmappedPrefix},
		{"not registered → 501", errors.New("adapter not registered"), CodeAdapterNotFound},
		{"not installed → 501", errors.New("adapter not installed"), CodeAdapterNotInstalled},
		{"not logged in → 409", errors.New("adapter not logged in; run: claude auth login"), CodeAdapterNotLoggedIn},
		{"no perm mode → 4xx", errors.New("permissionMode is required (no default configured)"), CodeInvalidRequest},
		{"invalid perm mode → 4xx", errors.New("invalid permissionMode: bogus"), CodeInvalidRequest},
		{"unsupported mode → 4xx", errors.New("unsupported mode for adapter"), CodeInvalidRequest},
		{"unknown → internal", errors.New("boom"), CodeInternal},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := wrapOrchestratorError(tc.err)
			if got.Code != tc.wantCode {
				t.Errorf("wrapOrchestratorError(%v) code = %q, want %q", tc.err, got.Code, tc.wantCode)
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
		{"blocked", core.ColBacklog},
		{"deferred", core.ColBacklog},
		{"in_progress", core.ColRunning},
		{"closed", core.ColDone},
		{"cancelled", core.ColDone},
		{"superseded", core.ColDone},
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

// fakeCLI is a CLIRunner that returns a fixed issue for every write.
type fakeCLI struct{ iss store.Issue }

func (f fakeCLI) Create(context.Context, bdshell.CreateInput) (store.Issue, error) {
	return f.iss, nil
}
func (f fakeCLI) Update(context.Context, string, bdshell.UpdatePatch) (store.Issue, error) {
	return f.iss, nil
}
func (f fakeCLI) Close(context.Context, string) (store.Issue, error)    { return f.iss, nil }
func (f fakeCLI) Dispatch(context.Context, string) (store.Issue, error) { return f.iss, nil }
func (f fakeCLI) AppendNote(context.Context, string, string) (store.Issue, error) {
	return f.iss, nil
}

func TestPublishOnWrite_RemoteMode(t *testing.T) {
	backend := store.NewMemoryBackend(nil)
	cli := fakeCLI{iss: store.Issue{ID: "mp-aaa", Title: "T", Status: "open", IssueType: "task"}}

	var frames []ws.Frame
	pub := func(f ws.Frame) { frames = append(frames, f) }
	svc := NewBeadServiceWithRepo(backend, cli, pub, "muster", true)

	if _, err := svc.Create(context.Background(), CreateBeadInput{Title: "T", Type: core.TypeTask, Priority: 2}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := svc.Patch(context.Background(), "mp-aaa", PatchBeadInput{Title: strPtr("T2")}); err != nil {
		t.Fatalf("Patch: %v", err)
	}

	if len(frames) != 2 {
		t.Fatalf("want 2 frames, got %d: %+v", len(frames), frames)
	}
	if frames[0].Type != ws.EventBeadCreated || frames[0].Bead == nil {
		t.Errorf("frame[0] = %+v, want bead.created with Bead", frames[0])
	}
	if frames[1].Type != ws.EventBeadUpdated || frames[1].Bead == nil {
		t.Errorf("frame[1] = %+v, want bead.updated with Bead", frames[1])
	}
}

func TestPublishOnWrite_EmbeddedModeSilent(t *testing.T) {
	backend := store.NewMemoryBackend(nil)
	cli := fakeCLI{iss: store.Issue{ID: "mp-aaa", Title: "T", Status: "open", IssueType: "task"}}

	var frames []ws.Frame
	pub := func(f ws.Frame) { frames = append(frames, f) }
	// publishOnWrite=false → watcher is the WS source, service must not publish.
	svc := NewBeadServiceWithRepo(backend, cli, pub, "muster", false)

	if _, err := svc.Create(context.Background(), CreateBeadInput{Title: "T", Type: core.TypeTask, Priority: 2}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if len(frames) != 0 {
		t.Errorf("embedded mode must not publish on write, got %d frames", len(frames))
	}
}

func strPtr(s string) *string { return &s }
func boolPtr(b bool) *bool    { return &b }
func intPtr(i int) *int       { return &i }

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
