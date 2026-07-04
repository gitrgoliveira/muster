package config_test

import (
	"strings"
	"testing"

	"github.com/gitrgoliveira/muster/internal/config"
)

// ── T001: ParseMaxConcurrent (failing test) ──────────────────────────────────

func TestParseMaxConcurrent(t *testing.T) {
	t.Run("empty string defaults to 4", func(t *testing.T) {
		n, err := config.ParseMaxConcurrent("")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if n != 4 {
			t.Errorf("empty string: want 4 got %d", n)
		}
	})

	t.Run("valid positive integer is accepted", func(t *testing.T) {
		n, err := config.ParseMaxConcurrent("8")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if n != 8 {
			t.Errorf("want 8 got %d", n)
		}
	})

	t.Run("1 is valid (minimum positive)", func(t *testing.T) {
		n, err := config.ParseMaxConcurrent("1")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if n != 1 {
			t.Errorf("want 1 got %d", n)
		}
	})

	t.Run("zero returns typed error", func(t *testing.T) {
		_, err := config.ParseMaxConcurrent("0")
		if err == nil {
			t.Fatal("want error for 0, got nil")
		}
		if !strings.Contains(err.Error(), "must be") {
			t.Errorf("error %q should mention 'must be'", err.Error())
		}
	})

	t.Run("negative integer returns typed error", func(t *testing.T) {
		_, err := config.ParseMaxConcurrent("-1")
		if err == nil {
			t.Fatal("want error for -1, got nil")
		}
		if !strings.Contains(err.Error(), "must be") {
			t.Errorf("error %q should mention 'must be'", err.Error())
		}
	})

	t.Run("non-integer returns typed error", func(t *testing.T) {
		_, err := config.ParseMaxConcurrent("abc")
		if err == nil {
			t.Fatal("want error for non-integer, got nil")
		}
	})

	t.Run("float returns typed error", func(t *testing.T) {
		_, err := config.ParseMaxConcurrent("3.14")
		if err == nil {
			t.Fatal("want error for float, got nil")
		}
	})
}
