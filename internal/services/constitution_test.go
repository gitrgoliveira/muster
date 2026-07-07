package services

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/gitrgoliveira/muster/internal/ws"
)

func TestConstitution_FreshInstall_EmptyV0(t *testing.T) {
	s := NewConstitutionService(t.TempDir(), nil)
	c := s.Get()
	if c.Markdown != "" || c.Version != 0 {
		t.Fatalf("fresh install should be empty v0, got %+v", c)
	}
	md, v := s.Snapshot()
	if md != "" || v != 0 {
		t.Fatalf("Snapshot fresh = %q v%d", md, v)
	}
}

func TestConstitution_SetBumpsVersionAndEmits(t *testing.T) {
	dir := t.TempDir()
	var got []ws.Frame
	var mu sync.Mutex
	pub := func(f ws.Frame) { mu.Lock(); got = append(got, f); mu.Unlock() }

	s := NewConstitutionService(dir, pub)
	c1, err := s.Set("# v1 rules")
	if err != nil {
		t.Fatal(err)
	}
	if c1.Version != 1 {
		t.Fatalf("first Set should be v1, got %d", c1.Version)
	}
	c2, err := s.Set("# v2 rules")
	if err != nil {
		t.Fatal(err)
	}
	if c2.Version != 2 {
		t.Fatalf("second Set should be v2, got %d", c2.Version)
	}

	// The .md file is overwritten with the latest markdown.
	b, err := os.ReadFile(filepath.Join(dir, "constitution.md"))
	if err != nil || string(b) != "# v2 rules" {
		t.Fatalf("constitution.md = %q err=%v", b, err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(got) != 2 {
		t.Fatalf("want 2 constitution.changed events, got %d", len(got))
	}
	for i, f := range got {
		if f.Type != ws.EventConstitutionChanged || f.Version == nil || *f.Version != i+1 {
			t.Errorf("event %d wrong: %+v", i, f)
		}
	}
}

func TestConstitution_PersistsAcrossRestart(t *testing.T) {
	dir := t.TempDir()
	s := NewConstitutionService(dir, nil)
	if _, err := s.Set("# persisted"); err != nil {
		t.Fatal(err)
	}
	// New service instance = a restart; must reload the same markdown+version.
	s2 := NewConstitutionService(dir, nil)
	md, v := s2.Snapshot()
	if md != "# persisted" || v != 1 {
		t.Fatalf("after restart got %q v%d, want %q v1", md, v, "# persisted")
	}
}

func TestConstitution_CorruptMetaFallsBackToDefault(t *testing.T) {
	dir := t.TempDir()
	// A present markdown but a corrupt meta sidecar must not fail startup — it
	// falls back to version 0 (corrupt-file edge case).
	if err := os.WriteFile(filepath.Join(dir, "constitution.md"), []byte("# hand-edited"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "constitution.meta.json"), []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	s := NewConstitutionService(dir, nil)
	md, v := s.Snapshot()
	if md != "# hand-edited" || v != 0 {
		t.Fatalf("corrupt meta: got %q v%d, want markdown kept at v0", md, v)
	}
}

func TestConstitution_ConcurrentSnapshotNoTorn(t *testing.T) {
	// -race: hammer Snapshot while Set runs. Each Snapshot must be internally
	// consistent (never a panic / torn read).
	s := NewConstitutionService(t.TempDir(), nil)
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 50 {
				_, _ = s.Snapshot()
			}
		}()
	}
	for range 20 {
		if _, err := s.Set("# churn"); err != nil {
			t.Error(err)
		}
	}
	wg.Wait()
}
