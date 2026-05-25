package services

import (
	"context"
	"errors"
	"testing"

	"github.com/gitrgoliveira/muster/internal/store/bdshell"
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
