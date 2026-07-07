package bdshell_test

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/gitrgoliveira/muster/internal/store/bdshell"
)

// TestMemories_RealBD is a skip-gated real-binary integration test exercising
// the actual bd remember/memories/forget path in an isolated project.
func TestMemories_RealBD(t *testing.T) {
	bdPath, err := exec.LookPath("bd")
	if err != nil {
		t.Skip("bd not on PATH; skipping")
	}
	dir := t.TempDir()
	home := t.TempDir()
	beadsDir := filepath.Join(dir, ".beads")

	initCmd := exec.Command(bdPath, "init", "--prefix", "mtest")
	initCmd.Dir = dir
	initCmd.Env = append(os.Environ(), "HOME="+home, "BEADS_DIR="+beadsDir)
	if out, err := initCmd.CombinedOutput(); err != nil {
		t.Skipf("could not init isolated bd project: %v\n%s", err, out)
	}

	// The CLI already sets BEADS_DIR from beadsDir; set HOME via the process env
	// so discovery stays inside the isolated project.
	t.Setenv("HOME", home)
	cli, err := bdshell.NewCLI(bdPath, beadsDir)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	key, err := cli.Remember(ctx, "", "always run tests with -race")
	if err != nil {
		t.Fatalf("Remember: %v", err)
	}
	if key == "" {
		t.Fatal("Remember returned an empty key")
	}

	m, err := cli.Memories(ctx, "")
	if err != nil {
		t.Fatalf("Memories: %v", err)
	}
	if _, ok := m[key]; !ok {
		t.Fatalf("memory %q not in list %v", key, m)
	}
	if _, ok := m["schema_version"]; ok {
		t.Fatal("schema_version leaked into memories")
	}

	if err := cli.Forget(ctx, key); err != nil {
		t.Fatalf("Forget: %v", err)
	}
	if err := cli.Forget(ctx, key); !errors.Is(err, bdshell.ErrMemoryNotFound) {
		t.Fatalf("Forget already-gone = %v, want ErrMemoryNotFound", err)
	}
}
