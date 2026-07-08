package memories

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gitrgoliveira/muster/internal/services"
	"github.com/gitrgoliveira/muster/internal/store/bdshell"
	"github.com/go-chi/chi/v5"
)

// fakeStore implements services.MemoryStore.
type fakeStore struct{ m map[string]string }

func (f *fakeStore) Remember(_ context.Context, key, value string) (string, error) {
	if key == "" {
		// bd derives a kebab-slug key (no spaces); mimic that.
		key = "k-" + strings.ReplaceAll(value, " ", "-")
	}
	f.m[key] = value
	return key, nil
}
func (f *fakeStore) Recall(_ context.Context, key string) (string, error) { return f.m[key], nil }
func (f *fakeStore) Forget(_ context.Context, key string) error {
	if _, ok := f.m[key]; !ok {
		return bdshell.ErrMemoryNotFound
	}
	delete(f.m, key)
	return nil
}
func (f *fakeStore) Memories(_ context.Context, _ string) (map[string]string, error) {
	return f.m, nil
}

func testRouter(t *testing.T) http.Handler {
	t.Helper()
	svc := services.NewMemoriesService(&fakeStore{m: map[string]string{}}, t.TempDir())
	h := NewHandlers(svc)
	r := chi.NewRouter()
	r.Get("/api/v1/memories", h.List)
	r.Post("/api/v1/memories", h.Upsert)
	r.Delete("/api/v1/memories/{key}", h.Delete)
	r.Post("/api/v1/memories/prime", h.Prime)
	return r
}

func req(t *testing.T, r http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(method, path, strings.NewReader(body)))
	return rec
}

func TestMemories_RoundTrip(t *testing.T) {
	r := testRouter(t)

	rec := req(t, r, http.MethodPost, "/api/v1/memories", `{"value":"run -race"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST = %d body %s", rec.Code, rec.Body)
	}
	var m services.Memory
	_ = json.Unmarshal(rec.Body.Bytes(), &m)
	if m.Key == "" {
		t.Fatal("POST should return an auto-derived key")
	}

	rec = req(t, r, http.MethodGet, "/api/v1/memories?q=race", "")
	var list listResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &list)
	if len(list.Memories) != 1 {
		t.Fatalf("GET list = %v", list.Memories)
	}

	rec = req(t, r, http.MethodDelete, "/api/v1/memories/"+m.Key, "")
	if rec.Code != http.StatusNoContent {
		t.Fatalf("DELETE = %d", rec.Code)
	}
	rec = req(t, r, http.MethodDelete, "/api/v1/memories/"+m.Key, "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("DELETE missing = %d, want 404", rec.Code)
	}
}

func TestMemories_Prime(t *testing.T) {
	r := testRouter(t)
	_ = req(t, r, http.MethodPost, "/api/v1/memories", `{"value":"v"}`)
	rec := req(t, r, http.MethodPost, "/api/v1/memories/prime", `{"beadID":"muster-ep0"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("prime = %d body %s", rec.Code, rec.Body)
	}
	var resp primeResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp.Primed != 1 {
		t.Fatalf("primed = %d, want 1", resp.Primed)
	}
}
