package render

import (
	"encoding/json"
	"net/http"
)

// WriteJSON sets Content-Type: application/json, writes the given HTTP status
// code, and marshals v to JSON in the response body. Marshalling errors are
// silently swallowed — callers are expected to pass serialisable values.
func WriteJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
