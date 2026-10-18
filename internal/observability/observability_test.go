package observability

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestTraceContext_ParseTraceparent(t *testing.T) {
	// Valid W3C traceparent
	validHeader := "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"
	tc, err := ParseTraceparent(validHeader)
	if err != nil {
		t.Fatalf("unexpected error parsing valid traceparent: %v", err)
	}
	if tc.Version != "00" {
		t.Errorf("expected version 00, got %s", tc.Version)
	}
	if tc.TraceID != "4bf92f3577b34da6a3ce929d0e0e4736" {
		t.Errorf("unexpected trace_id: %s", tc.TraceID)
	}
	if tc.SpanID != "00f067aa0ba902b7" {
		t.Errorf("unexpected span_id: %s", tc.SpanID)
	}
	if tc.TraceFlags != "01" {
		t.Errorf("unexpected trace_flags: %s", tc.TraceFlags)
	}
	if tc.FormatTraceparent() != validHeader {
		t.Errorf("expected re-formatted header %s, got %s", validHeader, tc.FormatTraceparent())
	}

	// Invalid cases
	invalidCases := []string{
		"",
		"invalid-format",
		"00-short-id-01",
		"00-00000000000000000000000000000000-00f067aa0ba902b7-01", // all zeros trace_id
		"00-4bf92f3577b34da6a3ce929d0e0e4736-0000000000000000-01", // all zeros span_id
		"ff-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01", // version ff
		"00-not-hex-chars-32-characters-long!-00f067aa0ba902b7-01", // non-hex
	}

	for _, c := range invalidCases {
		if _, err := ParseTraceparent(c); err == nil {
			t.Errorf("expected error for invalid traceparent %q, got nil", c)
		}
	}
}

func TestTraceContext_NewAndPropagation(t *testing.T) {
	tc := NewTraceContext()
	if len(tc.TraceID) != 32 {
		t.Errorf("expected 32 hex chars trace_id, got %d", len(tc.TraceID))
	}
	if len(tc.SpanID) != 16 {
		t.Errorf("expected 16 hex chars span_id, got %d", len(tc.SpanID))
	}

	ctx := ContextWithTrace(context.Background(), tc)
	retrieved := TraceFromContext(ctx)
	if retrieved == nil || retrieved.TraceID != tc.TraceID {
		t.Errorf("failed to retrieve trace from context")
	}

	cid := CorrelationIDFromContext(ctx)
	if cid != tc.TraceID {
		t.Errorf("expected correlation ID %s, got %s", tc.TraceID, cid)
	}
}

func TestREDMetricsRegistry(t *testing.T) {
	reg := NewREDMetricsRegistry()

	// Record successes
	reg.RecordRequest("GET", "/training-environments", http.StatusOK, 15*time.Millisecond)
	reg.RecordRequest("GET", "/training-environments", http.StatusOK, 20*time.Millisecond)

	// Record errors
	reg.RecordRequest("POST", "/training-environments", http.StatusUnprocessableEntity, 5*time.Millisecond)
	reg.RecordRequest("GET", "/training-environments/11111111-2222-3333-4444-555555555555", http.StatusNotFound, 2*time.Millisecond)

	total, errors, errorRate := reg.GetSummary()
	if total != 4 {
		t.Errorf("expected total 4, got %d", total)
	}
	if errors != 2 {
		t.Errorf("expected errors 2, got %d", errors)
	}
	if errorRate != 50.0 {
		t.Errorf("expected error rate 50%%, got %.2f%%", errorRate)
	}

	prom := reg.ExportPrometheus()
	if !strings.Contains(prom, "http_requests_total") {
		t.Errorf("expected prom output to contain http_requests_total")
	}
	if !strings.Contains(prom, "http_requests_errors_total") {
		t.Errorf("expected prom output to contain http_requests_errors_total")
	}
	if !strings.Contains(prom, "http_request_duration_seconds_bucket") {
		t.Errorf("expected prom output to contain http_request_duration_seconds_bucket")
	}
	// Verify path sanitization parameterized the UUID
	if !strings.Contains(prom, "path=\"/training-environments/{id}\"") {
		t.Errorf("expected sanitized path /training-environments/{id} in metrics")
	}
}

func TestObservability_MiddlewareIntegration(t *testing.T) {
	obs := New(Config{
		ServiceName:    "test-service",
		ServiceVersion: "1.0.0",
		Environment:    "test",
	})

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tc := TraceFromContext(r.Context())
		if tc == nil {
			t.Errorf("expected trace context in request context")
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"status":"created"}`))
	})

	wrapped := obs.Middleware(handler)

	// Case 1: Incoming W3C traceparent and X-Correlation-ID
	req := httptest.NewRequest("POST", "/training-environments", strings.NewReader("{}"))
	req.Header.Set("traceparent", "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01")
	req.Header.Set("x-correlation-id", "corr-test-12345")

	rec := httptest.NewRecorder()
	wrapped.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Errorf("expected 201 Created, got %d", rec.Code)
	}

	// Verify outgoing headers
	respTraceparent := rec.Header().Get("traceparent")
	if !strings.HasPrefix(respTraceparent, "00-4bf92f3577b34da6a3ce929d0e0e4736-") {
		t.Errorf("expected outgoing traceparent to preserve trace_id, got %s", respTraceparent)
	}

	respCorrelationID := rec.Header().Get("x-correlation-id")
	if respCorrelationID != "corr-test-12345" {
		t.Errorf("expected outgoing x-correlation-id corr-test-12345, got %s", respCorrelationID)
	}

	// Case 2: Verify /metrics endpoint
	metricsRec := httptest.NewRecorder()
	obs.MetricsHandler().ServeHTTP(metricsRec, httptest.NewRequest("GET", "/metrics", nil))

	if metricsRec.Code != http.StatusOK {
		t.Errorf("expected 200 OK from /metrics, got %d", metricsRec.Code)
	}
	body := metricsRec.Body.String()
	if !strings.Contains(body, "http_requests_total{method=\"POST\",path=\"/training-environments\",status=\"201\"} 1") {
		t.Errorf("expected metrics to reflect POST request: %s", body)
	}
}
