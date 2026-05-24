// Package bdshell provides a typed wrapper around the bd CLI binary.
package bdshell

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// ErrCLIMissing is returned when the bd binary cannot be located.
var ErrCLIMissing = errors.New("bd CLI not found")

// CLIError is returned when bd exits with a non-zero code.
type CLIError struct {
	ExitCode int
	Stderr   string
}

func (e *CLIError) Error() string {
	return fmt.Sprintf("bd exited %d: %s", e.ExitCode, e.Stderr)
}

// Result holds the captured output from a bd invocation.
type Result struct {
	Stdout string
	Stderr string
}

// CLI wraps the bd binary with a fixed beads directory.
type CLI struct {
	Path     string
	BeadsDir string
	Timeout  time.Duration
}

// NewCLI constructs a CLI, resolving bdBin via exec.LookPath if empty.
// Returns ErrCLIMissing if the binary is not found.
func NewCLI(bdBin, beadsDir string) (*CLI, error) {
	if bdBin == "" {
		resolved, err := exec.LookPath("bd")
		if err != nil {
			return nil, ErrCLIMissing
		}
		bdBin = resolved
	} else {
		// Validate that an explicitly supplied path is executable.
		if _, err := exec.LookPath(bdBin); err != nil {
			return nil, ErrCLIMissing
		}
	}
	return &CLI{
		Path:     bdBin,
		BeadsDir: beadsDir,
		Timeout:  30 * time.Second,
	}, nil
}

// Run executes bd with the given args, returning the captured output.
// The subprocess environment is minimal: BEADS_DIR, PATH, HOME only.
// On non-zero exit, returns a *CLIError with the exit code.
func (c *CLI) Run(ctx context.Context, args ...string) (Result, error) {
	timeout := c.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, c.Path, args...) //nolint:gosec
	cmd.Env = buildEnv(c.BeadsDir)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	res := Result{
		Stdout: stdout.String(),
		Stderr: truncate(stripANSI(stderr.String()), 512),
	}
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return res, fmt.Errorf("bd timed out: %w", ctx.Err())
		}
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return res, &CLIError{ExitCode: ee.ExitCode(), Stderr: res.Stderr}
		}
		return res, err
	}
	return res, nil
}

// RunJSON calls Run and unmarshals stdout JSON into dst.
func (c *CLI) RunJSON(ctx context.Context, dst any, args ...string) error {
	res, err := c.Run(ctx, args...)
	if err != nil {
		return err
	}
	if err := json.Unmarshal([]byte(res.Stdout), dst); err != nil {
		return fmt.Errorf("bd output parse error: %w", err)
	}
	return nil
}

// RunVoid calls Run and discards output.
func (c *CLI) RunVoid(ctx context.Context, args ...string) error {
	_, err := c.Run(ctx, args...)
	return err
}

func buildEnv(beadsDir string) []string {
	env := []string{"BEADS_DIR=" + beadsDir}
	if p := envGet("PATH"); p != "" {
		env = append(env, "PATH="+p)
	}
	if home := envGet("HOME"); home != "" {
		env = append(env, "HOME="+home)
	}
	return env
}

// stripANSI removes ANSI escape sequences from s.
func stripANSI(s string) string {
	var b strings.Builder
	inEscape := false
	for _, r := range s {
		if inEscape {
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
				inEscape = false
			}
			continue
		}
		if r == '\x1b' {
			inEscape = true
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// truncate caps s to n bytes.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

func envGet(key string) string {
	for _, pair := range os.Environ() {
		if k, v, ok := strings.Cut(pair, "="); ok && k == key {
			return v
		}
	}
	return ""
}
