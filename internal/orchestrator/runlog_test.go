package orchestrator

import (
	"encoding/base64"
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

	// Data is base64-encoded raw bytes (terminal output may be non-UTF-8).
	// Decode and verify the ANSI escape survived round-trip (no stripping).
	var combined []byte
	for _, f := range frames {
		// StepIdx must be present (pointer to 0), not dropped by omitempty.
		if f.StepIdx == nil || *f.StepIdx != 0 {
			t.Errorf("StepIdx want *0, got %v", f.StepIdx)
		}
		chunk, err := base64.StdEncoding.DecodeString(f.Data)
		if err != nil {
			t.Fatalf("Data is not valid base64: %v", err)
		}
		combined = append(combined, chunk...)
	}
	if !strings.Contains(string(combined), "\x1b[32m") {
		t.Error("ANSI escape should be preserved (after base64 decode) in runlog.line data")
	}
}

func TestRunlogStreamer_NilPublish(t *testing.T) {
	// Should not panic when publish is nil.
	s := &runlogStreamer{beadID: "mp-abc", stepIdx: 0, publish: nil}
	r := strings.NewReader("some data\n")
	// Should not panic.
	s.stream(r)
}
