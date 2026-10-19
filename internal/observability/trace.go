package observability

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"go.opentelemetry.io/otel/trace"
)

// W3C Trace Context Constants
const (
	TraceparentHeader = "traceparent"
	TracestateHeader  = "tracestate"
	CorrelationHeader = "x-correlation-id"
	RequestIDHeader   = "x-request-id"
	TraceIDHeader     = "x-trace-id"

	CurrentTraceVersion = "00"
	DefaultTraceFlags   = "01" // Sampled / Recorded
)

type contextKey string

const (
	traceContextKey       contextKey = "otel_trace_context"
	correlationContextKey contextKey = "correlation_id"
)

// TraceContext represents an OpenTelemetry W3C compliant trace context.
type TraceContext struct {
	Version       string `json:"version"`
	TraceID       string `json:"trace_id"`
	SpanID        string `json:"span_id"`
	TraceFlags    string `json:"trace_flags"`
	Tracestate    string `json:"tracestate,omitempty"`
	CorrelationID string `json:"correlation_id"`
	IsRemote      bool   `json:"is_remote"`
}

// FormatTraceparent returns the canonical W3C traceparent string: 00-{trace_id}-{span_id}-{trace_flags}
func (t *TraceContext) FormatTraceparent() string {
	if t == nil {
		return ""
	}
	ver := t.Version
	if ver == "" {
		ver = CurrentTraceVersion
	}
	flags := t.TraceFlags
	if flags == "" {
		flags = DefaultTraceFlags
	}
	return fmt.Sprintf("%s-%s-%s-%s", ver, t.TraceID, t.SpanID, flags)
}

// ParseTraceparent parses a W3C traceparent header string according to the W3C Trace Context spec.
func ParseTraceparent(header string) (*TraceContext, error) {
	header = strings.TrimSpace(header)
	if header == "" {
		return nil, errors.New("empty traceparent header")
	}

	parts := strings.Split(header, "-")
	if len(parts) < 4 {
		return nil, fmt.Errorf("invalid traceparent format: expected at least 4 segments, got %d", len(parts))
	}

	version := parts[0]
	traceID := strings.ToLower(parts[1])
	spanID := strings.ToLower(parts[2])
	traceFlags := parts[3]

	// Validate version
	if len(version) != 2 || !isValidHex(version) {
		return nil, errors.New("invalid traceparent version")
	}
	if version == "ff" {
		return nil, errors.New("unsupported traceparent version ff")
	}

	// Validate trace-id (16 bytes = 32 hex chars, cannot be all zeros)
	if len(traceID) != 32 || !isValidHex(traceID) || isAllZeros(traceID) {
		return nil, errors.New("invalid trace-id: must be 32 hex chars and not all zeros")
	}

	// Validate parent-id/span-id (8 bytes = 16 hex chars, cannot be all zeros)
	if len(spanID) != 16 || !isValidHex(spanID) || isAllZeros(spanID) {
		return nil, errors.New("invalid span-id: must be 16 hex chars and not all zeros")
	}

	// Validate trace-flags (1 byte = 2 hex chars)
	if len(traceFlags) != 2 || !isValidHex(traceFlags) {
		return nil, errors.New("invalid trace-flags: must be 2 hex chars")
	}

	return &TraceContext{
		Version:       version,
		TraceID:       traceID,
		SpanID:        spanID,
		TraceFlags:    traceFlags,
		CorrelationID: traceID,
		IsRemote:      true,
	}, nil
}

// NewTraceContext creates a brand new W3C compliant TraceContext with cryptographically secure IDs.
func NewTraceContext() *TraceContext {
	traceID := generateHex(16) // 16 bytes = 32 hex characters
	spanID := generateHex(8)   // 8 bytes = 16 hex characters

	return &TraceContext{
		Version:       CurrentTraceVersion,
		TraceID:       traceID,
		SpanID:        spanID,
		TraceFlags:    DefaultTraceFlags,
		CorrelationID: traceID,
		IsRemote:      false,
	}
}

// NewChildSpan creates a child span context under the current trace.
func (t *TraceContext) NewChildSpan() *TraceContext {
	return &TraceContext{
		Version:       t.Version,
		TraceID:       t.TraceID,
		SpanID:        generateHex(8),
		TraceFlags:    t.TraceFlags,
		Tracestate:    t.Tracestate,
		CorrelationID: t.CorrelationID,
		IsRemote:      false,
	}
}

// ContextWithTrace stores the TraceContext in the context.Context.
func ContextWithTrace(ctx context.Context, tc *TraceContext) context.Context {
	if tc == nil {
		return ctx
	}
	ctx = context.WithValue(ctx, traceContextKey, tc)
	ctx = context.WithValue(ctx, correlationContextKey, tc.CorrelationID)
	return ctx
}

// ContextWithCorrelationID attaches a correlation ID to context.Context.
func ContextWithCorrelationID(ctx context.Context, correlationID string) context.Context {
	if strings.TrimSpace(correlationID) == "" {
		return ctx
	}
	return context.WithValue(ctx, correlationContextKey, correlationID)
}

// TraceFromContext retrieves the TraceContext from context.Context, preferring active OpenTelemetry span context.
func TraceFromContext(ctx context.Context) *TraceContext {
	if ctx == nil {
		return nil
	}

	// 1. Inspect active OpenTelemetry span
	span := trace.SpanFromContext(ctx)
	if span != nil && span.SpanContext().IsValid() {
		sc := span.SpanContext()
		cid := ""
		if v, ok := ctx.Value(correlationContextKey).(string); ok {
			cid = v
		}
		if cid == "" {
			cid = sc.TraceID().String()
		}
		return &TraceContext{
			Version:       CurrentTraceVersion,
			TraceID:       sc.TraceID().String(),
			SpanID:        sc.SpanID().String(),
			TraceFlags:    sc.TraceFlags().String(),
			CorrelationID: cid,
			IsRemote:      sc.IsRemote(),
		}
	}

	// 2. Fallback to manually stored TraceContext
	if tc, ok := ctx.Value(traceContextKey).(*TraceContext); ok {
		return tc
	}
	return nil
}

// CorrelationIDFromContext retrieves the correlation_id from context.Context, or empty string.
func CorrelationIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if cid, ok := ctx.Value(correlationContextKey).(string); ok && cid != "" {
		return cid
	}
	if tc := TraceFromContext(ctx); tc != nil {
		return tc.CorrelationID
	}
	return ""
}

func generateHex(byteCount int) string {
	b := make([]byte, byteCount)
	_, err := rand.Read(b)
	if err != nil {
		// Fallback should rand fail (extremely unlikely)
		return strings.Repeat("a", byteCount*2)
	}
	return hex.EncodeToString(b)
}

func isValidHex(s string) bool {
	_, err := hex.DecodeString(s)
	return err == nil
}

func isAllZeros(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] != '0' {
			return false
		}
	}
	return true
}
