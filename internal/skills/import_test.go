package skills

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFetchSkill_HappyPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(validSkill))
	}))
	defer srv.Close()

	s, err := fetchSkill(srv.Client(), srv.URL) // loopback is allowed
	if err != nil {
		t.Fatal(err)
	}
	if s.ID != "repo-grep" || s.BuiltIn {
		t.Fatalf("imported skill wrong: %+v", s)
	}
}

func TestFetchSkill_Oversize(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("---\nid: big\nname: Big\n---\n"))
		_, _ = w.Write([]byte(strings.Repeat("x", importMaxBytes+100)))
	}))
	defer srv.Close()
	if _, err := fetchSkill(srv.Client(), srv.URL); err == nil {
		t.Fatal("oversize body should error")
	}
}

func TestFetchSkill_MalformedAndBadStatus(t *testing.T) {
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("not a skill document"))
	}))
	defer bad.Close()
	if _, err := fetchSkill(bad.Client(), bad.URL); err == nil {
		t.Fatal("malformed body should error (no partial registration)")
	}

	status := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer status.Close()
	if _, err := fetchSkill(status.Client(), status.URL); err == nil {
		t.Fatal("non-200 status should error")
	}
}
