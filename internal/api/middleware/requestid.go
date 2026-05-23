package middleware

import (
	"net/http"

	"github.com/gitrgoliveira/muster/internal/api/render"
	"github.com/google/uuid"
)

const requestIDHeader = "X-Request-ID"

// RequestID is an HTTP middleware that ensures every request carries a
// request ID. It reads X-Request-ID from the incoming request header; if
// absent, it generates a new UUIDv4. The ID is stored in the request context
// via render.SetRequestID so downstream handlers and render.WriteError can
// include it in responses. The X-Request-ID header is also set on the
// response before calling the next handler.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get(requestIDHeader)
		if id == "" {
			id = uuid.NewString()
		}

		ctx := render.SetRequestID(r.Context(), id)
		w.Header().Set(requestIDHeader, id)

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
