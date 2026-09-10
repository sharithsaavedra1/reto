package response

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"
)

// ErrorDetail represents detailed contextual validation/error messages.
type ErrorDetail struct {
	Field   string `json:"field,omitempty"`
	Message string `json:"message"`
}

// ErrorBody represents the standard error body in _shared.yaml#/components/schemas/Error.
type ErrorBody struct {
	Code    string      `json:"code"`
	Message string      `json:"message"`
	Details interface{} `json:"details,omitempty"`
	TraceID string      `json:"trace_id"`
}

// ErrorEnvelope wraps the error body under the "error" key.
type ErrorEnvelope struct {
	Error ErrorBody `json:"error"`
}

// WriteJSON sends a successful JSON response with the given status code.
func WriteJSON(w http.ResponseWriter, statusCode int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(data)
}

// WriteError sends a standardized error response adhering to the contract envelope.
func WriteError(w http.ResponseWriter, r *http.Request, statusCode int, errorCode, message string, details interface{}) {
	traceID := GetOrCreateTraceID(r)

	if details == nil {
		details = []ErrorDetail{}
	}

	envelope := ErrorEnvelope{
		Error: ErrorBody{
			Code:    errorCode,
			Message: message,
			Details: details,
			TraceID: traceID,
		},
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(envelope)
}

// GetOrCreateTraceID retrieves the trace ID from HTTP headers (traceparent, X-Trace-ID) or generates one.
func GetOrCreateTraceID(r *http.Request) string {
	if r != nil {
		if tid := r.Header.Get("X-Trace-ID"); tid != "" {
			return tid
		}
		if tp := r.Header.Get("traceparent"); tp != "" {
			parts := strings.Split(tp, "-")
			if len(parts) >= 2 && len(parts[1]) > 0 {
				return parts[1]
			}
		}
	}

	var buf [16]byte
	_, _ = rand.Read(buf[:])
	return hex.EncodeToString(buf[:])
}
