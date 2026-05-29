package orchestrator

import (
	"strings"
	"sync"
	"testing"

	"github.com/gitrgoliveira/muster/internal/ws"
)

func TestRunlogStreamer_SequentialSeq(t *testing.T) {
	var mu sync.Mutex
	var frames []ws.Frame
	pub := Publisher(func(f ws.Frame) {
		mu.Lock()
		frames = append(frames, f)
		mu.Unlock()
	})

	s := &runlogStreamer{
		beadID:  "mp-abc",
		stepIdx: 0,
		publish: pub,
	}

	// Feed 5 chunks.
	input := strings.Repeat("chunk\n", 5)
	r := strings.NewReader(input)
	s.stream(r)

	mu.Lock()
	defer mu.Unlock()

	if len(frames) == 0 {
		t.Fatal("no frames emitted")
	}

	// Verify seq is monotonically increasing.
	var lastSeq uint64
	for _, f := range frames {
		if f.Type != ws.EventRunlogLine {
			t.Errorf("unexpected event type %q", f.Type)
		}
		if f.BeadID != "mp-abc" {
			t.Errorf("BeadID want mp-abc got %q", f.BeadID)
		}
		if f.Seq <= lastSeq {
			t.Errorf("seq not monotonically increasing: got %d after %d", f.Seq, lastSeq)
		}
		lastSeq = f.Seq
	}
}

func TestRunlogStreamer_PreservesRawBytes(t *testing.T) {
	var frames []ws.Frame
	pub := Publisher(func(f ws.Frame) {
		frames = append(frames, f)
	})

	s := &runlogStreamer{beadID: "mp-abc", stepIdx: 0, publish: pub}

	// Include ANSI escape sequences (raw bytes — plan D1).
	ansiData := "\x1b[32mGreen text\x1b[0m\n"
	r := strings.NewReader(ansiData)
	s.stream(r)

	if len(frames) == 0 {
		t.Fatal("no frames emitted")
	}

	// Data must be preserved as-is (no stripping).
	combined := ""
	for _, f := range frames {
		combined += f.Data
	}
	if !strings.Contains(combined, "\x1b[32m") {
		t.Error("ANSI escape should be preserved in runlog.line data")
	}
}

func TestIsMusterMarker(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"normal output", false},
		{"<muster:subbead foo>", true},
		{"<muster:checkpoint>", true},
		{"no marker here", false},
		{"<muster:", true}, // partial prefix
	}
	for _, tt := range tests {
		got := isMusterMarker(tt.input)
		if got != tt.want {
			t.Errorf("isMusterMarker(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestRunlogStreamer_NilPublish(t *testing.T) {
	// Should not panic when publish is nil.
	s := &runlogStreamer{beadID: "mp-abc", stepIdx: 0, publish: nil}
	r := strings.NewReader("some data\n")
	// Should not panic.
	s.stream(r)
}
