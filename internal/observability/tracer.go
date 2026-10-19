package observability

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"time"

	"github.com/XSAM/otelsql"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

const (
	TracerName = "training-environment-service"
)

// OTelManager handles the lifecycle of OpenTelemetry TracerProvider, Propagators, and Exporters.
type OTelManager struct {
	TracerProvider *sdktrace.TracerProvider
	Propagator     propagation.TextMapPropagator
	Tracer         trace.Tracer
}

// InitOpenTelemetry initializes the official OpenTelemetry TracerProvider,
// registers W3C TraceContext and Baggage propagators globally, and configures
// OTLP gRPC and/or stdout exporters.
func InitOpenTelemetry(ctx context.Context, cfg Config) (*OTelManager, func(context.Context) error, error) {
	if cfg.ServiceName == "" {
		cfg.ServiceName = "training-environment-service"
	}
	if cfg.ServiceVersion == "" {
		cfg.ServiceVersion = "1.0.0"
	}
	if cfg.Environment == "" {
		cfg.Environment = "development"
	}

	// 1. Build OpenTelemetry Resource with semantic conventions
	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceNameKey.String(cfg.ServiceName),
			semconv.ServiceVersionKey.String(cfg.ServiceVersion),
			semconv.DeploymentEnvironmentKey.String(cfg.Environment),
		),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create otel resource: %w", err)
	}

	// 2. Build Span Processors
	var spanProcessors []sdktrace.SpanProcessor

	// Check if OTLP gRPC endpoint is specified (e.g., Tempo or OpenTelemetry Collector)
	otlpEndpoint := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")
	if otlpEndpoint != "" {
		otlpEndpoint = strings.TrimPrefix(otlpEndpoint, "http://")
		otlpEndpoint = strings.TrimPrefix(otlpEndpoint, "https://")
		dialCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		defer cancel()

		conn, err := grpc.DialContext(dialCtx, otlpEndpoint,
			grpc.WithTransportCredentials(insecure.NewCredentials()),
			grpc.WithBlock(),
		)
		if err == nil {
			otlpExporter, err := otlptracegrpc.New(ctx, otlptracegrpc.WithGRPCConn(conn))
			if err == nil {
				spanProcessors = append(spanProcessors, sdktrace.NewBatchSpanProcessor(otlpExporter))
				log.Printf("[OpenTelemetry] OTLP gRPC exporter active towards %s", otlpEndpoint)
			}
		} else {
			log.Printf("[OpenTelemetry] Warning: Could not connect to OTLP collector at %s: %v (continuing with local telemetry)", otlpEndpoint, err)
		}
	}

	// Check if stdout exporter is explicitly requested or used in debug
	tracesExporter := strings.ToLower(os.Getenv("OTEL_TRACES_EXPORTER"))
	if tracesExporter == "stdout" {
		stdoutExp, err := stdouttrace.New(stdouttrace.WithPrettyPrint())
		if err == nil {
			spanProcessors = append(spanProcessors, sdktrace.NewSimpleSpanProcessor(stdoutExp))
			log.Println("[OpenTelemetry] Stdout span exporter active")
		}
	} else if len(spanProcessors) == 0 {
		// Silent stdout discard exporter when no collector is configured, ensuring TracerProvider always records valid spans
		discardExp, _ := stdouttrace.New(stdouttrace.WithWriter(io.Discard))
		spanProcessors = append(spanProcessors, sdktrace.NewSimpleSpanProcessor(discardExp))
	}

	// 3. Configure TracerProvider with ParentBased AlwaysSample sampler
	opts := []sdktrace.TracerProviderOption{
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.AlwaysSample())),
	}
	for _, sp := range spanProcessors {
		opts = append(opts, sdktrace.WithSpanProcessor(sp))
	}

	tp := sdktrace.NewTracerProvider(opts...)

	// 4. Register global TracerProvider and W3C TextMapPropagator
	otel.SetTracerProvider(tp)

	propagator := propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	)
	otel.SetTextMapPropagator(propagator)

	manager := &OTelManager{
		TracerProvider: tp,
		Propagator:     propagator,
		Tracer:         tp.Tracer(TracerName),
	}

	shutdown := func(shutdownCtx context.Context) error {
		return tp.Shutdown(shutdownCtx)
	}

	return manager, shutdown, nil
}

// OpenTracedPostgresDB opens a PostgreSQL connection wrapped with OpenTelemetry instrumentation (otelsql).
// Every QueryContext, ExecContext, and Transaction automatically creates child spans linked to the active HTTP trace.
func OpenTracedPostgresDB(dbURL string) (*sql.DB, error) {
	db, err := otelsql.Open("postgres", dbURL,
		otelsql.WithAttributes(
			semconv.DBSystemPostgreSQL,
			attribute.String("db.name", "training_environment_db"),
		),
		otelsql.WithSpanOptions(otelsql.SpanOptions{
			OmitConnResetSession: true,
		}),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to open traced postgres db: %w", err)
	}

	// Register DB stats metrics for Prometheus/OTel monitoring
	_, _ = otelsql.RegisterDBStatsMetrics(db, otelsql.WithAttributes(
		semconv.DBSystemPostgreSQL,
	))

	return db, nil
}
