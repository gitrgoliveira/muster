package middleware_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gitrgoliveira/muster/internal/api/middleware"
	"github.com/gitrgoliveira/muster/internal/api/render"
)

// echoLenHandler responds 200 and writes the number of bytes it received.
var echoLenHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "read error", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]int{"bytes": len(body)})
})

func newRequest(method string, bodySize int) *http.Request {
	var body io.Reader
	if bodySize > 0 {
		body = bytes.NewReader(bytes.Repeat([]byte("x"), bodySize))
	}
	req := httptest.NewRequest(method, "/", body)
	return req
}

func TestBodyLimit_RejectsOversize_400_InvalidRequest(t *testing.T) {
	handler := middleware.BodyLimit(echoLenHandler)

	req := newRequest(http.MethodPost, (1<<20)+1)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}

	var resp render.ErrorResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode error response: %v", err)
	}
	if resp.Error.Code != render.CodeInvalidRequest {
		t.Errorf("expected code %q, got %q", render.CodeInvalidRequest, resp.Error.Code)
	}
	if !strings.Contains(resp.Error.Message, "1 MiB") {
		t.Errorf("expected message to contain \"1 MiB\", got %q", resp.Error.Message)
	}
}

func TestBodyLimit_AcceptsBelowLimit(t *testing.T) {
	handler := middleware.BodyLimit(echoLenHandler)

	bodySize := (1 << 20) - 1
	req := newRequest(http.MethodPost, bodySize)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp map[string]int
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if got := resp["bytes"]; got != bodySize {
		t.Errorf("expected handler to receive %d bytes, got %d", bodySize, got)
	}
}

func TestBodyLimit_MessageContainsLimit(t *testing.T) {
	handler := middleware.BodyLimit(echoLenHandler)

	req := newRequest(http.MethodPost, (1<<20)+1)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}

	var resp render.ErrorResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode error response: %v", err)
	}
	if !strings.Contains(resp.Error.Message, "1 MiB") {
		t.Errorf("expected message to contain \"1 MiB\", got %q", resp.Error.Message)
	}
}

func TestBodyLimit_PassesThroughGet(t *testing.T) {
	called := false
	sentinel := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})
	handler := middleware.BodyLimit(sentinel)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if !called {
		t.Error("expected downstream handler to be called for GET request")
	}
}
