package tmux

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// fakeTmuxPath returns the absolute path to the fake_tmux.sh script in testdata.
func fakeTmuxPath(t *testing.T) string {
	t.Helper()
	p, err := filepath.Abs("testdata/fake_tmux.sh")
	if err != nil {
		t.Fatalf("abs fake_tmux.sh: %v", err)
	}
	if _, err := os.Stat(p); err != nil {
		t.Fatalf("fake_tmux.sh not found: %v", err)
	}
	return p
}

// setupFakeTmux installs the fake tmux binary on PATH and returns a tmpdir
// that records all invocations. The recordFile is the path to the
// FAKE_TMUX_RECORD_FILE.
func setupFakeTmux(t *testing.T) (recordFile string) {
	t.Helper()
	fakeScript := fakeTmuxPath(t)

	binDir := t.TempDir()
	dest := filepath.Join(binDir, "tmux")
	if err := os.Symlink(fakeScript, dest); err != nil {
		t.Fatalf("symlink fake tmux: %v", err)
	}

	oldPath := os.Getenv("PATH")
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+oldPath)

	recordFile = filepath.Join(t.TempDir(), "invocations.txt")
	t.Setenv("FAKE_TMUX_RECORD_FILE", recordFile)

	return recordFile
}

// readRecordFile reads the invocation record file, returning each line.
func readRecordFile(t *testing.T, recordFile string) []string {
	t.Helper()
	data, err := os.ReadFile(recordFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		t.Fatalf("read record file: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	var result []string
	for _, l := range lines {
		if l != "" {
			result = append(result, l)
		}
	}
	return result
}

func TestRealTmuxManager_Detect(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell-script fake tmux requires unix")
	}
	setupFakeTmux(t)

	// fake_tmux.sh reads FAKE_TMUX_OUTPUT_FILE to decide what to print for
	// any subcommand (here `tmux -V`). Point it at a temp file with a fake
	// version string.
	versionFile := filepath.Join(t.TempDir(), "version_output")
	if err := os.WriteFile(versionFile, []byte("tmux 3.6b\n"), 0644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FAKE_TMUX_OUTPUT_FILE", versionFile)

	m := NewRealManager("")
	version, err := m.Detect()
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if version == "" {
		t.Error("version should not be empty")
	}
}

func TestRealTmuxManager_Detect_NotFound(t *testing.T) {
	// Use an empty PATH so tmux is not found.
	t.Setenv("PATH", t.TempDir())

	m := NewRealManager("")
	_, err := m.Detect()
	if err == nil {
		t.Error("Detect should return error when tmux not on PATH")
	}
}

func TestRealTmuxManager_Spawn_CallsNewSession(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("shell-script fake tmux requires unix")
	}
	recordFile := setupFakeTmux(t)

	m := NewRealManager("")
	sess, err := m.Spawn("muster/mp-abc/0/0", t.TempDir(), nil, []string{"sh", "-c", "echo hello"})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	if sess.Name != "muster/mp-abc/0/0" {
		t.Errorf("Session.Name want muster/mp-abc/0/0 got %q", sess.Name)
	}

	lines := readRecordFile(t, recordFile)
	// Should have called new-session and set remain-on-exit.
	found := false
	for _, l := range lines {
		if strings.Contains(l, "new-session") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected new-session invocation in records, got: %v", lines)
	}

	// Check for remain-on-exit call.
	foundRemain := false
	for _, l := range lines {
		if strings.Contains(l, "remain-on-exit") {
			foundRemain = true
			break
		}
	}
	if !foundRemain {
		t.Errorf("expected remain-on-exit in tmux invocations, got: %v", lines)
	}
}

func TestRealTmuxManager_Kill_CallsKillSession(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("shell-script fake tmux requires unix")
	}
	recordFile := setupFakeTmux(t)

	m := NewRealManager("")
	err := m.Kill("muster/mp-abc/0/0")
	if err != nil {
		t.Fatalf("Kill: %v", err)
	}

	lines := readRecordFile(t, recordFile)
	found := false
	for _, l := range lines {
		if strings.Contains(l, "kill-session") && strings.Contains(l, "muster/mp-abc/0/0") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected kill-session muster/mp-abc/0/0 in records, got: %v", lines)
	}
}

func TestRealTmuxManager_DeadStatus(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("shell-script fake tmux requires unix")
	}
	setupFakeTmux(t)

	// Fake output: "1 0" means pane_dead=1, pane_dead_status=0.
	outputFile := filepath.Join(t.TempDir(), "dead_output")
	if err := os.WriteFile(outputFile, []byte("1 0\n"), 0644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FAKE_TMUX_OUTPUT_FILE", outputFile)

	m := NewRealManager("")
	code, dead, err := m.DeadStatus("muster/mp-abc/0/0")
	if err != nil {
		t.Fatalf("DeadStatus: %v", err)
	}
	if !dead {
		t.Error("dead want true")
	}
	if code != 0 {
		t.Errorf("code want 0 got %d", code)
	}
}

func TestRealTmuxManager_DeadStatus_NotDead(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("shell-script fake tmux requires unix")
	}
	setupFakeTmux(t)

	// "0 " means pane_dead=0 (not dead).
	outputFile := filepath.Join(t.TempDir(), "alive_output")
	if err := os.WriteFile(outputFile, []byte("0 \n"), 0644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FAKE_TMUX_OUTPUT_FILE", outputFile)

	m := NewRealManager("")
	_, dead, err := m.DeadStatus("muster/mp-abc/0/0")
	if err != nil {
		t.Fatalf("DeadStatus: %v", err)
	}
	if dead {
		t.Error("dead want false")
	}
}

func TestRealTmuxManager_DeadStatus_SignalKilledNoStatus(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("shell-script fake tmux requires unix")
	}
	setupFakeTmux(t)

	// "1 " means pane_dead=1 but pane_dead_status is empty/missing (signal death,
	// no numeric exit code). DeadStatus must report a non-zero code so the run is
	// treated as a failure rather than a success.
	outputFile := filepath.Join(t.TempDir(), "dead_no_status")
	if err := os.WriteFile(outputFile, []byte("1 \n"), 0644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FAKE_TMUX_OUTPUT_FILE", outputFile)

	m := NewRealManager("")
	code, dead, err := m.DeadStatus("muster/mp-abc/0/0")
	if err != nil {
		t.Fatalf("DeadStatus: %v", err)
	}
	if !dead {
		t.Error("dead want true")
	}
	if code == 0 {
		t.Errorf("code want non-zero (signal death) got %d", code)
	}
}

func TestRealTmuxManager_List(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("shell-script fake tmux requires unix")
	}
	setupFakeTmux(t)

	outputFile := filepath.Join(t.TempDir(), "list_output")
	content := "muster/mp-abc/0/0\nmuster/bd-0001/0/0\nother-session\n"
	if err := os.WriteFile(outputFile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FAKE_TMUX_OUTPUT_FILE", outputFile)

	m := NewRealManager("")
	sessions, err := m.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	// Should only return muster/ sessions.
	if len(sessions) != 2 {
		t.Errorf("want 2 muster sessions, got %d: %v", len(sessions), sessions)
	}

	names := make(map[string]bool)
	for _, s := range sessions {
		names[s.Name] = true
	}
	if !names["muster/mp-abc/0/0"] {
		t.Error("missing muster/mp-abc/0/0")
	}
	if !names["muster/bd-0001/0/0"] {
		t.Error("missing muster/bd-0001/0/0")
	}
	if names["other-session"] {
		t.Error("other-session should be filtered out")
	}
}

func TestRealTmuxManager_Attach_ReturnsCommand(t *testing.T) {
	m := NewRealManager("")
	cmd, err := m.Attach("muster/mp-abc/0/0")
	if err != nil {
		t.Fatalf("Attach: %v", err)
	}
	expected := "tmux attach -t muster/mp-abc/0/0"
	if cmd != expected {
		t.Errorf("Attach want %q got %q", expected, cmd)
	}
}

func TestRealTmuxManager_Send(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("shell-script fake tmux requires unix")
	}
	recordFile := setupFakeTmux(t)

	m := NewRealManager("")
	err := m.Send("muster/mp-abc/0/0", "y\n")
	if err != nil {
		t.Fatalf("Send: %v", err)
	}

	lines := readRecordFile(t, recordFile)
	found := false
	for _, l := range lines {
		if strings.Contains(l, "send-keys") && strings.Contains(l, "muster/mp-abc/0/0") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected send-keys in records, got: %v", lines)
	}
}

// TestRealTmuxManager_Capture verifies that Capture calls capture-pane with correct args.
func TestRealTmuxManager_Capture(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("shell-script fake tmux requires unix")
	}
	recordFile := setupFakeTmux(t)

	// Provide canned output for the capture-pane call.
	outputFile := filepath.Join(t.TempDir(), "capture_output")
	if err := os.WriteFile(outputFile, []byte("captured content\n"), 0644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FAKE_TMUX_OUTPUT_FILE", outputFile)

	m := NewRealManager("")
	out, err := m.Capture("muster/mp-abc/0/0", false)
	if err != nil {
		t.Fatalf("Capture: %v", err)
	}
	if out == "" {
		t.Error("Capture should return output")
	}

	lines := readRecordFile(t, recordFile)
	found := false
	for _, l := range lines {
		if strings.Contains(l, "capture-pane") && strings.Contains(l, "muster/mp-abc/0/0") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected capture-pane in records, got: %v", lines)
	}
}

// TestRealTmuxManager_CaptureWithEscapes verifies -e flag is passed with withEscapes=true.
func TestRealTmuxManager_CaptureWithEscapes(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("shell-script fake tmux requires unix")
	}
	recordFile := setupFakeTmux(t)

	outputFile := filepath.Join(t.TempDir(), "capture_output")
	if err := os.WriteFile(outputFile, []byte("content\n"), 0644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FAKE_TMUX_OUTPUT_FILE", outputFile)

	m := NewRealManager("")
	_, err := m.Capture("muster/mp-abc/0/0", true) // withEscapes=true
	if err != nil {
		t.Fatalf("Capture: %v", err)
	}

	lines := readRecordFile(t, recordFile)
	found := false
	for _, l := range lines {
		if strings.Contains(l, "capture-pane") && strings.Contains(l, "-e") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected capture-pane -e in records, got: %v", lines)
	}
}
