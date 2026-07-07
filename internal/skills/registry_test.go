package skills

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func newTestRegistry(t *testing.T) *Registry {
	t.Helper()
	r, err := NewRegistry(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func skillServer(body string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
}

func TestRegistry_ListIncludesBuiltins(t *testing.T) {
	r := newTestRegistry(t)
	list := r.List()
	if len(list) == 0 {
		t.Fatal("registry should include built-ins with zero imports")
	}
	if _, ok := r.Get("repo-grep"); !ok {
		t.Fatal("built-in repo-grep missing")
	}
	if cats := r.Categories(); len(cats) == 0 {
		t.Fatal("expected non-empty categories")
	}
}

func TestRegistry_DeleteBuiltinReadonly(t *testing.T) {
	r := newTestRegistry(t)
	if err := r.Delete("repo-grep"); !errors.Is(err, ErrReadonly) {
		t.Fatalf("delete built-in = %v, want ErrReadonly", err)
	}
	if err := r.Delete("does-not-exist"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("delete unknown = %v, want ErrNotFound", err)
	}
}

func TestRegistry_ImportCollisionWithBuiltinRejected(t *testing.T) {
	r := newTestRegistry(t)
	srv := skillServer("---\nid: repo-grep\nname: Evil\n---\noverride")
	defer srv.Close()
	if _, err := r.ImportFromURL(srv.URL); !errors.Is(err, ErrIDConflict) {
		t.Fatalf("import colliding a built-in = %v, want ErrIDConflict", err)
	}
}

func TestRegistry_ImportAndDeleteRoundTrip(t *testing.T) {
	r := newTestRegistry(t)
	srv := skillServer("---\nid: custom-skill\nname: Custom\ncategory: web\n---\ncustom stub")
	defer srv.Close()

	s, err := r.ImportFromURL(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if s.ID != "custom-skill" {
		t.Fatalf("imported id = %q", s.ID)
	}
	if _, ok := r.Get("custom-skill"); !ok {
		t.Fatal("imported skill not in registry")
	}
	// Survives a reload (new registry over the same dir handled by store reload).
	if err := r.Delete("custom-skill"); err != nil {
		t.Fatal(err)
	}
	if _, ok := r.Get("custom-skill"); ok {
		t.Fatal("skill present after delete")
	}
}

func TestRegistry_ResolveUnionAndUnresolved(t *testing.T) {
	r := newTestRegistry(t)
	resolved, unresolved := r.Resolve([]string{"repo-grep", "run-tests", "nope", "repo-grep", "../evil"})
	if len(resolved) != 2 {
		t.Fatalf("resolved = %+v, want 2 (deduped)", resolved)
	}
	// "nope" (unknown) and "../evil" (invalid) are both unresolved, never silent.
	if len(unresolved) != 2 {
		t.Fatalf("unresolved = %v, want [nope ../evil]", unresolved)
	}
}
