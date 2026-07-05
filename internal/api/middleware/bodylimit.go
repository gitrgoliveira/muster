package middleware

import (
	"bytes"
	"io"
	"net/http"

	"github.com/gitrgoliveira/muster/internal/api/render"
)

// BodyLimit is a Chi-compatible middleware that enforces a 1 MiB body limit on
// POST, PATCH, and PUT requests. All other methods are passed through unchanged.
//
// The body is read eagerly in the middleware. If it exceeds 1 MiB the request
// is rejected immediately with 400 INVALID_REQUEST before the handler runs.
// Otherwise the body is replaced with a buffered reader so the downstream
// handler can still read it normally.
func BodyLimit(next http.Handler) http.Handler {
	const maxBytes = 1 << 20 // 1 MiB

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost && r.Method != http.MethodPatch && r.Method != http.MethodPut {
			next.ServeHTTP(w, r)
			return
		}

		// Read up to maxBytes+1 so we can detect when the limit is exceeded.
		lr := &io.LimitedReader{R: r.Body, N: maxBytes + 1}
		body, err := io.ReadAll(lr)
		_ = r.Body.Close()

		if err != nil {
			render.WriteError(w, r, http.StatusBadRequest, render.CodeInvalidRequest, "request body exceeds 1 MiB limit")
			return
		}

		// If N reached zero the underlying reader was not exhausted — the body
		// is larger than maxBytes.
		if lr.N == 0 {
			render.WriteError(w, r, http.StatusBadRequest, render.CodeInvalidRequest, "request body exceeds 1 MiB limit")
			return
		}

		r.Body = io.NopCloser(bytes.NewReader(body))
		next.ServeHTTP(w, r)
	})
}
