package orchestrator

import (
	"bufio"
	"io"
	"sync/atomic"

	"github.com/gitrgoliveira/muster/internal/ws"
)

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
					StepIdx: s.stepIdx,
					Seq:     seq,
					Data:    string(data),
				})
			}
		}
		if err != nil {
			break
		}
	}
}

// streamLines reads line-by-line, emitting one frame per line.
// Used for structured output (not the default raw-bytes path).
func (s *runlogStreamer) streamLines(r io.Reader) {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := scanner.Text() + "\n"
		seq := s.seq.Add(1)
		if s.publish != nil {
			s.publish(ws.Frame{
				Type:    ws.EventRunlogLine,
				BeadID:  s.beadID,
				StepIdx: s.stepIdx,
				Seq:     seq,
				Data:    line,
			})
		}
	}
}

// musterMarkerTag prefix for special markers emitted in pane output.
// FR-020: treated as inert (logged, not acted upon in M2).
const musterMarkerTag = "<muster:"

// isMusterMarker returns true if data contains a muster marker tag.
// FR-020: markers are inert in M2.
func isMusterMarker(data string) bool {
	for i := 0; i < len(data)-len(musterMarkerTag)+1; i++ {
		if data[i:i+len(musterMarkerTag)] == musterMarkerTag {
			return true
		}
	}
	return false
}
