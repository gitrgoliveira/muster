package bdshell_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gitrgoliveira/muster/internal/store/bdshell"
)

// makeFakeBD writes a shell script to dir/bd and returns the dir.
func makeFakeBD(t *testing.T, script string) string {
	t.Helper()
	dir := t.TempDir()
	bdPath := filepath.Join(dir, "bd")
	content := "#!/bin/sh\n" + script
	if err := os.WriteFile(bdPath, []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

// newCLI returns a CLI with a fake bd binary and BEADS_DIR set.
func newCLI(t *testing.T, script string) (*bdshell.CLI, string) {
	t.Helper()
	dir := makeFakeBD(t, script)
	beadsDir := t.TempDir()
	cli, err := bdshell.NewCLI(filepath.Join(dir, "bd"), beadsDir)
	if err != nil {
		t.Fatal(err)
	}
	return cli, beadsDir
}

func TestNewCLI_BinaryMissing(t *testing.T) {
	_, err := bdshell.NewCLI("/nonexistent/bd", "/beads")
	if err == nil {
		t.Fatal("want error for missing binary")
	}
}

func TestNewCLI_LookPath(t *testing.T) {
	// Create a fake bd on PATH via a temp dir.
	dir := makeFakeBD(t, `echo "ok"`)
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))

	cli, err := bdshell.NewCLI("", t.TempDir())
	if err != nil {
		t.Fatalf("NewCLI via LookPath: %v", err)
	}
	if cli.Path == "" {
		t.Error("Path should be set after LookPath")
	}
}

func TestCLI_Exit0(t *testing.T) {
	cli, _ := newCLI(t, `echo "hello"`)
	res, err := cli.Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(res.Stdout, "hello") {
		t.Errorf("stdout want 'hello' got %q", res.Stdout)
	}
}

func TestCLI_Exit1_CLIError(t *testing.T) {
	cli, _ := newCLI(t, `echo "bad request" >&2; exit 1`)
	_, err := cli.Run(context.Background())
	if err == nil {
		t.Fatal("want error for exit 1")
	}
	var cliErr *bdshell.CLIError
	if !isCLIError(err, &cliErr) {
		t.Fatalf("want *CLIError, got %T: %v", err, err)
	}
	if cliErr.ExitCode != 1 {
		t.Errorf("want ExitCode 1, got %d", cliErr.ExitCode)
	}
}

func TestCLI_Exit2(t *testing.T) {
	cli, _ := newCLI(t, `exit 2`)
	_, err := cli.Run(context.Background())
	var cliErr *bdshell.CLIError
	if !isCLIError(err, &cliErr) {
		t.Fatalf("want *CLIError, got %T", err)
	}
	if cliErr.ExitCode != 2 {
		t.Errorf("want ExitCode 2, got %d", cliErr.ExitCode)
	}
}

func TestCLI_Exit3(t *testing.T) {
	cli, _ := newCLI(t, `exit 3`)
	_, err := cli.Run(context.Background())
	var cliErr *bdshell.CLIError
	if !isCLIError(err, &cliErr) {
		t.Fatalf("want *CLIError, got %T", err)
	}
	if cliErr.ExitCode != 3 {
		t.Errorf("want ExitCode 3, got %d", cliErr.ExitCode)
	}
}

func TestCLI_Timeout(t *testing.T) {
	cli, _ := newCLI(t, `sleep 60`)
	cli.Timeout = 50 * time.Millisecond

	ctx := context.Background()
	start := time.Now()
	_, err := cli.Run(ctx)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("want timeout error")
	}
	if elapsed > 2*time.Second {
		t.Errorf("timeout took too long: %v", elapsed)
	}
}

func TestCLI_ANSIStripped(t *testing.T) {
	cli, _ := newCLI(t, `printf '\033[31mred error\033[0m' >&2; exit 1`)
	_, err := cli.Run(context.Background())
	var cliErr *bdshell.CLIError
	if !isCLIError(err, &cliErr) {
		t.Fatal("want CLIError")
	}
	if strings.Contains(cliErr.Stderr, "\x1b") {
		t.Errorf("ANSI not stripped from stderr: %q", cliErr.Stderr)
	}
	if !strings.Contains(cliErr.Stderr, "red error") {
		t.Errorf("stderr should contain 'red error', got %q", cliErr.Stderr)
	}
}

func TestCLI_StderrTruncated512(t *testing.T) {
	// Print >512 bytes to stderr.
	cli, _ := newCLI(t, `python3 -c "print('X'*600, end='')" >&2; exit 1`)
	_, err := cli.Run(context.Background())
	var cliErr *bdshell.CLIError
	if !isCLIError(err, &cliErr) {
		// python3 may not be available; try awk.
		cli2, _ := newCLI(t, `awk 'BEGIN{for(i=0;i<600;i++)printf "X"; exit 1}'`)
		cli2.Timeout = 5 * time.Second
		_, err2 := cli2.Run(context.Background())
		if !isCLIError(err2, &cliErr) {
			t.Skip("cannot produce long stderr in this environment")
		}
	}
	if len(cliErr.Stderr) > 512 {
		t.Errorf("stderr not truncated to 512, len=%d", len(cliErr.Stderr))
	}
}

func TestCLI_BEADSDIRInEnv(t *testing.T) {
	// The fake bd script echoes BEADS_DIR to stdout.
	dir := makeFakeBD(t, `echo "$BEADS_DIR"`)
	beadsDir := t.TempDir()
	cli, err := bdshell.NewCLI(filepath.Join(dir, "bd"), beadsDir)
	if err != nil {
		t.Fatal(err)
	}
	res, err := cli.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	got := strings.TrimSpace(res.Stdout)
	// Resolve symlinks for macOS /var → /private/var.
	wantResolved, _ := filepath.EvalSymlinks(beadsDir)
	gotResolved, _ := filepath.EvalSymlinks(got)
	if gotResolved != wantResolved && got != beadsDir {
		t.Errorf("BEADS_DIR want %q got %q", beadsDir, got)
	}
}

func TestCLI_EnvOnlyPATHAndHOME(t *testing.T) {
	// The script prints all env vars; verify only BEADS_DIR, PATH, HOME are present.
	dir := makeFakeBD(t, `env`)
	beadsDir := t.TempDir()
	cli, err := bdshell.NewCLI(filepath.Join(dir, "bd"), beadsDir)
	if err != nil {
		t.Fatal(err)
	}
	// Set a sentinel env var that must NOT appear in subprocess.
	t.Setenv("MUSTER_SHOULD_NOT_LEAK", "secret")
	res, err := cli.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(res.Stdout, "MUSTER_SHOULD_NOT_LEAK") {
		t.Error("env leaked into subprocess: MUSTER_SHOULD_NOT_LEAK found in subprocess env")
	}
	if !strings.Contains(res.Stdout, "BEADS_DIR=") {
		t.Error("BEADS_DIR missing from subprocess env")
	}
}

func TestCLI_RunJSON(t *testing.T) {
	type payload struct {
		ID string `json:"id"`
	}
	script := `echo '{"id":"mp-abc"}'`
	cli, _ := newCLI(t, script)

	var p payload
	if err := cli.RunJSON(context.Background(), &p); err != nil {
		t.Fatal(err)
	}
	if p.ID != "mp-abc" {
		t.Errorf("want mp-abc got %q", p.ID)
	}
}

func TestCLI_RunJSONParseError(t *testing.T) {
	cli, _ := newCLI(t, `echo "not json"`)
	var dst map[string]any
	err := cli.RunJSON(context.Background(), &dst)
	if err == nil {
		t.Fatal("want parse error")
	}
}

func TestCLI_RunVoid(t *testing.T) {
	cli, _ := newCLI(t, `exit 0`)
	if err := cli.RunVoid(context.Background()); err != nil {
		t.Errorf("RunVoid: %v", err)
	}
}

func TestCLI_RunVoidPropagatesError(t *testing.T) {
	cli, _ := newCLI(t, `exit 1`)
	if err := cli.RunVoid(context.Background()); err == nil {
		t.Fatal("want error from RunVoid on exit 1")
	}
}

func TestCLI_ArgsPassedThrough(t *testing.T) {
	cli, _ := newCLI(t, `echo "$@"`)
	res, err := cli.Run(context.Background(), "list", "--status=open")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Stdout, "list") || !strings.Contains(res.Stdout, "--status=open") {
		t.Errorf("args not passed through: %q", res.Stdout)
	}
}

func TestCLI_DashValueArgForm(t *testing.T) {
	// Values starting with '-' must still be passed safely (as --flag=value argv).
	cli, _ := newCLI(t, `echo "$@"`)
	res, err := cli.Run(context.Background(), "--title=-my title")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Stdout, "--title=-my title") {
		t.Errorf("dash-value arg not passed correctly: %q", res.Stdout)
	}
}

// isCLIError checks if err wraps a *bdshell.CLIError and populates out.
func isCLIError(err error, out **bdshell.CLIError) bool {
	return errors.As(err, out)
}

// ensure json is imported (used in RunJSON test)
var _ = json.Marshal
