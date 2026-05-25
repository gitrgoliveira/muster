package render

import (
	"context"
	"encoding/json"
	"net/http"
)

// Error codes.
const (
	CodeBeadNotFound     = "BEAD_NOT_FOUND"
	CodeNotFound         = "NOT_FOUND"
	CodeInvalidRequest   = "INVALID_REQUEST"
	CodeInvalidState     = "INVALID_STATE"
	CodeMethodNotAllowed = "METHOD_NOT_ALLOWED"
	CodeInternal         = "INTERNAL"
	CodeCLIMissing       = "BD_CLI_MISSING"
	CodeCLIValidation    = "BD_CLI_VALIDATION"
	CodeStoreUnavailable = "STORE_UNAVAILABLE"
	CodeGatewayTimeout   = "GATEWAY_TIMEOUT"
)

// ErrorResponse is the top-level JSON envelope returned for all error responses.
type ErrorResponse struct {
	Error ErrorDetail `json:"error"`
}

// ErrorDetail carries the structured error payload.
type ErrorDetail struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"requestID"`
}

// contextKey is a local unexported type used to avoid context key collisions.
type contextKey string

const requestIDKey contextKey = "requestID"

// SetRequestID stores a request ID in the context. Middleware should call this
// to inject the request ID so it can be retrieved by WriteError.
func SetRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, requestIDKey, id)
}

// GetRequestID retrieves the request ID from the context. Returns an empty
// string if no ID was set.
func GetRequestID(ctx context.Context) string {
	if id, ok := ctx.Value(requestIDKey).(string); ok {
		return id
	}
	return ""
}

// httpStatusForCode maps an application error code to the appropriate HTTP
// status code.
func httpStatusForCode(code string) int {
	switch code {
	case CodeBeadNotFound, CodeNotFound:
		return http.StatusNotFound
	case CodeInvalidRequest, CodeInvalidState:
		return http.StatusBadRequest
	case CodeMethodNotAllowed:
		return http.StatusMethodNotAllowed
	default:
		return http.StatusInternalServerError
	}
}

// WriteError writes a structured JSON error response. If httpStatus is zero,
// the status is derived from code via httpStatusForCode.
func WriteError(w http.ResponseWriter, r *http.Request, httpStatus int, code, message string) {
	if httpStatus == 0 {
		httpStatus = httpStatusForCode(code)
	}
	resp := ErrorResponse{
		Error: ErrorDetail{
			Code:      code,
			Message:   message,
			RequestID: GetRequestID(r.Context()),
		},
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(httpStatus)
	_ = json.NewEncoder(w).Encode(resp)
}
