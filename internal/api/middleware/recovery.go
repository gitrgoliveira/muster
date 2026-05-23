package middleware

import (
	"log/slog"
	"net/http"
	"runtime/debug"

	"github.com/gitrgoliveira/muster/internal/api/render"
)

// Recovery catches panics from downstream handlers, logs the stack trace, and
// returns a 500 INTERNAL JSON response so the server stays alive.
func Recovery(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				slog.Error("handler panic recovered",
					"recover", rec,
					"stack", string(debug.Stack()))
				render.WriteError(w, r, http.StatusInternalServerError,
					render.CodeInternal, "internal server error")
			}
		}()
		next.ServeHTTP(w, r)
	})
}
