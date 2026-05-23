package render_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gitrgoliveira/muster/internal/api/render"
)

// TestWriteJSON_SetsHeadersAndStatus verifies that WriteJSON sets the correct
// Content-Type header, status code, and produces a valid JSON body.
func TestWriteJSON_SetsHeadersAndStatus(t *testing.T) {
	type payload struct {
		Greeting string `json:"greeting"`
	}

	w := httptest.NewRecorder()
	render.WriteJSON(w, http.StatusCreated, payload{Greeting: "hello"})

	res := w.Result()

	if got := res.Header.Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type = %q, want %q", got, "application/json")
	}

	if res.StatusCode != http.StatusCreated {
		t.Errorf("status = %d, want %d", res.StatusCode, http.StatusCreated)
	}

	var out payload
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		t.Fatalf("body is not valid JSON: %v", err)
	}
	if out.Greeting != "hello" {
		t.Errorf("greeting = %q, want %q", out.Greeting, "hello")
	}
}

// TestWriteError_AllCodes is a table-driven test that verifies each error code
// maps to the expected HTTP status and that the JSON body has the right shape.
func TestWriteError_AllCodes(t *testing.T) {
	cases := []struct {
		code           string
		expectedStatus int
	}{
		{render.CodeBeadNotFound, http.StatusNotFound},
		{render.CodeNotFound, http.StatusNotFound},
		{render.CodeInvalidRequest, http.StatusBadRequest},
		{render.CodeInvalidState, http.StatusBadRequest},
		{render.CodeMethodNotAllowed, http.StatusMethodNotAllowed},
		{render.CodeInternal, http.StatusInternalServerError},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.code, func(t *testing.T) {
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/", nil)

			render.WriteError(w, req, tc.expectedStatus, tc.code, "test message")

			res := w.Result()

			if res.StatusCode != tc.expectedStatus {
				t.Errorf("code %s: status = %d, want %d", tc.code, res.StatusCode, tc.expectedStatus)
			}

			if got := res.Header.Get("Content-Type"); got != "application/json" {
				t.Errorf("code %s: Content-Type = %q, want %q", tc.code, got, "application/json")
			}

			var errResp render.ErrorResponse
			if err := json.NewDecoder(res.Body).Decode(&errResp); err != nil {
				t.Fatalf("code %s: body is not valid JSON: %v", tc.code, err)
			}

			if errResp.Error.Code != tc.code {
				t.Errorf("code %s: body code = %q, want %q", tc.code, errResp.Error.Code, tc.code)
			}

			if errResp.Error.Message != "test message" {
				t.Errorf("code %s: body message = %q, want %q", tc.code, errResp.Error.Message, "test message")
			}
		})
	}
}

// TestWriteError_IncludesRequestID verifies that a request ID stored in the
// context via render.SetRequestID appears in the JSON error response body.
func TestWriteError_IncludesRequestID(t *testing.T) {
	const wantID = "req-abc-123"

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)

	// Inject requestID into the context using the exported helper.
	ctx := render.SetRequestID(req.Context(), wantID)
	req = req.WithContext(ctx)

	render.WriteError(w, req, http.StatusInternalServerError, render.CodeInternal, "something broke")

	var errResp render.ErrorResponse
	if err := json.NewDecoder(w.Result().Body).Decode(&errResp); err != nil {
		t.Fatalf("body is not valid JSON: %v", err)
	}

	if errResp.Error.RequestID != wantID {
		t.Errorf("requestID = %q, want %q", errResp.Error.RequestID, wantID)
	}
}

// TestGetRequestID_MissingReturnsEmpty ensures GetRequestID returns an empty
// string when no ID was stored.
func TestGetRequestID_MissingReturnsEmpty(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	if got := render.GetRequestID(req.Context()); got != "" {
		t.Errorf("GetRequestID = %q, want empty string", got)
	}
}

// TestWriteError_StatusDerivedFromCode verifies that passing httpStatus=0
// causes WriteError to derive the status from the code.
func TestWriteError_StatusDerivedFromCode(t *testing.T) {
	cases := []struct {
		code           string
		expectedStatus int
	}{
		{render.CodeBeadNotFound, http.StatusNotFound},
		{render.CodeNotFound, http.StatusNotFound},
		{render.CodeInvalidRequest, http.StatusBadRequest},
		{render.CodeInvalidState, http.StatusBadRequest},
		{render.CodeMethodNotAllowed, http.StatusMethodNotAllowed},
		{render.CodeInternal, http.StatusInternalServerError},
	}

	for _, tc := range cases {
		tc := tc
		t.Run("derived_"+tc.code, func(t *testing.T) {
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/", nil)

			render.WriteError(w, req, 0, tc.code, "auto-derived status")

			if w.Code != tc.expectedStatus {
				t.Errorf("code %s: derived status = %d, want %d", tc.code, w.Code, tc.expectedStatus)
			}
		})
	}
}
