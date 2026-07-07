package bdshell_test

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"

	"github.com/gitrgoliveira/muster/internal/store/bdshell"
)

func firstBeadID(t *testing.T, listJSON []byte) string {
	t.Helper()
	var beads []struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(listJSON, &beads); err != nil || len(beads) == 0 {
		return ""
	}
	return beads[0].ID
}

// TestLabels_RealBD is a skip-gated real-binary integration test: it runs only
// when a real `bd` is on PATH and an isolated project can be initialized. It
// exercises the actual `bd label list --json` path the fake-bd unit test stubs.
func TestLabels_RealBD(t *testing.T) {
	bdPath, err := exec.LookPath("bd")
	if err != nil {
		t.Skip("bd not on PATH; skipping real-bd integration test")
	}

	// Isolated project + HOME so discovery cannot escape to a real beads DB.
	dir := t.TempDir()
	home := t.TempDir()
	run := func(args ...string) error {
		cmd := exec.Command(bdPath, args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "HOME="+home, "BEADS_DIR="+filepath.Join(dir, ".beads"))
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Logf("bd %v: %v\n%s", args, err, out)
		}
		return err
	}

	if err := run("init", "--prefix", "itest"); err != nil {
		t.Skip("could not init an isolated bd project; skipping")
	}
	created := exec.Command(bdPath, "create", "--json", "labelled bead")
	created.Dir = dir
	created.Env = append(os.Environ(), "HOME="+home, "BEADS_DIR="+filepath.Join(dir, ".beads"))
	if out, err := created.CombinedOutput(); err != nil {
		t.Skipf("could not create a bead: %v\n%s", err, out)
	}

	// Find the created bead id.
	list := exec.Command(bdPath, "list", "--json")
	list.Dir = dir
	list.Env = append(os.Environ(), "HOME="+home, "BEADS_DIR="+filepath.Join(dir, ".beads"))
	out, err := list.Output()
	if err != nil || len(out) == 0 {
		t.Skipf("could not list beads: %v", err)
	}
	id := firstBeadID(t, out)
	if id == "" {
		t.Skip("no bead id found")
	}

	// `bd label add [issue-id...] [label]` takes ONE label (the last arg), so add
	// each separately.
	if err := run("label", "add", id, "skill:repo-grep"); err != nil {
		t.Skip("bd label add failed; skipping")
	}
	if err := run("label", "add", id, "area:core"); err != nil {
		t.Skip("bd label add failed; skipping")
	}

	cli, err := bdshell.NewCLI(bdPath, filepath.Join(dir, ".beads"))
	if err != nil {
		t.Fatal(err)
	}
	// CLI runs with HOME defaulted; point it at the isolated project via BEADS_DIR
	// (NewCLI already sets BEADS_DIR from the beadsDir arg).
	got, err := cli.Labels(context.Background(), id)
	if err != nil {
		t.Fatalf("real bd Labels: %v", err)
	}
	if !contains(got, "skill:repo-grep") || !contains(got, "area:core") {
		t.Fatalf("labels = %v, want skill:repo-grep + area:core", got)
	}
}

func contains(ss []string, want string) bool {
	return slices.Contains(ss, want)
}
