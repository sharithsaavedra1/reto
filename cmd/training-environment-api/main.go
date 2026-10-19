package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	_ "github.com/lib/pq"

	httpAdapter "github.com/code-sena/design-software-training-environment-service/internal/adapter/in/http"
	"github.com/code-sena/design-software-training-environment-service/internal/adapter/in/http/middleware"
	"github.com/code-sena/design-software-training-environment-service/internal/adapter/out/persistence"
	"github.com/code-sena/design-software-training-environment-service/internal/application/port"
	"github.com/code-sena/design-software-training-environment-service/internal/application/usecase"
	"github.com/code-sena/design-software-training-environment-service/internal/observability"
)

func main() {
	appEnv := strings.ToLower(env("APP_ENV", env("ENVIRONMENT", env("ENV", "development"))))

	// 1. Observability suite initialization (OpenTelemetry TracerProvider, RED Metrics, Structured Logging)
	obs := observability.New(observability.Config{
		ServiceName:    "training-environment-service",
		ServiceVersion: "1.0.0",
		Environment:    appEnv,
	})
	if obs.Shutdown != nil {
		defer func() {
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = obs.Shutdown(shutdownCtx)
		}()
	}

	// 2. Storage repositories setup (PostgreSQL with OpenTelemetry instrumentation or In-Memory fallback)
	var envTypeRepo port.EnvironmentTypeRepository
	var envRepo port.EnvironmentRepository
	var availabilityRuleRepo port.AvailabilityRuleRepository
	var maintenanceRepo port.MaintenanceRepository
	var reservationRepo port.ReservationRepository
	var utilizationRepo port.EnvironmentUtilizationRepository

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL != "" {
		db, err := observability.OpenTracedPostgresDB(dbURL)
		if err != nil {
			log.Fatalf("failed to connect to traced postgres: %v", err)
		}
		defer db.Close()
		envTypeRepo = persistence.NewPostgresEnvironmentTypeRepository(db)
		envRepo = persistence.NewPostgresEnvironmentRepository(db)
		availabilityRuleRepo = persistence.NewPostgresAvailabilityRuleRepository(db)
		maintenanceRepo = persistence.NewPostgresMaintenanceRepository(db)
		reservationRepo = persistence.NewPostgresReservationRepository(db)
		utilizationRepo = persistence.NewPostgresEnvironmentUtilizationRepository(db)
		log.Println("connected to PostgreSQL repositories with OpenTelemetry instrumentation")
	} else {
		envTypeRepo = persistence.NewMemoryEnvironmentTypeRepository()
		envRepo = persistence.NewMemoryEnvironmentRepository()
		availabilityRuleRepo = persistence.NewMemoryAvailabilityRuleRepository(envRepo)
		maintenanceRepo = persistence.NewMemoryMaintenanceRepository(envRepo)
		reservationRepo = persistence.NewMemoryReservationRepository(envRepo)
		utilizationRepo = persistence.NewMemoryEnvironmentUtilizationRepository(envRepo, envTypeRepo, availabilityRuleRepo, maintenanceRepo, reservationRepo)
		log.Println("using in-memory repositories with default seed data")
	}

	// 3. Application use cases setup
	envTypeUseCase := usecase.NewEnvironmentTypeUseCase(envTypeRepo)
	envUseCase := usecase.NewEnvironmentUseCase(envRepo, envTypeRepo)

	// 4. Security middleware setup with strict environment checks (RS256 / JWKS IAM Governance & HS256 dev fallback)
	jwtSecret := strings.TrimSpace(os.Getenv("JWT_SECRET"))
	jwtPublicKey := strings.TrimSpace(os.Getenv("JWT_PUBLIC_KEY"))
	jwksURL := strings.TrimSpace(os.Getenv("JWKS_URL"))
	jwksJSON := strings.TrimSpace(os.Getenv("JWKS_JSON"))

	if jwtSecret == "" && jwtPublicKey == "" && jwksURL == "" && jwksJSON == "" {
		if isNonLocalEnv(appEnv) {
			log.Fatalf("FATAL: Authentication configuration (JWT_PUBLIC_KEY, JWKS_URL, or JWT_SECRET) is mandatory in '%s' environment. Startup aborted.", appEnv)
		}
		log.Println("WARNING: No JWT key configured. Using temporary dev secret for local development only.")
		jwtSecret = "dev-secret-key"
	}
	authMiddleware := middleware.NewAuthMiddleware(jwtSecret)

	// 5. HTTP Router and Handlers
	mux := http.NewServeMux()

	// Metrics endpoint (Prometheus text exposition format)
	mux.HandleFunc("GET /metrics", obs.MetricsHandler())

	// Health check endpoint
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	// Register HU-ENV-001 routes (/environment-types)
	envTypeHandler := httpAdapter.NewEnvironmentTypeHandler(envTypeUseCase)
	envTypeHandler.RegisterRoutes(mux, authMiddleware)

	// Register HU-ENV-002 routes (/training-environments)
	envHandler := httpAdapter.NewEnvironmentHandler(envUseCase)
	envHandler.RegisterRoutes(mux, authMiddleware)

	// Register HU-ENV-005 routes (/training-environments/{id}/availability-rules)
	availabilityRuleUseCase := usecase.NewAvailabilityRuleUseCase(availabilityRuleRepo)
	availabilityRuleHandler := httpAdapter.NewAvailabilityRuleHandler(availabilityRuleUseCase)
	availabilityRuleHandler.RegisterRoutes(mux, authMiddleware)

	// Register HU-ENV-006 routes (/training-environments/{id}/maintenance)
	maintenanceUseCase := usecase.NewMaintenanceUseCase(maintenanceRepo)
	maintenanceHandler := httpAdapter.NewMaintenanceHandler(maintenanceUseCase)
	maintenanceHandler.RegisterRoutes(mux, authMiddleware)

	// Register HU-ENV-007/008 routes (/reservations)
	reservationUseCase := usecase.NewReservationUseCase(reservationRepo, envRepo)
	reservationHandler := httpAdapter.NewReservationHandler(reservationUseCase)
	reservationHandler.RegisterRoutes(mux, authMiddleware)

	// Register HU-ENV-010 routes (/reports/environment-utilization)
	utilizationUseCase := usecase.NewEnvironmentUtilizationUseCase(utilizationRepo)
	utilizationHandler := httpAdapter.NewEnvironmentUtilizationHandler(utilizationUseCase)
	utilizationHandler.RegisterRoutes(mux, authMiddleware)

	addr := ":" + env("PORT", "8080")
	log.Printf("training-environment-service API listening on %s (env: %s) with OpenTelemetry W3C Tracing and RED Metrics (/metrics)", addr, appEnv)

	// Wrap entire HTTP router with Observability Middleware
	rootHandler := obs.Middleware(mux)
	log.Fatal(http.ListenAndServe(addr, rootHandler))
}

func isNonLocalEnv(e string) bool {
	switch e {
	case "qa", "staging", "prod", "production", "main":
		return true
	default:
		return false
	}
}

func env(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
