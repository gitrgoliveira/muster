package wt_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// initGitRepo creates a temporary git repository with an initial commit and
// returns its absolute path. Mirrors the helper in internal/worktree.
func initGitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		// Hermetic identity: do not rely on the developer's global git config.
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=Test",
			"GIT_AUTHOR_EMAIL=test@test.com",
			"GIT_COMMITTER_NAME=Test",
			"GIT_COMMITTER_EMAIL=test@test.com",
		)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}

	run("init")
	run("config", "user.email", "test@test.com")
	run("config", "user.name", "Test")

	// Write an initial commit so the repo has a HEAD.
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# test\n"), 0o644); err != nil {
		t.Fatalf("write README: %v", err)
	}
	run("add", ".")
	run("commit", "-m", "initial commit")

	return dir
}

// runGitCmd runs a git command in dir with hermetic identity env and returns
// the combined output. Fatals on non-zero exit.
func runGitCmd(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=Test",
		"GIT_AUTHOR_EMAIL=test@test.com",
		"GIT_COMMITTER_NAME=Test",
		"GIT_COMMITTER_EMAIL=test@test.com",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v in %s: %v\n%s", args, dir, err, out)
	}
	return string(out)
}

// jjAvailable returns true if jj is available on PATH.
func jjAvailable() bool {
	_, err := exec.LookPath("jj")
	return err == nil
}

// fakeJJPath returns the absolute path to the fake_jj.sh test binary bundled
// under internal/wt/testdata/.
func fakeJJPath(t *testing.T) string {
	t.Helper()
	abs, err := filepath.Abs(filepath.Join("testdata", "fake_jj.sh"))
	if err != nil {
		t.Fatalf("fakeJJPath: %v", err)
	}
	if _, err := os.Stat(abs); err != nil {
		t.Fatalf("fake_jj.sh not found at %s: %v", abs, err)
	}
	return abs
}

// addFakeJJToBinDir creates a symlink "jj" → fake_jj.sh in binDir and
// prepends binDir to PATH for the duration of the test.
func addFakeJJToBinDir(t *testing.T, binDir string) {
	t.Helper()
	dest := filepath.Join(binDir, "jj")
	if err := os.Symlink(fakeJJPath(t), dest); err != nil {
		t.Fatalf("symlink fake_jj: %v", err)
	}
	oldPath := os.Getenv("PATH")
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+oldPath)
}
