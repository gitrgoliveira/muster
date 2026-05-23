package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	"github.com/gitrgoliveira/muster/internal/api/middleware"
	"github.com/gitrgoliveira/muster/internal/api/render"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const xRequestID = "X-Request-ID"

// uuidRE matches a canonical UUIDv4 string (36 hex chars + dashes).
var uuidRE = regexp.MustCompile(`^[0-9a-f-]{36}$`)

// makeHandler wraps a simple next handler with the RequestID middleware and
// returns a ready-to-use *httptest.ResponseRecorder after serving req.
func serve(t *testing.T, req *http.Request, next http.HandlerFunc) *httptest.ResponseRecorder {
	t.Helper()
	rr := httptest.NewRecorder()
	middleware.RequestID(next).ServeHTTP(rr, req)
	return rr
}

// TestRequestID_EchoesSupplied verifies that when the client sends an
// X-Request-ID header the middleware echoes the same value in the response.
func TestRequestID_EchoesSupplied(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set(xRequestID, "my-id")

	rr := serve(t, req, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	assert.Equal(t, "my-id", rr.Header().Get(xRequestID))
}

// TestRequestID_GeneratesWhenAbsent verifies that when the client does NOT
// send X-Request-ID the middleware generates a UUID and sets it in the
// response header.
func TestRequestID_GeneratesWhenAbsent(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)

	rr := serve(t, req, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	got := rr.Header().Get(xRequestID)
	require.NotEmpty(t, got, "X-Request-ID response header must be set")
	assert.Regexp(t, uuidRE, got, "generated ID should match UUID format")
}

// TestRequestID_AvailableInContext verifies that the request ID is stored in
// the request context so that downstream handlers can retrieve it via
// render.GetRequestID.
func TestRequestID_AvailableInContext(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set(xRequestID, "ctx-test-id")

	var ctxID string
	serve(t, req, func(w http.ResponseWriter, r *http.Request) {
		ctxID = render.GetRequestID(r.Context())
		w.WriteHeader(http.StatusOK)
	})

	assert.Equal(t, "ctx-test-id", ctxID,
		"render.GetRequestID should return the same ID that was set by the middleware")
}

// TestRequestID_OnErrorResponses verifies that the X-Request-ID header is
// present in the response even when the downstream handler writes a 404.
func TestRequestID_OnErrorResponses(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/missing", nil)
	req.Header.Set(xRequestID, "err-id")

	rr := serve(t, req, func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	})

	assert.Equal(t, http.StatusNotFound, rr.Code)
	assert.Equal(t, "err-id", rr.Header().Get(xRequestID),
		"X-Request-ID must be present even on error responses")
}
