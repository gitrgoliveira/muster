//go:build windows

package tmux

import "errors"

// mkFifoSyscall is a stub on Windows. The tmux RealManager is only viable
// where tmux itself runs (POSIX); on Windows muster falls back to
// FallbackManager (no tmux), which does not call mkfifo. This stub exists
// so the tmux package compiles cross-platform.
func mkFifoSyscall(_ string) error {
	return errors.New("tmux pipe: named pipes are not supported on Windows (fallback transport is used instead)")
}
