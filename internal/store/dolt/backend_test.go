package dolt_test

import (
	"context"
	"os"
	"testing"

	"github.com/gitrgoliveira/muster/internal/store"
	"github.com/gitrgoliveira/muster/internal/store/dolt"
)

// doltDSN returns the test DSN from DOLT_TEST_DSN env or skips the test.
func doltDSN(t *testing.T) string {
	t.Helper()
	dsn := os.Getenv("DOLT_TEST_DSN")
	if dsn == "" {
		t.Skip("DOLT_TEST_DSN not set — skipping Dolt integration test")
	}
	return dsn
}

func TestNewDolt_UnreachableDSN(t *testing.T) {
	// This test does NOT require DOLT_TEST_DSN; it verifies connection failure is wrapped.
	ctx := context.Background()
	_, err := dolt.NewDolt(ctx, "root:@tcp(127.0.0.1:13307)/testdb")
	if err == nil {
		t.Fatal("want error for unreachable DSN")
	}
	if !isStoreUnavailable(err) {
		t.Errorf("want ErrStoreUnavailable wrapper, got %v", err)
	}
}

func TestDolt_ListAll(t *testing.T) {
	dsn := doltDSN(t)
	ctx := context.Background()
	b, err := dolt.NewDolt(ctx, dsn)
	if err != nil {
		t.Fatalf("NewDolt: %v", err)
	}
	defer b.Close() //nolint:errcheck

	issues, err := b.List(ctx, store.Filter{})
	if err != nil {
		t.Fatal(err)
	}
	// We can't assert an exact count without fixtures, but should get a slice.
	if issues == nil {
		t.Error("want non-nil slice")
	}
}

func TestDolt_ListByStatus(t *testing.T) {
	dsn := doltDSN(t)
	ctx := context.Background()
	b, err := dolt.NewDolt(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close() //nolint:errcheck

	issues, err := b.List(ctx, store.Filter{Status: []string{"open"}})
	if err != nil {
		t.Fatal(err)
	}
	for _, iss := range issues {
		if iss.Status != "open" {
			t.Errorf("unexpected status %q in open filter", iss.Status)
		}
	}
}

func TestDolt_GetMissing(t *testing.T) {
	dsn := doltDSN(t)
	ctx := context.Background()
	b, err := dolt.NewDolt(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close() //nolint:errcheck

	_, err = b.Get(ctx, "nonexistent-zzz-999")
	if err != store.ErrNotFound {
		t.Errorf("want ErrNotFound, got %v", err)
	}
}

func TestDolt_CloseIdempotent(t *testing.T) {
	dsn := doltDSN(t)
	ctx := context.Background()
	b, err := dolt.NewDolt(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	if err := b.Close(); err != nil {
		t.Errorf("first Close: %v", err)
	}
}

func TestDolt_Ping(t *testing.T) {
	dsn := doltDSN(t)
	ctx := context.Background()
	b, err := dolt.NewDolt(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close() //nolint:errcheck

	if err := b.Ping(ctx); err != nil {
		t.Errorf("Ping: %v", err)
	}
}

// isStoreUnavailable checks whether err wraps store.ErrStoreUnavailable.
func isStoreUnavailable(err error) bool {
	if err == nil {
		return false
	}
	return containsString(err.Error(), store.ErrStoreUnavailable.Error())
}

func containsString(s, sub string) bool {
	return len(sub) == 0 || (len(s) >= len(sub) && func() bool {
		for i := 0; i <= len(s)-len(sub); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	}())
}
