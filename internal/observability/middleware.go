package observability

import (
	"net/http"
	"strings"
	"time"
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
	Config   Config
}

// New initializes the complete Observability system.
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

	return &Observability{
		Registry: NewREDMetricsRegistry(),
		Logger:   NewStructuredLogger(cfg.ServiceName, cfg.ServiceVersion, cfg.Environment),
		Config:   cfg,
	}
}

// MetricsHandler exposes the Prometheus exposition endpoint.
func (o *Observability) MetricsHandler() http.HandlerFunc {
	return o.Registry.MetricsHandler()
}

// Middleware creates an HTTP middleware that extracts/generates W3C TraceContext,
// records RED metrics, emits structured logs, and propagates correlation headers.
func (o *Observability) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// 1. In-flight requests gauge
		o.Registry.IncInFlight()
		defer o.Registry.DecInFlight()

		// 2. Extract or generate OpenTelemetry W3C Trace Context
		traceparentHeader := r.Header.Get(TraceparentHeader)
		var traceCtx *TraceContext
		var err error

		if traceparentHeader != "" {
			traceCtx, err = ParseTraceparent(traceparentHeader)
		}
		if traceCtx == nil || err != nil {
			traceCtx = NewTraceContext()
		}

		// 3. Extract or correlate Correlation-ID (checking x-correlation-id, x-request-id)
		incomingCID := strings.TrimSpace(r.Header.Get(CorrelationHeader))
		if incomingCID == "" {
			incomingCID = strings.TrimSpace(r.Header.Get(RequestIDHeader))
		}
		if incomingCID != "" {
			traceCtx.CorrelationID = incomingCID
		}

		// Extract optional tracestate
		if ts := r.Header.Get(TracestateHeader); ts != "" {
			traceCtx.Tracestate = ts
		}

		// 4. Inject trace and correlation context into Go request context
		ctx := ContextWithTrace(r.Context(), traceCtx)
		r = r.WithContext(ctx)

		// 5. Inject response headers for frontend (MFE) / gateway propagation
		w.Header().Set(TraceparentHeader, traceCtx.FormatTraceparent())
		w.Header().Set(CorrelationHeader, traceCtx.CorrelationID)
		w.Header().Set(TraceIDHeader, traceCtx.TraceID)
		if traceCtx.Tracestate != "" {
			w.Header().Set(TracestateHeader, traceCtx.Tracestate)
		}

		// 6. Wrap ResponseWriter to capture HTTP status code and written size
		rw := &responseWriterInterceptor{ResponseWriter: w, statusCode: http.StatusOK}

		// 7. Execute request handler pipeline
		next.ServeHTTP(rw, r)

		duration := time.Since(start)

		// Do not pollute application metrics with /metrics calls
		if r.URL.Path != "/metrics" {
			// 8. Record RED Metrics (Rate, Errors, Duration)
			o.Registry.RecordRequest(r.Method, r.URL.Path, rw.statusCode, duration)

			// 9. Emit Structured JSON Log
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
