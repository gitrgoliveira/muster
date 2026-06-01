package claude_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gitrgoliveira/muster/internal/adapter"
	"github.com/gitrgoliveira/muster/internal/adapter/claude"
	"github.com/gitrgoliveira/muster/internal/core"
)

// fakeBinDir returns a temp dir containing a 'claude' symlink to the fake script,
// and prepends it to PATH in the test environment.
func fakeBinDir(t *testing.T) (dir string) {
	t.Helper()
	dir = t.TempDir()

	// Find the fake_claude.sh script relative to this test file.
	script, err := filepath.Abs("testdata/fake_claude.sh")
	if err != nil {
		t.Fatalf("abs fake_claude.sh: %v", err)
	}
	if _, err := os.Stat(script); err != nil {
		t.Fatalf("fake_claude.sh not found: %v", err)
	}

	dest := filepath.Join(dir, "claude")
	if err := os.Symlink(script, dest); err != nil {
		t.Fatalf("symlink fake_claude: %v", err)
	}

	// Prepend the fake bin dir to PATH.
	oldPath := os.Getenv("PATH")
	t.Setenv("PATH", dir+string(os.PathListSeparator)+oldPath)

	return dir
}

func TestDetect_Installed_LoggedIn(t *testing.T) {
	fakeBinDir(t)
	// Default: loggedIn=true
	a := claude.New(claude.Options{})
	result, err := a.Detect(context.Background())
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if !result.Installed {
		t.Error("Installed want true")
	}
	if result.Version == "" {
		t.Error("Version should be non-empty")
	}
	if !result.LoggedIn {
		t.Error("LoggedIn want true")
	}
}

func TestDetect_NotLoggedIn(t *testing.T) {
	fakeBinDir(t)
	t.Setenv("FAKE_CLAUDE_AUTH_LOGGED_IN", "false")
	a := claude.New(claude.Options{})
	result, err := a.Detect(context.Background())
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if !result.Installed {
		t.Error("Installed want true")
	}
	if result.LoggedIn {
		t.Error("LoggedIn want false")
	}
}

func TestDetect_NotInstalled(t *testing.T) {
	// Use an empty PATH so claude is not found.
	t.Setenv("PATH", t.TempDir())
	a := claude.New(claude.Options{})
	result, err := a.Detect(context.Background())
	if err != nil {
		t.Fatalf("Detect returned unexpected error: %v", err)
	}
	if result.Installed {
		t.Error("Installed want false when claude not on PATH")
	}
}

func TestModes_PlanAndAgent(t *testing.T) {
	a := claude.New(claude.Options{})
	modes := a.Modes()
	if len(modes) < 2 {
		t.Fatalf("Modes want at least 2, got %d", len(modes))
	}

	modeMap := make(map[core.Mode]adapter.Mode)
	for _, m := range modes {
		modeMap[m.ID] = m
	}

	// Plan mode: --permission-mode plan
	planMode, ok := modeMap[core.ModePlan]
	if !ok {
		t.Fatal("plan mode not found")
	}
	planArgs := planMode.Args(core.PermAcceptEdits) // permission mode is irrelevant for plan
	if len(planArgs) < 2 {
		t.Fatalf("plan args want at least 2, got %v", planArgs)
	}
	// Should contain "--permission-mode" and "plan"
	found := false
	for i, arg := range planArgs {
		if arg == "--permission-mode" && i+1 < len(planArgs) && planArgs[i+1] == "plan" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("plan mode args should contain --permission-mode plan, got %v", planArgs)
	}

	// Agent mode: --permission-mode <pm>
	agentMode, ok := modeMap[core.ModeAgent]
	if !ok {
		t.Fatal("agent mode not found")
	}
	agentArgs := agentMode.Args(core.PermAcceptEdits)
	found = false
	for i, arg := range agentArgs {
		if arg == "--permission-mode" && i+1 < len(agentArgs) && agentArgs[i+1] == "acceptEdits" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("agent mode args should contain --permission-mode acceptEdits, got %v", agentArgs)
	}
}

func TestInvoke_Spec(t *testing.T) {
	fakeBinDir(t)
	a := claude.New(claude.Options{})
	worktree := t.TempDir()
	promptFile := filepath.Join(worktree, ".muster-prompt-0.txt")
	if err := os.WriteFile(promptFile, []byte("test prompt"), 0644); err != nil {
		t.Fatal(err)
	}

	req := adapter.InvokeReq{
		Bead: core.Bead{
			ID:    "mp-abc",
			Title: "Test task",
		},
		Mode:           core.ModeAgent,
		PermissionMode: core.PermAcceptEdits,
		Worktree:       worktree,
		PromptFile:     promptFile,
	}

	spec, err := a.Invoke(context.Background(), req)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}

	// Spec should have argv
	if len(spec.Argv) == 0 {
		t.Error("Spec.Argv should not be empty")
	}
	// Cwd should match worktree
	if spec.Cwd != worktree {
		t.Errorf("Cwd want %q got %q", worktree, spec.Cwd)
	}
	// Argv should contain claude binary
	if spec.Argv[0] == "" {
		t.Error("Argv[0] (binary) should not be empty")
	}
}

func TestLogin_ReturnsErrNotSupported(t *testing.T) {
	a := claude.New(claude.Options{})
	_, err := a.Login(context.Background())
	if err != adapter.ErrNotSupported {
		t.Errorf("Login want ErrNotSupported got %v", err)
	}
}

func TestQuotaSource_None(t *testing.T) {
	a := claude.New(claude.Options{})
	if qs := a.QuotaSource(); qs != adapter.QuotaNone {
		t.Errorf("QuotaSource want QuotaNone got %v", qs)
	}
}

func TestID_IsClaude(t *testing.T) {
	a := claude.New(claude.Options{})
	if id := a.ID(); id != core.AgentClaude {
		t.Errorf("ID want claude got %q", id)
	}
}

func TestDetect_VersionParsedFromOutput(t *testing.T) {
	// Provide a custom version string
	fakeBinDir(t)
	t.Setenv("FAKE_CLAUDE_VERSION", "9.9.999 (Claude Code)")
	a := claude.New(claude.Options{})
	result, err := a.Detect(context.Background())
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if result.Version != "9.9.999 (Claude Code)" {
		t.Errorf("Version want '9.9.999 (Claude Code)' got %q", result.Version)
	}
}

func TestInvoke_BinPathWithSpace(t *testing.T) {
	// Verify that a claude binary path containing a space is shell-quoted
	// correctly so the sh -c one-liner does not break.
	worktree := t.TempDir()
	promptFile := filepath.Join(worktree, ".muster-prompt-0.txt")
	if err := os.WriteFile(promptFile, []byte("test"), 0644); err != nil {
		t.Fatal(err)
	}

	// Use an explicit bin path that contains a space.
	a := claude.New(claude.Options{Bin: "/Users/Some User/bin/claude"})
	req := adapter.InvokeReq{
		Bead:           core.Bead{ID: "mp-abc", Title: "Test"},
		Mode:           core.ModeAgent,
		PermissionMode: core.PermAcceptEdits,
		Worktree:       worktree,
		PromptFile:     promptFile,
	}

	spec, err := a.Invoke(context.Background(), req)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if len(spec.Argv) < 3 || spec.Argv[0] != "sh" || spec.Argv[1] != "-c" {
		t.Fatalf("unexpected argv: %v", spec.Argv)
	}
	shellCmd := spec.Argv[2]
	// The binary path must be single-quoted in the shell command.
	if !strings.Contains(shellCmd, "'/Users/Some User/bin/claude'") {
		t.Errorf("expected single-quoted binary path in shell command, got: %q", shellCmd)
	}
}

func TestDetect_ExplicitBin(t *testing.T) {
	// Use the fake_claude.sh script as an explicit binary path
	script, err := filepath.Abs("testdata/fake_claude.sh")
	if err != nil {
		t.Fatalf("abs: %v", err)
	}
	if _, err := exec.LookPath(script); err != nil {
		t.Skip("fake_claude.sh not executable, skipping")
	}
	a := claude.New(claude.Options{Bin: script})
	result, err := a.Detect(context.Background())
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if !result.Installed {
		t.Error("Installed want true")
	}
}
