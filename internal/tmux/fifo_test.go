//go:build !windows

package tmux

import (
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

// These cover the FIFO plumbing deterministically (no tmux server needed):
// mkFifoSyscall, mkfifo, and fifoReader.Close.

func TestMkFifoSyscall_CreatesNamedPipe(t *testing.T) {
	p := filepath.Join(t.TempDir(), "f")
	if err := mkFifoSyscall(p); err != nil {
		t.Fatalf("mkFifoSyscall: %v", err)
	}
	fi, err := os.Stat(p)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if fi.Mode()&fs.ModeNamedPipe == 0 {
		t.Errorf("want named pipe, got mode %v", fi.Mode())
	}
}

func TestMkfifo_CreatesPipeInTempDir(t *testing.T) {
	path, err := mkfifo()
	if err != nil {
		t.Fatalf("mkfifo: %v", err)
	}
	defer func() { _ = os.RemoveAll(filepath.Dir(path)) }()

	fi, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if fi.Mode()&fs.ModeNamedPipe == 0 {
		t.Errorf("want named pipe, got mode %v", fi.Mode())
	}
}

func TestFifoReader_CloseRemovesPath(t *testing.T) {
	p := filepath.Join(t.TempDir(), "pipe")
	if err := mkFifoSyscall(p); err != nil {
		t.Fatalf("mkFifoSyscall: %v", err)
	}
	// O_NONBLOCK so opening the read end doesn't block waiting for a writer.
	f, err := os.OpenFile(p, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		t.Fatalf("open fifo: %v", err)
	}
	fr := &fifoReader{File: f, path: p}
	if err := fr.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if _, err := os.Stat(p); !os.IsNotExist(err) {
		t.Errorf("fifo should be removed after Close; stat err=%v", err)
	}
}
