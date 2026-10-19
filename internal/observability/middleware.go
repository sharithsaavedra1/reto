package observability

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

// Config configures the Observability suite.
type Config struct {
	ServiceName    string
	ServiceVersion string
	Environment    string
}

// Observability coordinates Distributed Tracing, RED Metrics, and Structured Logging.
type Observability struct {
	Registry *REDMetricsRegistry
	Logger   *StructuredLogger
	OTel     *OTelManager
	Shutdown func(context.Context) error
	Config   Config
}

// New initializes the complete Observability system with official OpenTelemetry SDK.
func New(cfg Config) *Observability {
	if cfg.ServiceName == "" {
		cfg.ServiceName = "training-environment-service"
	}
	if cfg.ServiceVersion == "" {
		cfg.ServiceVersion = "1.0.0"
	}
	if cfg.Environment == "" {
		cfg.Environment = "development"
	}

	otelMgr, shutdown, err := InitOpenTelemetry(context.Background(), cfg)
	if err != nil {
		fmt.Printf("[Observability] Warning: OpenTelemetry init error: %v\n", err)
	}

	return &Observability{
		Registry: NewREDMetricsRegistry(),
		Logger:   NewStructuredLogger(cfg.ServiceName, cfg.ServiceVersion, cfg.Environment),
		OTel:     otelMgr,
		Shutdown: shutdown,
		Config:   cfg,
	}
}

// MetricsHandler exposes the Prometheus exposition endpoint.
func (o *Observability) MetricsHandler() http.HandlerFunc {
	return o.Registry.MetricsHandler()
}

// Middleware creates an HTTP middleware that extracts/propagates OpenTelemetry W3C TraceContext,
// starts server spans, records RED metrics, emits structured logs, and propagates correlation headers.
func (o *Observability) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// 1. In-flight requests gauge
		o.Registry.IncInFlight()
		defer o.Registry.DecInFlight()

		// 2. Extract W3C Trace Context from incoming HTTP headers (traceparent, tracestate, baggage)
		extractedCtx := otel.GetTextMapPropagator().Extract(r.Context(), propagation.HeaderCarrier(r.Header))

		// 3. Extract or correlate Correlation-ID (checking x-correlation-id, x-request-id)
		incomingCID := strings.TrimSpace(r.Header.Get(CorrelationHeader))
		if incomingCID == "" {
			incomingCID = strings.TrimSpace(r.Header.Get(RequestIDHeader))
		}

		// 4. Start OpenTelemetry Server Span
		tr := otel.Tracer(TracerName)
		spanName := fmt.Sprintf("%s %s", r.Method, r.URL.Path)
		ctx, span := tr.Start(extractedCtx, spanName,
			trace.WithSpanKind(trace.SpanKindServer),
			trace.WithAttributes(
				attribute.String("http.method", r.Method),
				attribute.String("http.target", r.URL.Path),
				attribute.String("http.client_ip", extractClientIP(r)),
				attribute.String("http.user_agent", r.UserAgent()),
			),
		)
		defer span.End()

		spanCtx := span.SpanContext()
		traceID := ""
		spanID := ""
		if spanCtx.IsValid() {
			traceID = spanCtx.TraceID().String()
			spanID = spanCtx.SpanID().String()
		}

		if incomingCID == "" {
			incomingCID = traceID
		}
		ctx = ContextWithCorrelationID(ctx, incomingCID)

		// 5. Inject OpenTelemetry W3C Trace Context into response headers
		otel.GetTextMapPropagator().Inject(ctx, propagation.HeaderCarrier(w.Header()))

		// Set explicit canonical headers for downstream MFEs, Gateways, and clients
		if spanCtx.IsValid() {
			w.Header().Set(TraceparentHeader, fmt.Sprintf("00-%s-%s-%s", traceID, spanID, spanCtx.TraceFlags().String()))
			w.Header().Set(CorrelationHeader, incomingCID)
			w.Header().Set(TraceIDHeader, traceID)
		}

		// 6. Wrap ResponseWriter to capture HTTP status code and written size
		rw := &responseWriterInterceptor{ResponseWriter: w, statusCode: http.StatusOK}

		// 7. Execute request handler pipeline with trace-enriched context
		next.ServeHTTP(rw, r.WithContext(ctx))

		duration := time.Since(start)

		// 8. Update Span Status and attributes based on response code
		span.SetAttributes(attribute.Int("http.status_code", rw.statusCode))
		if rw.statusCode >= 500 {
			span.SetStatus(codes.Error, fmt.Sprintf("Server error HTTP %d", rw.statusCode))
		} else if rw.statusCode >= 400 {
			span.SetStatus(codes.Error, fmt.Sprintf("Client error HTTP %d", rw.statusCode))
		} else {
			span.SetStatus(codes.Ok, "OK")
		}

		// Do not pollute application metrics with /metrics calls
		if r.URL.Path != "/metrics" {
			// 9. Record RED Metrics (Rate, Errors, Duration)
			o.Registry.RecordRequest(r.Method, r.URL.Path, rw.statusCode, duration)

			// 10. Emit Structured JSON Log
			clientIP := extractClientIP(r)
			userAgent := r.UserAgent()
			o.Logger.LogRequest(ctx, r.Method, r.URL.Path, clientIP, userAgent, rw.statusCode, duration)
		}
	})
}

// responseWriterInterceptor wraps http.ResponseWriter to capture the emitted status code.
type responseWriterInterceptor struct {
	http.ResponseWriter
	statusCode  int
	wroteHeader bool
	bytesCount  int
}

func (rw *responseWriterInterceptor) WriteHeader(code int) {
	if !rw.wroteHeader {
		rw.statusCode = code
		rw.wroteHeader = true
		rw.ResponseWriter.WriteHeader(code)
	}
}

func (rw *responseWriterInterceptor) Write(b []byte) (int, error) {
	if !rw.wroteHeader {
		rw.WriteHeader(http.StatusOK)
	}
	n, err := rw.ResponseWriter.Write(b)
	rw.bytesCount += n
	return n, err
}

func extractClientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		return strings.TrimSpace(parts[0])
	}
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return strings.TrimSpace(xri)
	}
	parts := strings.Split(r.RemoteAddr, ":")
	if len(parts) > 0 {
		return parts[0]
	}
	return r.RemoteAddr
}
