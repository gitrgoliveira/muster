package orchestrator

import (
	"errors"
	"fmt"
	"testing"

	"github.com/gitrgoliveira/muster/internal/services"
)

// TestMapDispatchError verifies orchestrator sentinels map to the right
// services error codes via errors.Is/As (not message-string matching), and that
// wrapping is handled.
func TestMapDispatchError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		wantCode string
	}{
		{"run already active", ErrRunAlreadyActive, services.CodeRunAlreadyActive},
		{"unmapped prefix", ErrUnmappedPrefix, services.CodeUnmappedPrefix},
		{"adapter not found", ErrAdapterNotFound, services.CodeAdapterNotFound},
		{"adapter not installed", ErrAdapterNotInstalled, services.CodeAdapterNotInstalled},
		{"adapter not logged in", ErrAdapterNotLoggedIn, services.CodeAdapterNotLoggedIn},
		{"no permission mode", ErrNoPermissionMode, services.CodeInvalidRequest},
		{"unsupported mode", ErrUnsupportedMode, services.CodeInvalidRequest},
		{"perm mode error", &PermModeError{Mode: "bogus"}, services.CodeInvalidRequest},
		{"wrapped sentinel", fmt.Errorf("dispatch: %w", ErrUnmappedPrefix), services.CodeUnmappedPrefix},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			out := mapDispatchError(tc.err)
			var se *services.ServiceError
			if !errors.As(out, &se) {
				t.Fatalf("mapDispatchError(%v) = %T, want *services.ServiceError", tc.err, out)
			}
			if se.Code != tc.wantCode {
				t.Errorf("code = %q, want %q", se.Code, tc.wantCode)
			}
		})
	}

	if mapDispatchError(nil) != nil {
		t.Error("mapDispatchError(nil) should be nil")
	}
	// Unknown errors are passed through untyped (services maps them to Internal).
	unknown := errors.New("boom")
	if out := mapDispatchError(unknown); out != unknown {
		t.Errorf("unknown error should pass through unchanged, got %v", out)
	}
}
