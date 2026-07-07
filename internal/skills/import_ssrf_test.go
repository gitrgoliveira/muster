package skills

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestValidateFetchURL_BlocksUnsafe(t *testing.T) {
	// Literal IPs / schemes keep this test offline (no DNS).
	blocked := []string{
		"file:///etc/passwd",            // scheme not allowed
		"ftp://example.com/x",           // scheme not allowed
		"http://192.0.2.1/x",            // non-loopback http (TEST-NET literal)
		"http://169.254.169.254/latest", // link-local + metadata
		"https://169.254.169.254/x",     // metadata over https
		"https://10.0.0.1/x",            // private range
		"https://192.168.1.1/x",         // private range
	}
	for _, raw := range blocked {
		u := mustParse(t, raw)
		if err := validateFetchURL(u); err == nil {
			t.Errorf("validateFetchURL(%q) = nil, want blocked", raw)
		}
	}

	allowed := []string{
		"http://127.0.0.1:8080/x", // loopback http (dev carve-out)
		"https://93.184.216.34/x", // public literal IP over https
	}
	for _, raw := range allowed {
		if err := validateFetchURL(mustParse(t, raw)); err != nil {
			t.Errorf("validateFetchURL(%q) = %v, want allowed", raw, err)
		}
	}
}

func TestFetchSkill_RedirectToBlockedHostRejected(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://169.254.169.254/latest/meta-data", http.StatusFound)
	}))
	defer srv.Close()
	// The initial URL is loopback (allowed), but the redirect target is blocked;
	// CheckRedirect must reject it.
	if _, err := fetchSkill(newImportClient(), srv.URL); err == nil {
		t.Fatal("redirect to a blocked host should be rejected")
	}
}

func mustParse(t *testing.T, raw string) *url.URL {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parse %q: %v", raw, err)
	}
	return u
}
