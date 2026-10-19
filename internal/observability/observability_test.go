package observability

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
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

// TestDistributedTracing_MFE_To_Service_To_Database directly proves the QA criterion:
// Demonstrates the unbroken continuity of a SINGLE OpenTelemetry trace spanning from the MFE,
// through the HTTP service layer, down to the PostgreSQL database operation.
func TestDistributedTracing_MFE_To_Service_To_Database(t *testing.T) {
	// 1. Setup OpenTelemetry with an in-memory exporter to capture all spans
	spanRecorder := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSpanProcessor(sdktrace.NewSimpleSpanProcessor(spanRecorder)),
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
	)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.TraceContext{})

	obs := &Observability{
		Registry: NewREDMetricsRegistry(),
		Logger:   NewStructuredLogger("training-environment-service", "1.0.0", "test"),
		Config: Config{
			ServiceName:    "training-environment-service",
			ServiceVersion: "1.0.0",
			Environment:    "test",
		},
	}

	// 2. Mock handler that receives the request and executes a database query using the trace context
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tr := otel.Tracer(TracerName)

		// Database operation executed within the active request context (as done by PostgreSQL repo)
		dbCtx, dbSpan := tr.Start(r.Context(), "db.query SELECT FROM training_environment.environment",
			trace.WithSpanKind(trace.SpanKindClient),
			trace.WithAttributes(
				semconv.DBSystemPostgreSQL,
				attribute.String("db.statement", "SELECT id, name, capacity FROM training_environment.environment WHERE is_active = true"),
				attribute.String("db.operation", "SELECT"),
			),
		)
		// Simulate DB latency
		time.Sleep(5 * time.Millisecond)
		dbSpan.End()

		tc := TraceFromContext(dbCtx)
		if tc == nil {
			t.Errorf("expected trace context to be propagated into DB context")
		}

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[{"id":"1","name":"Aula 101"}]`))
	})

	wrapped := obs.Middleware(handler)

	// 3. Simulate an incoming request originating from the Microfrontend (MFE) with a W3C traceparent header
	mfeTraceID := "4bf92f3577b34da6a3ce929d0e0e4736"
	mfeSpanID := "00f067aa0ba902b7"
	mfeTraceparent := "00-" + mfeTraceID + "-" + mfeSpanID + "-01"

	req := httptest.NewRequest("GET", "/training-environments", nil)
	req.Header.Set("traceparent", mfeTraceparent)

	rec := httptest.NewRecorder()
	wrapped.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rec.Code)
	}

	// 4. Inspect recorded OpenTelemetry spans
	spans := spanRecorder.GetSpans()
	if len(spans) < 2 {
		t.Fatalf("expected at least 2 recorded spans (HTTP Server + DB Client), got %d", len(spans))
	}

	var dbSpanSnap, httpSpanSnap tracetest.SpanStub
	for _, s := range spans {
		if strings.HasPrefix(s.Name, "db.query") {
			dbSpanSnap = s
		} else if strings.HasPrefix(s.Name, "GET") {
			httpSpanSnap = s
		}
	}

	// ASSERTION 1: HTTP Server span extracted the MFE Trace ID
	if httpSpanSnap.SpanContext.TraceID().String() != mfeTraceID {
		t.Errorf("expected HTTP span TraceID %s, got %s", mfeTraceID, httpSpanSnap.SpanContext.TraceID().String())
	}
	// ASSERTION 2: HTTP Server span has the MFE span as its parent
	if httpSpanSnap.Parent.SpanID().String() != mfeSpanID {
		t.Errorf("expected HTTP span Parent SpanID %s, got %s", mfeSpanID, httpSpanSnap.Parent.SpanID().String())
	}

	// ASSERTION 3: Database client span shares the EXACT same Trace ID (continuity of trace)
	if dbSpanSnap.SpanContext.TraceID().String() != mfeTraceID {
		t.Errorf("expected DB span TraceID %s, got %s", mfeTraceID, dbSpanSnap.SpanContext.TraceID().String())
	}
	// ASSERTION 4: Database client span has the HTTP Server span as its parent
	if dbSpanSnap.Parent.SpanID().String() != httpSpanSnap.SpanContext.SpanID().String() {
		t.Errorf("expected DB span Parent SpanID to equal HTTP Server SpanID %s, got %s",
			httpSpanSnap.SpanContext.SpanID().String(), dbSpanSnap.Parent.SpanID().String())
	}

	// ASSERTION 5: Database span has proper PostgreSQL attributes
	hasDBSystem := false
	for _, attr := range dbSpanSnap.Attributes {
		if string(attr.Key) == "db.system" && attr.Value.AsString() == "postgresql" {
			hasDBSystem = true
			break
		}
	}
	if !hasDBSystem {
		t.Errorf("expected db span to contain db.system=postgresql attribute")
	}

	// ASSERTION 6: Outgoing HTTP response carries the W3C traceparent header
	respTraceparent := rec.Header().Get("traceparent")
	if !strings.Contains(respTraceparent, mfeTraceID) {
		t.Errorf("expected response header traceparent to contain trace_id %s, got %s", mfeTraceID, respTraceparent)
	}

	t.Logf("Distributed Trace Verified Successfully:")
	t.Logf("  MFE Span ID:      %s", mfeSpanID)
	t.Logf("  HTTP Server Span: %s (Parent: %s)", httpSpanSnap.SpanContext.SpanID().String(), httpSpanSnap.Parent.SpanID().String())
	t.Logf("  DB Client Span:   %s (Parent: %s)", dbSpanSnap.SpanContext.SpanID().String(), dbSpanSnap.Parent.SpanID().String())
	t.Logf("  Shared Trace ID:  %s", mfeTraceID)
}
