package skills

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func sampleSkill(id string) Skill {
	return Skill{ID: id, Name: "S " + id, Desc: "d", Category: "code", PromptStub: "stub for " + id}
}

func TestFileStore_CRUDRoundTrip(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "skills")
	s, err := newFileStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.list()) != 0 {
		t.Fatal("fresh store should be empty")
	}
	if err := s.put(sampleSkill("repo-grep")); err != nil {
		t.Fatal(err)
	}
	got, ok := s.get("repo-grep")
	if !ok || got.PromptStub != "stub for repo-grep" {
		t.Fatalf("get after put: %+v ok=%v", got, ok)
	}

	// Reload (simulated restart) preserves the imported skill.
	s2, err := newFileStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := s2.get("repo-grep"); !ok {
		t.Fatal("skill did not survive reload")
	}

	if err := s2.delete("repo-grep"); err != nil {
		t.Fatal(err)
	}
	if _, ok := s2.get("repo-grep"); ok {
		t.Fatal("skill present after delete")
	}
	if err := s2.delete("repo-grep"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("delete missing = %v, want ErrNotFound", err)
	}
}

func TestFileStore_RejectsTraversalBeforeFilesystem(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "skills")
	s, _ := newFileStore(dir)
	if err := s.put(sampleSkill("../escape")); err == nil {
		t.Fatal("traversal id must be rejected")
	}
	// Nothing should have been written outside the skills dir.
	if _, err := os.Stat(filepath.Join(filepath.Dir(dir), "escape.md")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("a file escaped the skills directory")
	}
}

func TestFileStore_ConcurrentCRUD_Serialized(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "skills")
	s, _ := newFileStore(dir)
	var wg sync.WaitGroup
	for i := range 12 {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			id := "skill-a"
			if n%2 == 0 {
				_ = s.put(sampleSkill(id))
			} else {
				_ = s.delete(id)
			}
		}(i)
	}
	wg.Wait()
	// Whatever the final state, the store must be internally consistent and the
	// file (if present) parseable — no half-written/orphaned file.
	if err := s.reload(); err != nil {
		t.Fatalf("store inconsistent after concurrent CRUD: %v", err)
	}
}
