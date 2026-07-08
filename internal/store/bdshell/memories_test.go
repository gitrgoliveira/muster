package bdshell_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gitrgoliveira/muster/internal/store/bdshell"
)

const memoriesFake = `
case "$1" in
  remember) echo '{"action":"remembered","key":"derived-key"}';;
  memories)  echo '{"m1":"v1","schema_version":1}';;
  recall)    echo "the-value";;
  forget)    if echo "$*" | grep -q missing; then echo 'No memory with key "missing"'; else echo 'Forgot [k]: v'; fi;;
esac`

func TestMemories_Remember_ReturnsKey(t *testing.T) {
	cli, _ := newCLI(t, memoriesFake)
	key, err := cli.Remember(context.Background(), "", "some value")
	if err != nil {
		t.Fatal(err)
	}
	if key != "derived-key" {
		t.Fatalf("Remember key = %q, want derived-key", key)
	}
}

func TestMemories_Remember_EmptyKeyIsError(t *testing.T) {
	// bd omits the derived key AND the caller gave none — a keyless memory is
	// unusable (ambiguous recall/delete), so Remember must fail rather than
	// return an empty key.
	cli, _ := newCLI(t, `echo '{"action":"remembered"}'`)
	if _, err := cli.Remember(context.Background(), "", "some value"); err == nil {
		t.Fatal("Remember with no derived key and no caller key = nil error, want error")
	}
	// But an explicit caller key still round-trips even if bd omits it.
	if key, err := cli.Remember(context.Background(), "my-key", "v"); err != nil || key != "my-key" {
		t.Fatalf("Remember with caller key = %q err=%v, want my-key", key, err)
	}
}

func TestMemories_List_FiltersSchemaVersion(t *testing.T) {
	cli, _ := newCLI(t, memoriesFake)
	m, err := cli.Memories(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if len(m) != 1 || m["m1"] != "v1" {
		t.Fatalf("Memories = %v (schema_version must be filtered out)", m)
	}
	if _, ok := m["schema_version"]; ok {
		t.Fatal("schema_version meta entry leaked into memories")
	}
}

func TestMemories_Forget_NotFoundDetected(t *testing.T) {
	cli, _ := newCLI(t, memoriesFake)
	if err := cli.Forget(context.Background(), "k"); err != nil {
		t.Fatalf("forget existing = %v", err)
	}
	if err := cli.Forget(context.Background(), "missing"); !errors.Is(err, bdshell.ErrMemoryNotFound) {
		t.Fatalf("forget missing = %v, want ErrMemoryNotFound", err)
	}
}

func TestMemories_Forget_ValueContainingSentinelSucceeds(t *testing.T) {
	// A successful delete whose VALUE mentions the not-found sentinel must not be
	// misreported as not-found (detection anchors on the "Forgot" prefix).
	cli, _ := newCLI(t, `echo 'Forgot [k]: a value mentioning No memory with key'`)
	if err := cli.Forget(context.Background(), "k"); err != nil {
		t.Fatalf("forget = %v, want nil", err)
	}
}

func TestMemories_ArgSeparatorGuardsInjection(t *testing.T) {
	// The fake records its argv so we can assert the `--` separator precedes a
	// value that starts with '-' (no flag injection).
	cli, beadsDir := newCLI(t, `echo "$@" > "$BEADS_DIR/args"; echo '{"action":"remembered","key":"k"}'`)
	if _, err := cli.Remember(context.Background(), "", "-danger"); err != nil {
		t.Fatal(err)
	}
	args, err := os.ReadFile(filepath.Join(beadsDir, "args"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(args), "-- -danger") {
		t.Fatalf("value not passed after '--': %q", args)
	}
}
