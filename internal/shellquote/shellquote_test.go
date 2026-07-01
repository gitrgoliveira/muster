package shellquote_test

import (
	"testing"

	"github.com/gitrgoliveira/muster/internal/shellquote"
)

func TestSingle(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"plain", "'plain'"},
		{"has space", "'has space'"},
		{"it's", `'it'\''s'`},
		{"", "''"},
	}
	for _, tt := range tests {
		got := shellquote.Single(tt.in)
		if got != tt.want {
			t.Errorf("Single(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
