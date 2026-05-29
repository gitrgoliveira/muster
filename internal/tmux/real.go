package tmux

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// RealManager implements Manager by shelling out to the `tmux` binary.
// It uses the default tmux socket so the user can attach with a plain
// `tmux attach -t muster/<bead>/0/0` command.
type RealManager struct {
	bin string // path to tmux binary (empty = resolve from PATH)
}

// NewRealManager creates a new RealManager.
// bin may be empty to auto-discover tmux on PATH.
func NewRealManager(bin string) *RealManager {
	return &RealManager{bin: bin}
}

// resolveBin returns the tmux binary path.
func (m *RealManager) resolveBin() (string, error) {
	if m.bin != "" {
		return m.bin, nil
	}
	return exec.LookPath("tmux")
}

// run executes a tmux subcommand and returns combined output.
func (m *RealManager) run(args ...string) (string, error) {
	bin, err := m.resolveBin()
	if err != nil {
		return "", fmt.Errorf("tmux not found: %w", err)
	}
	cmd := exec.Command(bin, args...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// Detect implements Manager. Returns the tmux version string or an error if
// tmux is not available.
func (m *RealManager) Detect() (string, error) {
	out, err := m.run("-V")
	if err != nil {
		return "", fmt.Errorf("tmux detect: %w", err)
	}
	version := strings.TrimSpace(out)
	return version, nil
}

// Spawn implements Manager. Creates a new tmux session on the default socket
// with remain-on-exit on, then runs the provided argv in the pane.
// argv must have at least one element.
func (m *RealManager) Spawn(name, cwd string, env, argv []string) (*Session, error) {
	bin, err := m.resolveBin()
	if err != nil {
		return nil, fmt.Errorf("tmux not found: %w", err)
	}

	if len(argv) == 0 {
		return nil, fmt.Errorf("tmux Spawn: argv must not be empty")
	}

	// Set environment variables: each as VAR=VALUE passed to env(1) in the shell.
	// We build a shell command that sets env vars and execs the command.
	shellCmd := buildShellCmd(env, argv)

	// tmux new-session -d -s <name> -x 220 -y 50 <shell> -c <cwd>
	spawnArgs := []string{
		"new-session", "-d",
		"-s", name,
		"-x", "220",
		"-y", "50",
	}
	if cwd != "" {
		spawnArgs = append(spawnArgs, "-c", cwd)
	}
	spawnArgs = append(spawnArgs, shellCmd...)

	cmd := exec.Command(bin, spawnArgs...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("tmux new-session %q: %w\n%s", name, err, out)
	}

	// Set remain-on-exit on for the window so pane survives process death.
	if _, err := m.run("set-option", "-t", name, "remain-on-exit", "on"); err != nil {
		// Non-fatal: kill the session and report.
		_ = m.Kill(name)
		return nil, fmt.Errorf("tmux set-option remain-on-exit: %w", err)
	}

	// Parse name to fill Session fields.
	beadID, stepIdx, loop, parseErr := ParseSessionName(name)
	if parseErr != nil {
		// Not a fatal error — name may not follow our convention in tests.
		beadID = name
	}

	return &Session{
		Name:      name,
		BeadID:    beadID,
		StepIdx:   stepIdx,
		Loop:      loop,
		StartedAt: time.Now(),
	}, nil
}

// buildShellCmd constructs the argv to pass to tmux new-session.
// If env is non-empty, wraps the command with env(1).
func buildShellCmd(env, argv []string) []string {
	if len(env) == 0 {
		return argv
	}
	// Prepend "env" with each VAR=VALUE entry.
	result := []string{"env"}
	result = append(result, env...)
	result = append(result, argv...)
	return result
}

// Pipe implements Manager. Attaches a pipe-pane to the session and returns a
// ReadCloser that receives raw terminal bytes (ANSI preserved — plan D1).
// The pipe is backed by a named pipe (FIFO) written to by tmux pipe-pane.
func (m *RealManager) Pipe(name string) (io.ReadCloser, error) {
	// Create a named pipe (FIFO).
	fifoPath, err := mkfifo()
	if err != nil {
		return nil, fmt.Errorf("tmux Pipe: create fifo: %w", err)
	}

	// Start tmux pipe-pane writing to the FIFO.
	// pipe-pane -o captures output only (not echoed input for clean streaming).
	pipeCmd := fmt.Sprintf("cat >> %s", fifoPath)
	_, err = m.run("pipe-pane", "-t", name, "-o", pipeCmd)
	if err != nil {
		_ = os.Remove(fifoPath)
		return nil, fmt.Errorf("tmux pipe-pane %q: %w", name, err)
	}

	// Open the read end of the FIFO. This blocks until the write end is open
	// (tmux has started writing), so open in a goroutine and surface errors.
	f, err := os.Open(fifoPath)
	if err != nil {
		_ = os.Remove(fifoPath)
		return nil, fmt.Errorf("tmux Pipe: open fifo: %w", err)
	}

	// Return a ReadCloser that removes the FIFO on Close.
	return &fifoReader{File: f, path: fifoPath}, nil
}

type fifoReader struct {
	*os.File
	path string
}

func (r *fifoReader) Close() error {
	err := r.File.Close()
	_ = os.Remove(r.path)
	return err
}

// mkfifo creates a named pipe in a temp directory and returns its path.
func mkfifo() (string, error) {
	dir, err := os.MkdirTemp("", "muster-pipe-*")
	if err != nil {
		return "", err
	}
	path := dir + "/pipe"
	if err := mkFifoSyscall(path); err != nil {
		_ = os.RemoveAll(dir)
		return "", err
	}
	return path, nil
}

// Capture implements Manager. Returns pane scrollback via capture-pane.
// withEscapes=true preserves ANSI codes for faithful catch-up rendering.
func (m *RealManager) Capture(name string, withEscapes bool) (string, error) {
	args := []string{"capture-pane", "-p", "-S", "-", "-t", name}
	if withEscapes {
		args = append(args, "-e")
	}
	out, err := m.run(args...)
	if err != nil {
		return "", fmt.Errorf("tmux capture-pane %q: %w", name, err)
	}
	return out, nil
}

// Send implements Manager. Delivers keystrokes via send-keys.
func (m *RealManager) Send(name, keys string) error {
	_, err := m.run("send-keys", "-t", name, keys)
	if err != nil {
		return fmt.Errorf("tmux send-keys %q: %w", name, err)
	}
	return nil
}

// Attach implements Manager. Returns the user-facing attach command string.
func (m *RealManager) Attach(name string) (string, error) {
	return "tmux attach -t " + name, nil
}

// DeadStatus implements Manager. Queries whether the pane is dead and its exit code.
// Uses `tmux display-message -p '#{pane_dead} #{pane_dead_status}'`.
func (m *RealManager) DeadStatus(name string) (code int, dead bool, err error) {
	out, runErr := m.run("display-message", "-p", "-t", name, "#{pane_dead} #{pane_dead_status}")
	if runErr != nil {
		return 0, false, fmt.Errorf("tmux display-message %q: %w", name, runErr)
	}

	parts := strings.Fields(strings.TrimSpace(out))
	if len(parts) == 0 {
		return 0, false, fmt.Errorf("tmux DeadStatus: empty output for %q", name)
	}

	deadVal, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, false, fmt.Errorf("tmux DeadStatus: parse pane_dead %q: %w", parts[0], err)
	}
	dead = deadVal == 1

	if dead {
		if len(parts) >= 2 && parts[1] != "" {
			code, err = strconv.Atoi(parts[1])
			if err != nil {
				return 0, true, fmt.Errorf("tmux DeadStatus: parse pane_dead_status %q: %w", parts[1], err)
			}
		} else {
			// Pane is dead but tmux reports no numeric exit status — this happens
			// when the process was killed by a signal (no $? code). Treat as a
			// failure (non-zero) so watchRun/finishRun do not mark it as success.
			code = -1
		}
	}

	return code, dead, nil
}

// Kill implements Manager. Terminates the named session.
func (m *RealManager) Kill(name string) error {
	_, err := m.run("kill-session", "-t", name)
	if err != nil {
		return fmt.Errorf("tmux kill-session %q: %w", name, err)
	}
	return nil
}

// List implements Manager. Returns all muster-owned sessions from the default socket.
func (m *RealManager) List() ([]Session, error) {
	out, err := m.run("list-sessions", "-F", "#{session_name}")
	if err != nil {
		// tmux returns exit 1 when there are no sessions — treat that as empty list.
		if strings.Contains(err.Error(), "no server running") ||
			strings.Contains(out, "no server running") {
			return nil, nil
		}
		return nil, fmt.Errorf("tmux list-sessions: %w", err)
	}

	var sessions []Session
	scanner := bufio.NewScanner(strings.NewReader(out))
	for scanner.Scan() {
		name := strings.TrimSpace(scanner.Text())
		if !IsMusterSession(name) {
			continue
		}
		beadID, stepIdx, loop, parseErr := ParseSessionName(name)
		if parseErr != nil {
			continue // skip malformed names
		}
		sessions = append(sessions, Session{
			Name:    name,
			BeadID:  beadID,
			StepIdx: stepIdx,
			Loop:    loop,
		})
	}
	return sessions, scanner.Err()
}
