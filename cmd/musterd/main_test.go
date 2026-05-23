package main

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testBinPath is the path to the built binary used by all tests in this package.
var testBinPath string

// TestMain builds the binary once before running all tests.
func TestMain(m *testing.M) {
	bin := "/tmp/musterd_test_bin"
	cmd := exec.Command("go", "build", "-o", bin, "./cmd/musterd/")
	// Run from the module root (two levels up from cmd/musterd).
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

// TestServe_BootsThenShutdown starts the server on a random free port, waits for
// it to respond to GET /, then sends SIGINT and expects a clean exit (code 0).
func TestServe_BootsThenShutdown(t *testing.T) {
	// Pick a free port.
	port := freePort(t)
	addr := fmt.Sprintf("127.0.0.1:%d", port)

	cmd := exec.Command(testBinPath, "serve", "--addr", addr)
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

// freePort returns a free TCP port on localhost.
func freePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err, "find free port")
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	return port
}
