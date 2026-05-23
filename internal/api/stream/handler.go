package stream

import (
	"net/http"

	"github.com/gitrgoliveira/muster/internal/ws"
)

// StreamHandler returns an http.HandlerFunc that upgrades the connection to
// WebSocket and hands it off to the hub.
func StreamHandler(hub *ws.Hub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ws.ServeWS(hub, w, r)
	}
}
