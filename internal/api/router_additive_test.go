package api_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"

	api "github.com/gitrgoliveira/muster/internal/api"
	"github.com/gitrgoliveira/muster/internal/api/health"
	"github.com/gitrgoliveira/muster/internal/services"
	"github.com/gitrgoliveira/muster/internal/skills"
	"github.com/gitrgoliveira/muster/internal/store"
	"github.com/gitrgoliveira/muster/internal/ws"
)

func newM6Router(t *testing.T, m6 api.M6Services) http.Handler {
	t.Helper()
	uiFS := fstest.MapFS{"ui/index.html": &fstest.MapFile{Data: []byte("<body>t</body>")}}
	backend := store.NewMemoryBackend(store.SeedIssues())
	svc := services.NewBeadService(backend, nil, services.Publisher(func(ws.Frame) {}))
	hub := ws.NewHub("0.9.1")
	go hub.Run()
	return api.NewRouter(svc, hub, uiFS, health.StatusConfig{BeadsVersion: "0.9.1", SchemaVersion: 1}, m6)
}

func status(t *testing.T, h http.Handler, method, path string) int {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(method, path, nil))
	return rec.Code
}

// TestM6RoutesAreAdditive confirms M6 routes are registered only when wired, and
// that M0–M5 routes are unaffected (SC-008 / Principle V).
func TestM6RoutesAreAdditive(t *testing.T) {
	reg, err := skills.NewRegistry(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	wired := newM6Router(t, api.M6Services{
		Constitution: services.NewConstitutionService(t.TempDir(), nil),
		Skills:       services.NewSkillService(reg),
		Memories:     services.NewMemoriesService(nil, t.TempDir()),
	})

	// M0–M5 route still works unchanged.
	if code := status(t, wired, http.MethodGet, "/api/v1/beads"); code != http.StatusOK {
		t.Errorf("GET /beads = %d, want 200 (M0–M5 unchanged)", code)
	}
	// M6 routes are present when wired.
	for _, p := range []string{"/api/v1/constitution", "/api/v1/skills", "/api/v1/skills/categories"} {
		if code := status(t, wired, http.MethodGet, p); code != http.StatusOK {
			t.Errorf("GET %s = %d, want 200 (M6 route registered)", p, code)
		}
	}

	// With no M6 services wired, the new routes are simply absent (404) — they are
	// purely additive, never displacing an existing route.
	bare := newM6Router(t, api.M6Services{})
	for _, p := range []string{"/api/v1/constitution", "/api/v1/skills"} {
		if code := status(t, bare, http.MethodGet, p); code != http.StatusNotFound {
			t.Errorf("GET %s (unwired) = %d, want 404", p, code)
		}
	}
	// And M0–M5 is unaffected by the absence of M6 services.
	if code := status(t, bare, http.MethodGet, "/api/v1/beads"); code != http.StatusOK {
		t.Errorf("GET /beads (bare) = %d, want 200", code)
	}
}
