package orchestrator

import (
	"encoding/base64"
	"io"
	"sync/atomic"

	"github.com/gitrgoliveira/muster/internal/ws"
)

// intPtr returns a pointer to i. Used for ws.Frame.StepIdx (*int) so the valid
// M2 value 0 is serialized rather than dropped by omitempty.
func intPtr(i int) *int { return &i }

// runlogStreamer reads raw bytes from a pane pipe and fans them to the WS hub
// as sequential runlog.line frames.
//
// Design D1: bytes are raw terminal sequences (ANSI preserved); the UI renders
// them via a terminal emulator (e.g. xterm.js). No server-side stripping.
type runlogStreamer struct {
	beadID  string
	stepIdx int
	seq     atomic.Uint64 // monotonic counter per (bead, step) run
	publish Publisher
}

// stream reads from r until EOF or error, publishing each read as a runlog.line frame.
// Designed to be run in a goroutine.
func (s *runlogStreamer) stream(r io.Reader) {
	buf := make([]byte, 4096)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			data := make([]byte, n)
			copy(data, buf[:n])
			seq := s.seq.Add(1)
			if s.publish != nil {
				s.publish(ws.Frame{
					Type:    ws.EventRunlogLine,
					BeadID:  s.beadID,
					StepIdx: intPtr(s.stepIdx),
					Seq:     &seq,
					// base64: pane output is raw terminal bytes and may not be
					// valid UTF-8; a Go string in JSON would corrupt those bytes.
					Data: base64.StdEncoding.EncodeToString(data),
				})
			}
		}
		if err != nil {
			break
		}
	}
}
