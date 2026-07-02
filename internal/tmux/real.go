package tmux

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gitrgoliveira/muster/internal/shellquote"
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

// minTmuxMajor and minTmuxMinor are the supported tmux version floor (FR-007,
// spike-verified against 3.6b — see specs/003-m2-cli-adapter/research.md).
// Below this floor, pipe-pane/remain-on-exit/pane_dead_status semantics this
// package relies on may differ in ways that surface as confusing failures
// much later (mid-run) rather than as a clean startup fallback.
const minTmuxMajor, minTmuxMinor = 3, 2

// tmuxVersionRe extracts the first "<major>.<minor>" pair from `tmux -V`
// output. Covers the common formats ("tmux 3.6b", "tmux 3.2a", "tmux next-3.4")
// without needing to special-case vendor suffixes/prefixes.
var tmuxVersionRe = regexp.MustCompile(`(\d+)\.(\d+)`)

// Detect implements Manager. Returns the tmux version string, or an error if
// tmux is not available, its version can't be parsed, or it is below the
// supported floor (>= 3.2). Any error here is treated by the caller as "tmux
// unavailable" and falls back to the direct-exec transport (see
// cmd/muster/main.go), so failing closed on an unparseable version is the
// safe default — better an unnecessary fallback than silently trusting an
// unverified version.
func (m *RealManager) Detect() (string, error) {
	out, err := m.run("-V")
	if err != nil {
		return "", fmt.Errorf("tmux detect: %w", err)
	}
	version := strings.TrimSpace(out)
	major, minor, ok := parseTmuxVersion(version)
	if !ok {
		return "", fmt.Errorf("tmux detect: could not parse a version number from %q (want >= %d.%d)", version, minTmuxMajor, minTmuxMinor)
	}
	if major < minTmuxMajor || (major == minTmuxMajor && minor < minTmuxMinor) {
		return "", fmt.Errorf("tmux detect: version %q is below the supported floor (>= %d.%d)", version, minTmuxMajor, minTmuxMinor)
	}
	return version, nil
}

// parseTmuxVersion extracts the major.minor version from a `tmux -V` output
// string (e.g. "tmux 3.6b" -> 3, 6, true).
func parseTmuxVersion(s string) (major, minor int, ok bool) {
	m := tmuxVersionRe.FindStringSubmatch(s)
	if m == nil {
		return 0, 0, false
	}
	major, err1 := strconv.Atoi(m[1])
	minor, err2 := strconv.Atoi(m[2])
	if err1 != nil || err2 != nil {
		return 0, 0, false
	}
	return major, minor, true
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

	// Set environment variables by prefixing the command with env(1) and one
	// VAR=VALUE argument per entry. tmux exec's this argv directly (no shell is
	// involved unless argv[0] itself is a shell), so env(1) — not a shell — is
	// what applies the variables before exec'ing the real command.
	cmdArgv := buildEnvArgv(env, argv)

	// Spawn in two phases to avoid a race: if we passed the agent command to
	// new-session and only set remain-on-exit afterwards, a fast-failing
	// command could exit before remain-on-exit was set, destroying the pane and
	// losing its exit code. Instead: create the session with the default
	// (holder) shell, enable remain-on-exit, THEN respawn the pane with the real
	// command — so the command runs only once the pane is guaranteed to persist.
	spawnArgs := []string{
		"new-session", "-d",
		"-s", name,
		"-x", "220",
		"-y", "50",
	}
	if cwd != "" {
		spawnArgs = append(spawnArgs, "-c", cwd)
	}

	cmd := exec.Command(bin, spawnArgs...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("tmux new-session %q: %w\n%s", name, err, out)
	}

	// Set remain-on-exit BEFORE the agent command runs (race-free).
	if _, err := m.run("set-option", "-t", name, "remain-on-exit", "on"); err != nil {
		_ = m.Kill(name)
		return nil, fmt.Errorf("tmux set-option remain-on-exit: %w", err)
	}

	// Query the pane ID now, before respawn-pane. respawn-pane -k reuses the
	// existing pane (kills the process inside it and starts a new one) rather
	// than creating a new one, so the pane ID is stable across the respawn —
	// verified with `tmux display-message` before/after respawn-pane in
	// manual testing. Best-effort: a failure here must not fail the whole
	// Spawn (the pane ID is only used to populate the attach response's
	// informational `pane` field).
	// Only trust the output on success: m.run folds stderr into its output via
	// CombinedOutput, so on failure paneID would be an error string that then
	// leaks into the attach response's `pane` field. Leave it empty instead.
	paneID := ""
	if out, perr := m.run("display-message", "-p", "-t", name, "#{pane_id}"); perr == nil {
		paneID = strings.TrimSpace(out)
	}

	// Respawn the pane with the real command, now that it will persist on exit.
	respawnArgs := []string{"respawn-pane", "-k", "-t", name}
	if cwd != "" {
		respawnArgs = append(respawnArgs, "-c", cwd)
	}
	respawnArgs = append(respawnArgs, cmdArgv...)
	if _, err := m.run(respawnArgs...); err != nil {
		_ = m.Kill(name)
		return nil, fmt.Errorf("tmux respawn-pane %q: %w", name, err)
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
		Pane:      paneID,
		StartedAt: time.Now(),
	}, nil
}

// buildEnvArgv constructs the argv tmux exec's for the pane. If env is
// non-empty, it prefixes the command with env(1) and one VAR=VALUE argument
// per entry (no shell involved). If env is empty, argv is returned unchanged.
func buildEnvArgv(env, argv []string) []string {
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
	fifoDir, fifoPath, err := mkfifo()
	if err != nil {
		return nil, fmt.Errorf("tmux Pipe: create fifo: %w", err)
	}

	// Start tmux pipe-pane writing to the FIFO. tmux executes the pipe-cmd via
	// a shell, so the FIFO path must be shell-quoted to survive a TMPDIR that
	// contains spaces or shell metacharacters.
	//
	// Deliberately NOT using -o ("only open a pipe if none is already open"):
	// on restart recovery the pane may still carry a pipe from a previous
	// muster process (whose FIFO reader is long gone). With -o, tmux would keep
	// writing to that dead pipe and skip attaching to the new FIFO, so the
	// os.Open below would block forever waiting for a writer that never comes.
	// Plain pipe-pane closes any existing pipe and opens this one, guaranteeing
	// the new FIFO gets tmux as its writer.
	//
	// pipe-pane captures raw pane output, which includes echoed input (a pty
	// echoes keystrokes back into the pane) and terminal control sequences
	// (e.g. bracketed-paste markers) — muster does not strip any of it; the
	// client renders via a terminal emulator (D1).
	pipeCmd := "cat >> " + shellquote.Single(fifoPath)
	_, err = m.run("pipe-pane", "-t", name, pipeCmd)
	if err != nil {
		_ = os.RemoveAll(fifoDir)
		return nil, fmt.Errorf("tmux pipe-pane %q: %w", name, err)
	}

	// Open the read end of the FIFO. POSIX semantics: opening a FIFO read-only
	// blocks until some process opens the write end. Here the writer is the
	// `cat >> fifo` process tmux spawns for pipe-pane. `pipe-pane` returning
	// above only means tmux accepted the command — it spawns `cat`
	// asynchronously, so this open may block briefly until `cat` opens the FIFO.
	// That wait is bounded by how fast tmux starts the pipe process (typically
	// negligible), so we open synchronously rather than spinning up a goroutine.
	f, err := os.Open(fifoPath)
	if err != nil {
		_ = os.RemoveAll(fifoDir)
		return nil, fmt.Errorf("tmux Pipe: open fifo: %w", err)
	}

	// Return a ReadCloser that removes the FIFO + its temp dir on Close.
	return &fifoReader{File: f, dir: fifoDir}, nil
}

type fifoReader struct {
	*os.File
	dir string // temp dir holding the FIFO; removed (with the FIFO inside) on Close
}

func (r *fifoReader) Close() error {
	err := r.File.Close()
	_ = os.RemoveAll(r.dir) // removes the FIFO file AND its parent temp dir
	return err
}

// mkfifo creates a named pipe in a fresh temp directory and returns both the
// directory and the FIFO path. The caller must os.RemoveAll(dir) on cleanup.
func mkfifo() (dir, path string, err error) {
	dir, err = os.MkdirTemp("", "muster-pipe-*")
	if err != nil {
		return "", "", err
	}
	path = filepath.Join(dir, "pipe")
	if err := mkFifoSyscall(path); err != nil {
		_ = os.RemoveAll(dir)
		return "", "", err
	}
	return dir, path, nil
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

// Send implements Manager. Delivers keys as literal text via send-keys -l.
//
// Without -l, tmux send-keys treats its argument as a KEY NAME to look up
// first (e.g. "Enter", "Space", "Tab", "C-c") and only falls back to sending
// it as literal characters if no key name matches — so an answer that
// happens to collide with a recognized key name (e.g. sending the literal
// text "Enter") would press that key instead of typing the text, and there is
// no way for a caller to know in advance which inputs will collide. -l
// disables that lookup entirely and sends the exact bytes given, including
// any embedded newline (verified against both a plain pipe reader and an
// interactive bash+readline shell: a trailing "\n" in a single -l call is
// delivered and accepted as Enter — no separate C-m/Enter keypress needed).
func (m *RealManager) Send(name, keys string) error {
	_, err := m.run("send-keys", "-t", name, "-l", keys)
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
	// #{session_name} #{pane_id}: session names never contain whitespace
	// (the bead ID is validated via core.ValidBeadID before use — see
	// recovery.go), so a single split on the first space is safe and avoids a
	// per-session subprocess just to learn the pane id for recovered sessions.
	out, err := m.run("list-sessions", "-F", "#{session_name} #{pane_id}")
	if err != nil {
		// tmux returns exit 1 when there are no sessions — treat that as empty
		// list. The "no server running" text arrives via out (m.run folds
		// stderr into it through CombinedOutput), which is why the match is on
		// out, not err. err itself may be an *exec.ExitError or a resolve/start
		// error (e.g. "tmux not found") whose text we don't rely on here.
		if strings.Contains(out, "no server running") {
			return nil, nil
		}
		return nil, fmt.Errorf("tmux list-sessions: %w", err)
	}

	var sessions []Session
	scanner := bufio.NewScanner(strings.NewReader(out))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		name, pane, _ := strings.Cut(line, " ")
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
			Pane:    pane,
		})
	}
	return sessions, scanner.Err()
}
