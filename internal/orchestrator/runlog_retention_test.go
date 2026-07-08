package orchestrator

import (
	"strings"
	"testing"
)

func TestTailBuffer_BoundsBytes(t *testing.T) {
	var tb tailBuffer
	// Write far more than the byte cap as a single blob (no newlines).
	tb.append([]byte(strings.Repeat("a", 5*maxTailBytes)))
	if got := len(tb.String()); got > maxTailBytes {
		t.Fatalf("tail exceeds byte cap: %d > %d", got, maxTailBytes)
	}
}

func TestTailBuffer_BoundsLines(t *testing.T) {
	var tb tailBuffer
	for range maxTailLines * 3 {
		tb.append([]byte("line\n"))
	}
	if got := strings.Count(tb.String(), "\n"); got > maxTailLines {
		t.Fatalf("tail exceeds line cap: %d > %d", got, maxTailLines)
	}
}

func TestTailBuffer_KeepsLastContent(t *testing.T) {
	var tb tailBuffer
	tb.append([]byte("early junk\n"))
	tb.append([]byte("the important last line\n"))
	if !strings.Contains(tb.String(), "the important last line") {
		t.Fatalf("tail dropped the last line: %q", tb.String())
	}
}

func TestRunlogStreamer_OnDoneReceivesTail(t *testing.T) {
	var got string
	s := &runlogStreamer{
		onDone: func(tail string) { got = tail },
		// no publish: exercise the tail/onDone path without a hub
	}
	s.stream(strings.NewReader("noise\nfinal summary line\n"))
	if !strings.Contains(got, "final summary line") {
		t.Fatalf("onDone tail missing final line: %q", got)
	}
	if want := oneLineSummary(got); want != "final summary line" {
		t.Fatalf("oneLineSummary(tail) = %q", want)
	}
}
