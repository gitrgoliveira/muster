package constitution

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gitrgoliveira/muster/internal/services"
)

func newTestHandlers(t *testing.T) *Handlers {
	t.Helper()
	return NewHandlers(services.NewConstitutionService(t.TempDir(), nil))
}

func TestGet_FreshInstall_V0NotFound(t *testing.T) {
	h := newTestHandlers(t)
	rec := httptest.NewRecorder()
	h.Get(rec, httptest.NewRequest(http.MethodGet, "/api/v1/constitution", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("fresh GET should be 200 (not 404), got %d", rec.Code)
	}
	var resp getResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Markdown != "" || resp.Version != 0 || resp.UpdatedAt != nil {
		t.Fatalf("fresh install = %+v, want empty/v0/null", resp)
	}
}

func TestPut_BumpsVersionAndGetReflects(t *testing.T) {
	h := newTestHandlers(t)

	rec := httptest.NewRecorder()
	h.Put(rec, httptest.NewRequest(http.MethodPut, "/api/v1/constitution", strings.NewReader(`{"markdown":"# rules"}`)))
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT = %d, body %s", rec.Code, rec.Body)
	}
	var put getResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &put)
	if put.Version != 1 || put.Markdown != "# rules" || put.UpdatedAt == nil {
		t.Fatalf("PUT response = %+v", put)
	}

	rec = httptest.NewRecorder()
	h.Get(rec, httptest.NewRequest(http.MethodGet, "/api/v1/constitution", nil))
	var got getResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if got.Version != 1 || got.Markdown != "# rules" {
		t.Fatalf("GET after PUT = %+v", got)
	}
}

func TestPut_MalformedAndUnknownField_400(t *testing.T) {
	h := newTestHandlers(t)
	for _, body := range []string{`{not json`, `{"markdown":"x","bogus":1}`} {
		rec := httptest.NewRecorder()
		h.Put(rec, httptest.NewRequest(http.MethodPut, "/api/v1/constitution", strings.NewReader(body)))
		if rec.Code != http.StatusBadRequest {
			t.Errorf("PUT %q = %d, want 400", body, rec.Code)
		}
	}
}
