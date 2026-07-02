package tmux

import (
	"testing"
)

func TestSessionName(t *testing.T) {
	tests := []struct {
		beadID  string
		stepIdx int
		loop    int
		want    string
	}{
		{"mp-abc", 0, 0, "muster/mp-abc/0/0"},
		{"mp-xyz", 1, 2, "muster/mp-xyz/1/2"},
		{"bd-0001", 0, 3, "muster/bd-0001/0/3"},
	}
	for _, tt := range tests {
		got := SessionName(tt.beadID, tt.stepIdx, tt.loop)
		if got != tt.want {
			t.Errorf("SessionName(%q,%d,%d) = %q, want %q", tt.beadID, tt.stepIdx, tt.loop, got, tt.want)
		}
	}
}

func TestParseSessionName(t *testing.T) {
	t.Run("valid roundtrip", func(t *testing.T) {
		name := SessionName("mp-abc", 0, 0)
		beadID, stepIdx, loop, err := ParseSessionName(name)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if beadID != "mp-abc" || stepIdx != 0 || loop != 0 {
			t.Errorf("got (%q,%d,%d), want (mp-abc,0,0)", beadID, stepIdx, loop)
		}
	})

	t.Run("non-zero indices", func(t *testing.T) {
		name := SessionName("bd-0001", 1, 3)
		beadID, stepIdx, loop, err := ParseSessionName(name)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if beadID != "bd-0001" || stepIdx != 1 || loop != 3 {
			t.Errorf("got (%q,%d,%d), want (bd-0001,1,3)", beadID, stepIdx, loop)
		}
	})

	t.Run("wrong prefix returns error", func(t *testing.T) {
		_, _, _, err := ParseSessionName("notmuster/bead/0/0")
		if err == nil {
			t.Error("want error for wrong prefix")
		}
	})

	t.Run("too few parts returns error", func(t *testing.T) {
		_, _, _, err := ParseSessionName("muster/bead/0")
		if err == nil {
			t.Error("want error for too few parts")
		}
	})

	t.Run("non-integer step returns error", func(t *testing.T) {
		_, _, _, err := ParseSessionName("muster/bead/x/0")
		if err == nil {
			t.Error("want error for non-integer step")
		}
	})

	t.Run("non-integer loop returns error", func(t *testing.T) {
		_, _, _, err := ParseSessionName("muster/bead/0/x")
		if err == nil {
			t.Error("want error for non-integer loop")
		}
	})
}

func TestIsMusterSession(t *testing.T) {
	if !IsMusterSession("muster/mp-abc/0/0") {
		t.Error("IsMusterSession should return true for muster/ prefix")
	}
	if IsMusterSession("other/session") {
		t.Error("IsMusterSession should return false for non-muster prefix")
	}
}
