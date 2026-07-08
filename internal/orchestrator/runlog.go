package orchestrator

import (
	"encoding/base64"
	"io"
	"strings"
	"sync/atomic"

	"github.com/gitrgoliveira/muster/internal/ws"
)

// intPtr returns a pointer to i. Used for ws.Frame.StepIdx (*int) so the valid
// M2 value 0 is serialized rather than dropped by omitempty.
func intPtr(i int) *int { return &i }

// maxTailBytes / maxTailLines bound the per-step runlog tail retained for M6
// earlier-step summaries (FR-004). The tail is disposable run state, not a
// durable runlog store.
const (
	maxTailBytes = 8 * 1024
	maxTailLines = 100
)

// tailBuffer keeps the last ~maxTailBytes of a stream, trimmed to the last
// maxTailLines on read. Not safe for concurrent use — a single streamer
// goroutine owns it.
type tailBuffer struct {
	b []byte
}

func (t *tailBuffer) append(p []byte) {
	t.b = append(t.b, p...)
	// Trim generously (only when well over the cap) to avoid reslicing on every
	// chunk; String() applies the exact cap.
	if len(t.b) > 2*maxTailBytes {
		t.b = append([]byte(nil), t.b[len(t.b)-maxTailBytes:]...)
	}
}

func (t *tailBuffer) String() string {
	b := t.b
	if len(b) > maxTailBytes {
		b = b[len(b)-maxTailBytes:]
	}
	s := string(b)
	// Keep only the last maxTailLines.
	if lines := strings.Split(s, "\n"); len(lines) > maxTailLines {
		s = strings.Join(lines[len(lines)-maxTailLines:], "\n")
	}
	return s
}

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

	// tail retains the bounded end of this step's output; onDone (if set) is
	// called with it on EOF so the orchestrator can store it for M6 earlier-step
	// summaries (FR-004).
	tail   tailBuffer
	onDone func(tail string)
}

// stream reads from r until EOF or error, publishing each read as a runlog.line frame.
// Designed to be run in a goroutine.
func (s *runlogStreamer) stream(r io.Reader) {
	buf := make([]byte, 4096)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			s.tail.append(buf[:n]) // retain bounded tail for FR-004 summaries
			seq := s.seq.Add(1)
			if s.publish != nil {
				// Hot path (high-volume pane output) — avoid per-chunk allocations:
				//   - StepIdx: &s.stepIdx reuses the streamer's immutable field
				//     address instead of intPtr allocating a fresh int each frame.
				//     s.stepIdx never changes, so the shared pointer is safe.
				//   - Seq: Frame.Seq is a value (uint64), so no pointer alloc.
				//   - Data: EncodeToString copies buf[:n] into a fresh string
				//     synchronously (before the next Read overwrites buf), so no
				//     separate copy of the chunk is needed either.
				s.publish(ws.Frame{
					Type:    ws.EventRunlogLine,
					BeadID:  s.beadID,
					StepIdx: &s.stepIdx,
					Seq:     seq,
					// base64: pane output is raw terminal bytes and may not be
					// valid UTF-8; a Go string in JSON would corrupt those bytes.
					Data: base64.StdEncoding.EncodeToString(buf[:n]),
				})
			}
		}
		if err != nil {
			break
		}
	}
	if s.onDone != nil {
		s.onDone(s.tail.String())
	}
}
