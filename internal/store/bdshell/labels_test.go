package bdshell_test

import (
	"context"
	"testing"

	"github.com/gitrgoliveira/muster/internal/store/bdshell"
)

func TestLabels_FakeBD_ParsesArray(t *testing.T) {
	cli, _ := newCLI(t, `echo '["skill:repo-grep","area:core"]'`)
	got, err := cli.Labels(context.Background(), "muster-ep0")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != "skill:repo-grep" || got[1] != "area:core" {
		t.Fatalf("Labels = %v", got)
	}
}

func TestLabels_FakeBD_EmptyArray(t *testing.T) {
	cli, _ := newCLI(t, `echo '[]'`)
	got, err := cli.Labels(context.Background(), "b-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("expected no labels, got %v", got)
	}
}

func TestLabels_FakeBD_NonZeroExitIsCLIError(t *testing.T) {
	cli, _ := newCLI(t, `echo "no such bead" >&2; exit 1`)
	_, err := cli.Labels(context.Background(), "nope")
	if err == nil {
		t.Fatal("want error for exit 1")
	}
	var cliErr *bdshell.CLIError
	if !isCLIError(err, &cliErr) {
		t.Fatalf("want *CLIError, got %T", err)
	}
}
