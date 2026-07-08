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

func TestMemories_Forget_NotFoundOnStdoutWithNonZeroExit(t *testing.T) {
	// bd exits non-zero but prints the sentinel to STDOUT (not stderr) — Forget
	// must still map it to ErrMemoryNotFound (error path checks both streams).
	cli, _ := newCLI(t, `echo 'No memory with key "missing"'; exit 1`)
	if err := cli.Forget(context.Background(), "missing"); !errors.Is(err, bdshell.ErrMemoryNotFound) {
		t.Fatalf("forget (non-zero exit, sentinel on stdout) = %v, want ErrMemoryNotFound", err)
	}
}

func TestMemories_Forget_UnrecognizedSuccessOutputIsError(t *testing.T) {
	// Exit 0 but output is neither a "Forgot" confirmation nor the not-found
	// sentinel — must NOT report a false success (which would become a bogus 204).
	cli, _ := newCLI(t, `echo 'huh, something changed'`)
	err := cli.Forget(context.Background(), "k")
	if err == nil || errors.Is(err, bdshell.ErrMemoryNotFound) {
		t.Fatalf("forget (unrecognized exit-0 output) = %v, want a non-nil non-NotFound error", err)
	}
}

func TestMemories_Forget_NonZeroExitWithoutSentinelIsError(t *testing.T) {
	// A genuine failure (non-zero exit, no not-found sentinel) must surface as an
	// error, not be swallowed as not-found.
	cli, _ := newCLI(t, `echo 'boom' 1>&2; exit 1`)
	err := cli.Forget(context.Background(), "k")
	if err == nil || errors.Is(err, bdshell.ErrMemoryNotFound) {
		t.Fatalf("forget (real failure) = %v, want a non-nil non-NotFound error", err)
	}
}

func TestMemories_List_NonStringValueIsError(t *testing.T) {
	// A non-string value (other than the schema_version meta) is unexpected bd
	// output and must be surfaced as a typed error, not silently dropped.
	cli, _ := newCLI(t, `echo '{"m1":"v1","weird":123}'`)
	if _, err := cli.Memories(context.Background(), ""); err == nil {
		t.Fatal("Memories with a non-string value = nil error, want error")
	}
	// The known schema_version int meta is still filtered without error.
	cli2, _ := newCLI(t, `echo '{"m1":"v1","schema_version":1}'`)
	if m, err := cli2.Memories(context.Background(), ""); err != nil || len(m) != 1 || m["m1"] != "v1" {
		t.Fatalf("Memories = %v err=%v, want {m1:v1}", m, err)
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
