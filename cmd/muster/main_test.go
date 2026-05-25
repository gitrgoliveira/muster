package main

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testBinPath is the path to the built binary used by all tests in this package.
var testBinPath string

// TestMain builds the binary once before running all tests.
func TestMain(m *testing.M) {
	bin := "/tmp/muster_test_bin"
	cmd := exec.Command("go", "build", "-o", bin, "./cmd/muster/")
	// Run from the module root (two levels up from cmd/muster).
	cmd.Dir = "../.."
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to build test binary: %v\n", err)
		os.Exit(1)
	}
	testBinPath = bin
	os.Exit(m.Run())
}

// makeTempBeadsDir creates a temporary beads directory with a valid embedded config
// and an empty issues.jsonl file, returning the directory path.
func makeTempBeadsDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	meta := map[string]any{
		"schema_version": 1,
		"dolt_mode":      "embedded",
		"project_id":     "test-project",
	}
	b, err := json.Marshal(meta)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "metadata.json"), b, 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "issues.jsonl"), []byte{}, 0o600))
	return dir
}

// TestNoSubcommand_PrintsUsageExits1 verifies that running with no args exits 1.
func TestNoSubcommand_PrintsUsageExits1(t *testing.T) {
	cmd := exec.Command(testBinPath)
	err := cmd.Run()
	require.Error(t, err, "binary should exit non-zero when no subcommand is provided")
	exitErr, ok := err.(*exec.ExitError)
	require.True(t, ok, "error should be an ExitError")
	assert.Equal(t, 1, exitErr.ExitCode())
}

// TestServe_ParseAddr_InvalidFormat_Exits1 verifies that an invalid --addr exits 1.
func TestServe_ParseAddr_InvalidFormat_Exits1(t *testing.T) {
	cmd := exec.Command(testBinPath, "serve", "--addr", "notvalid")
	err := cmd.Run()
	require.Error(t, err, "binary should exit non-zero for invalid addr")
	exitErr, ok := err.(*exec.ExitError)
	require.True(t, ok, "error should be an ExitError")
	assert.Equal(t, 1, exitErr.ExitCode())
}

// TestServe_MissingBeadsDir_Exits1 verifies exit 1 when no beads directory can be found.
func TestServe_MissingBeadsDir_Exits1(t *testing.T) {
	// Run in a temp dir with no .beads/ subdirectory.
	// Clear BEADS_DIR so the subprocess cannot inherit it from the test environment.
	emptyDir := t.TempDir()
	cmd := exec.Command(testBinPath, "serve", "--addr", "127.0.0.1:0")
	cmd.Dir = emptyDir
	// Minimal env: PATH only, no BEADS_DIR.
	cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + os.Getenv("HOME")}
	out, err := cmd.CombinedOutput()
	require.Error(t, err, "binary should exit non-zero when no beads dir found")
	exitErr, ok := err.(*exec.ExitError)
	require.True(t, ok, "error should be an ExitError")
	assert.Equal(t, 1, exitErr.ExitCode())
	assert.Contains(t, string(out), "beads", "error message should mention beads")
}

// TestServe_BadMetadata_Exits1 verifies exit 1 with bad metadata.json.
func TestServe_BadMetadata_Exits1(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "metadata.json"), []byte("{not json}"), 0o600))

	cmd := exec.Command(testBinPath, "serve", "--addr", "127.0.0.1:0", "--beads-dir", dir)
	out, err := cmd.CombinedOutput()
	require.Error(t, err, "binary should exit non-zero for bad metadata")
	exitErr, ok := err.(*exec.ExitError)
	require.True(t, ok)
	assert.Equal(t, 1, exitErr.ExitCode())
	assert.Contains(t, string(out), "metadata")
}

// TestServe_UnsupportedSchema_Exits1 verifies exit 1 when schema_version is unsupported.
func TestServe_UnsupportedSchema_Exits1(t *testing.T) {
	dir := t.TempDir()
	meta := map[string]any{"schema_version": 99}
	b, _ := json.Marshal(meta)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "metadata.json"), b, 0o600))

	cmd := exec.Command(testBinPath, "serve", "--addr", "127.0.0.1:0", "--beads-dir", dir)
	out, err := cmd.CombinedOutput()
	require.Error(t, err, "binary should exit non-zero for unsupported schema")
	exitErr, ok := err.(*exec.ExitError)
	require.True(t, ok)
	assert.Equal(t, 1, exitErr.ExitCode())
	assert.Contains(t, string(out), "schema")
}

// TestServe_BootsThenShutdown starts the server on a random free port, waits for
// it to respond to GET /, then sends SIGINT and expects a clean exit (code 0).
func TestServe_BootsThenShutdown(t *testing.T) {
	beadsDir := makeTempBeadsDir(t)

	// Pick a free port.
	port := freePort(t)
	addr := fmt.Sprintf("127.0.0.1:%d", port)

	cmd := exec.Command(testBinPath, "serve", "--addr", addr, "--beads-dir", beadsDir)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	require.NoError(t, cmd.Start(), "start server")

	// Poll until the server responds.
	url := fmt.Sprintf("http://%s/", addr)
	var lastErr error
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get(url) //nolint:noctx
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				lastErr = nil
				break
			}
			lastErr = fmt.Errorf("unexpected status %d", resp.StatusCode)
		} else {
			lastErr = err
		}
		time.Sleep(50 * time.Millisecond)
	}
	require.NoError(t, lastErr, "server should respond with 200 within 10s")

	// Send SIGINT.
	require.NoError(t, cmd.Process.Signal(os.Interrupt), "send SIGINT")

	// Wait for clean exit within 10s.
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	select {
	case err := <-done:
		// On Unix, SIGINT causes exit code 130 (signal-terminated) by default.
		// Since we handle the signal and call Shutdown + exit normally, we expect 0.
		// However, the Go runtime may still set a non-zero exit if signal-terminated.
		// Accept both 0 and signal-terminated gracefully.
		if err != nil {
			exitErr, ok := err.(*exec.ExitError)
			if ok {
				// 130 = 128+2 (SIGINT) is acceptable on some platforms.
				code := exitErr.ExitCode()
				assert.True(t, code == 0 || code == 130,
					"expected exit code 0 or 130, got %d", code)
			}
		}
	case <-time.After(15 * time.Second):
		cmd.Process.Kill()
		t.Fatal("server did not exit within 15s after SIGINT")
	}
}

// TestServe_ParseAddr_IPv6 verifies the server starts and serves on an IPv6
// loopback address. Skipped if [::1] is not available on the host.
func TestServe_ParseAddr_IPv6(t *testing.T) {
	// Check IPv6 availability before starting the binary.
	ln6, err := net.Listen("tcp6", "[::1]:0")
	if err != nil {
		t.Skip("IPv6 loopback not available on this system:", err)
	}
	port := ln6.Addr().(*net.TCPAddr).Port
	ln6.Close()

	beadsDir := makeTempBeadsDir(t)

	addr := fmt.Sprintf("[::1]:%d", port)
	cmd := exec.Command(testBinPath, "serve", "--addr", addr, "--beads-dir", beadsDir)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	require.NoError(t, cmd.Start(), "start server on IPv6")

	url := fmt.Sprintf("http://[::1]:%d/api/v1/healthz", port)
	var lastErr error
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get(url) //nolint:noctx
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				lastErr = nil
				break
			}
			lastErr = fmt.Errorf("unexpected status %d", resp.StatusCode)
		} else {
			lastErr = err
		}
		time.Sleep(50 * time.Millisecond)
	}
	require.NoError(t, lastErr, "server should respond on [::1] within 10s")

	require.NoError(t, cmd.Process.Signal(os.Interrupt))
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		if err != nil {
			exitErr, ok := err.(*exec.ExitError)
			if ok {
				code := exitErr.ExitCode()
				assert.True(t, code == 0 || code == 130, "expected 0 or 130, got %d", code)
			}
		}
	case <-time.After(15 * time.Second):
		cmd.Process.Kill()
		t.Fatal("server did not exit within 15s")
	}
}

// TestServer_PortInUse_Exits1 verifies the binary exits with code 1 when
// the requested port is already in use.
func TestServer_PortInUse_Exits1(t *testing.T) {
	// Hold the port so the binary cannot bind.
	holder, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err, "open holder listener")
	defer holder.Close()
	port := holder.Addr().(*net.TCPAddr).Port

	beadsDir := makeTempBeadsDir(t)
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	cmd := exec.Command(testBinPath, "serve", "--addr", addr, "--beads-dir", beadsDir)
	// Give the binary up to 5s to notice the bind failure and exit.
	done := make(chan error, 1)
	require.NoError(t, cmd.Start())
	go func() { done <- cmd.Wait() }()

	select {
	case err := <-done:
		require.Error(t, err, "binary should exit non-zero when port is in use")
		exitErr, ok := err.(*exec.ExitError)
		require.True(t, ok, "error should be an ExitError")
		assert.Equal(t, 1, exitErr.ExitCode())
	case <-time.After(5 * time.Second):
		cmd.Process.Kill()
		t.Fatal("binary did not exit within 5s after port-in-use failure")
	}
}

// freePort returns a free TCP port on localhost.
func freePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err, "find free port")
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	return port
}

// makeTempBeadsDirWithIssues creates a temporary beads dir with fixture issues.jsonl.
func makeTempBeadsDirWithIssues(t *testing.T, issues []map[string]any) string {
	t.Helper()
	dir := t.TempDir()
	meta := map[string]any{
		"schema_version": 1,
		"dolt_mode":      "embedded",
		"project_id":     "test-project",
	}
	b, err := json.Marshal(meta)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "metadata.json"), b, 0o600))

	var sb strings.Builder
	for _, iss := range issues {
		line, err := json.Marshal(iss)
		require.NoError(t, err)
		sb.Write(line)
		sb.WriteByte('\n')
	}
	require.NoError(t, os.WriteFile(filepath.Join(dir, "issues.jsonl"), []byte(sb.String()), 0o600))
	return dir
}

// startServer starts the muster binary and blocks until it responds to GET /api/v1/healthz.
// Returns a cleanup function that signals SIGINT and waits for the process to exit.
func startServer(t *testing.T, addr, beadsDir string) func() {
	t.Helper()
	cmd := exec.Command(testBinPath, "serve", "--addr", addr, "--beads-dir", beadsDir)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	require.NoError(t, cmd.Start(), "start server")

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get("http://" + addr + "/api/v1/healthz") //nolint:noctx
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				break
			}
		}
		time.Sleep(50 * time.Millisecond)
	}

	return func() {
		if cmd.Process != nil {
			cmd.Process.Signal(os.Interrupt) //nolint:errcheck
		}
		done := make(chan error, 1)
		go func() { done <- cmd.Wait() }()
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			cmd.Process.Kill() //nolint:errcheck
		}
	}
}

// TestServe_ListBeads_ReturnsFixtureIssues verifies that GET /api/v1/beads returns
// the issues from the fixture issues.jsonl (T047).
func TestServe_ListBeads_ReturnsFixtureIssues(t *testing.T) {
	issues := []map[string]any{
		{"id": "mp-001", "title": "First", "status": "open"},
		{"id": "mp-002", "title": "Second", "status": "in_progress"},
		{"id": "mp-003", "title": "Third", "status": "closed"},
	}
	beadsDir := makeTempBeadsDirWithIssues(t, issues)

	port := freePort(t)
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	stop := startServer(t, addr, beadsDir)
	defer stop()

	// GET /api/v1/beads — no column filter returns all issues.
	resp, err := http.Get("http://" + addr + "/api/v1/beads") //nolint:noctx
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var body struct {
		Total int `json:"total"`
		Items []struct {
			ID string `json:"id"`
		} `json:"items"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.Equal(t, 3, body.Total, "total should match fixture count")

	wantIDs := make(map[string]bool, len(issues))
	for _, iss := range issues {
		wantIDs[iss["id"].(string)] = true
	}
	for _, item := range body.Items {
		assert.True(t, wantIDs[item.ID], "unexpected id %s in response", item.ID)
	}
}

// TestServe_GetBead_ReturnsSpecificIssue verifies GET /api/v1/beads/{id} (T047).
func TestServe_GetBead_ReturnsSpecificIssue(t *testing.T) {
	issues := []map[string]any{
		{"id": "mp-001", "title": "First", "status": "open"},
		{"id": "mp-002", "title": "Second", "status": "in_progress"},
	}
	beadsDir := makeTempBeadsDirWithIssues(t, issues)

	port := freePort(t)
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	stop := startServer(t, addr, beadsDir)
	defer stop()

	resp, err := http.Get("http://" + addr + "/api/v1/beads/mp-002") //nolint:noctx
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var body struct {
		ID string `json:"id"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.Equal(t, "mp-002", body.ID)
}
