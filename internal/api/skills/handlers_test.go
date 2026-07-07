package skills

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gitrgoliveira/muster/internal/services"
	coreskills "github.com/gitrgoliveira/muster/internal/skills"
	"github.com/go-chi/chi/v5"
)

func testRouter(t *testing.T) http.Handler {
	t.Helper()
	reg, err := coreskills.NewRegistry(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	h := NewHandlers(services.NewSkillService(reg))
	r := chi.NewRouter()
	r.Get("/api/v1/skills", h.List)
	r.Get("/api/v1/skills/categories", h.Categories)
	r.Post("/api/v1/skills", h.Import)
	r.Delete("/api/v1/skills/{id}", h.Delete)
	return r
}

func do(t *testing.T, r http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	var rdr *strings.Reader
	if body != "" {
		rdr = strings.NewReader(body)
	} else {
		rdr = strings.NewReader("")
	}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(method, path, rdr))
	return rec
}

func TestSkills_ListHasBuiltinsZeroImports(t *testing.T) {
	r := testRouter(t)
	rec := do(t, r, http.MethodGet, "/api/v1/skills", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /skills = %d", rec.Code)
	}
	var resp listResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if len(resp.Skills) == 0 {
		t.Fatal("expected built-in skills with zero imports")
	}
}

func TestSkills_Categories(t *testing.T) {
	rec := do(t, testRouter(t), http.MethodGet, "/api/v1/skills/categories", "")
	var resp categoriesResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if len(resp.Categories) == 0 {
		t.Fatal("expected categories")
	}
}

func TestSkills_DeleteBuiltin403_Unknown404(t *testing.T) {
	r := testRouter(t)
	if rec := do(t, r, http.MethodDelete, "/api/v1/skills/repo-grep", ""); rec.Code != http.StatusForbidden {
		t.Fatalf("DELETE built-in = %d, want 403", rec.Code)
	}
	if rec := do(t, r, http.MethodDelete, "/api/v1/skills/does-not-exist", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("DELETE unknown = %d, want 404", rec.Code)
	}
}

func TestSkills_ImportBadBody400(t *testing.T) {
	r := testRouter(t)
	if rec := do(t, r, http.MethodPost, "/api/v1/skills", `{"url":""}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("empty url = %d, want 400", rec.Code)
	}
	if rec := do(t, r, http.MethodPost, "/api/v1/skills", `{not json`); rec.Code != http.StatusBadRequest {
		t.Fatalf("malformed = %d, want 400", rec.Code)
	}
	// A blocked scheme is a 400 (no partial registration).
	if rec := do(t, r, http.MethodPost, "/api/v1/skills", `{"url":"file:///etc/passwd"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("file:// = %d, want 400", rec.Code)
	}
}

func TestSkills_ImportRoundTrip(t *testing.T) {
	r := testRouter(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("---\nid: my-skill\nname: Mine\ncategory: web\n---\ndo the thing"))
	}))
	defer srv.Close()

	rec := do(t, r, http.MethodPost, "/api/v1/skills", `{"url":"`+srv.URL+`"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("import = %d, body %s", rec.Code, rec.Body)
	}
	var sk coreskills.Skill
	_ = json.Unmarshal(rec.Body.Bytes(), &sk)
	if sk.ID != "my-skill" {
		t.Fatalf("imported id = %q", sk.ID)
	}
	// Now deletable (imported, not built-in).
	if rec := do(t, r, http.MethodDelete, "/api/v1/skills/my-skill", ""); rec.Code != http.StatusNoContent {
		t.Fatalf("DELETE imported = %d, want 204", rec.Code)
	}
}
