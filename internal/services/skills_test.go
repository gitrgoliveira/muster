package services

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gitrgoliveira/muster/internal/skills"
)

func newSkillService(t *testing.T, musterDir string) *SkillService {
	t.Helper()
	reg, err := skills.NewRegistry(musterDir)
	if err != nil {
		t.Fatal(err)
	}
	return NewSkillService(reg)
}

// Delete classifies each failure precisely: an invalid id is a client 400
// (validated up front, never reaching the store), a built-in is 403, an unknown
// id is 404. An unexpected store fault would be a 500 — never a misclassified
// 400 that leaks raw error text.
func TestSkillService_DeleteErrorMapping(t *testing.T) {
	svc := newSkillService(t, t.TempDir())
	cases := []struct {
		id, wantCode string
	}{
		{"../evil", CodeSkillInvalidID},   // traversal → 400, validated up front
		{"has space", CodeSkillInvalidID}, // bad chars → 400
		{"repo-grep", CodeSkillReadonly},  // built-in → 403
		{"not-there", CodeSkillNotFound},  // valid but unknown → 404
	}
	for _, c := range cases {
		err := svc.Delete(c.id)
		var se *ServiceError
		if !errors.As(err, &se) {
			t.Fatalf("Delete(%q) = %v, want *ServiceError", c.id, err)
		}
		if se.Code != c.wantCode {
			t.Errorf("Delete(%q) code = %q, want %q", c.id, se.Code, c.wantCode)
		}
	}
}

// A fetch that succeeds but fails to persist is an INTERNAL (500), not a client
// 400 — a server fault must not be reported as the caller's mistake.
func TestSkillService_ImportPersistFailureIsInternal(t *testing.T) {
	base := t.TempDir()
	musterDir := filepath.Join(base, "x")
	svc := newSkillService(t, musterDir)
	if err := os.WriteFile(musterDir, []byte("blocker"), 0o600); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("---\nid: custom-skill\nname: Custom\n---\nstub"))
	}))
	defer srv.Close()

	_, err := svc.Import(srv.URL)
	var se *ServiceError
	if !errors.As(err, &se) {
		t.Fatalf("Import = %v, want *ServiceError", err)
	}
	if se.Code != CodeInternal {
		t.Errorf("Import persist failure code = %q, want %q", se.Code, CodeInternal)
	}
}

// A blocked/oversize/parse failure stays a client INVALID_REQUEST (400).
func TestSkillService_ImportBlockedURLIsInvalidRequest(t *testing.T) {
	svc := newSkillService(t, t.TempDir())
	_, err := svc.Import("file:///etc/passwd") // scheme blocked pre-dial
	var se *ServiceError
	if !errors.As(err, &se) {
		t.Fatalf("Import = %v, want *ServiceError", err)
	}
	if se.Code != CodeInvalidRequest {
		t.Errorf("Import blocked-URL code = %q, want %q", se.Code, CodeInvalidRequest)
	}
}
