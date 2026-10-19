package observability

import (
	"context"
	"log/slog"
	"os"
	"strings"
	"time"
)

// StructuredLogger wraps slog.Logger with OpenTelemetry and correlation context enrichment.
type StructuredLogger struct {
	logger         *slog.Logger
	serviceName    string
	serviceVersion string
	environment    string
}

// NewStructuredLogger creates a new JSON logger with default service attributes.
func NewStructuredLogger(serviceName, serviceVersion, environment string) *StructuredLogger {
	if serviceName == "" {
		serviceName = "training-environment-service"
	}
	if serviceVersion == "" {
		serviceVersion = "1.0.0"
	}
	if environment == "" {
		environment = "development"
	}

	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})

	baseLogger := slog.New(handler).With(
		slog.String("service.name", serviceName),
		slog.String("service.version", serviceVersion),
		slog.String("environment", environment),
	)

	return &StructuredLogger{
		logger:         baseLogger,
		serviceName:    serviceName,
		serviceVersion: serviceVersion,
		environment:    environment,
	}
}

// LogRequest emits a structured JSON log entry for an HTTP request with trace and correlation context.
func (l *StructuredLogger) LogRequest(ctx context.Context, method, path, clientIP, userAgent string, statusCode int, duration time.Duration) {
	tc := TraceFromContext(ctx)
	correlationID := CorrelationIDFromContext(ctx)

	traceID := ""
	spanID := ""
	traceparent := ""
	if tc != nil {
		traceID = tc.TraceID
		spanID = tc.SpanID
		traceparent = tc.FormatTraceparent()
		if correlationID == "" {
			correlationID = tc.CorrelationID
		}
	}

	durationMs := float64(duration.Microseconds()) / 1000.0

	// Determine log level based on HTTP status code
	var level slog.Level
	var msg string
	switch {
	case statusCode >= 500:
		level = slog.LevelError
		msg = "HTTP server error"
	case statusCode >= 400:
		level = slog.LevelWarn
		msg = "HTTP client error"
	default:
		level = slog.LevelInfo
		msg = "HTTP request completed"
	}

	attrs := []slog.Attr{
		slog.String("correlation_id", correlationID),
		slog.String("trace_id", traceID),
		slog.String("span_id", spanID),
		slog.String("traceparent", traceparent),
		slog.String("http.method", strings.ToUpper(method)),
		slog.String("http.route", path),
		slog.Int("http.status_code", statusCode),
		slog.Float64("http.duration_ms", durationMs),
		slog.String("http.client_ip", clientIP),
		slog.String("http.user_agent", userAgent),
	}

	l.logger.LogAttrs(ctx, level, msg, attrs...)
}

// Info logs an informational message enriched with trace context.
func (l *StructuredLogger) Info(ctx context.Context, msg string, args ...any) {
	tc := TraceFromContext(ctx)
	if tc != nil {
		args = append(args, "correlation_id", tc.CorrelationID, "trace_id", tc.TraceID)
	}
	l.logger.InfoContext(ctx, msg, args...)
}

// Error logs an error message enriched with trace context.
func (l *StructuredLogger) Error(ctx context.Context, msg string, args ...any) {
	tc := TraceFromContext(ctx)
	if tc != nil {
		args = append(args, "correlation_id", tc.CorrelationID, "trace_id", tc.TraceID)
	}
	l.logger.ErrorContext(ctx, msg, args...)
}
