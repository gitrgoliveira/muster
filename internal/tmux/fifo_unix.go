//go:build !windows

package tmux

import "golang.org/x/sys/unix"

// mkFifoSyscall creates a named pipe at path using the syscall.
func mkFifoSyscall(path string) error {
	return unix.Mkfifo(path, 0600)
}
