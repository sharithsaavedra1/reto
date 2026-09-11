package main

import (
	"database/sql"
	"log"
	"net/http"
	"os"

	_ "github.com/lib/pq"

	httpAdapter "github.com/code-sena/design-software-training-environment-service/internal/adapter/in/http"
	"github.com/code-sena/design-software-training-environment-service/internal/adapter/in/http/middleware"
	"github.com/code-sena/design-software-training-environment-service/internal/adapter/out/persistence"
	"github.com/code-sena/design-software-training-environment-service/internal/application/port"
	"github.com/code-sena/design-software-training-environment-service/internal/application/usecase"
)

func main() {
	// 1. Storage repository setup (PostgreSQL or In-Memory fallback)
	var repo port.EnvironmentTypeRepository
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL != "" {
		db, err := sql.Open("postgres", dbURL)
		if err != nil {
			log.Fatalf("failed to connect to postgres: %v", err)
		}
		defer db.Close()
		repo = persistence.NewPostgresEnvironmentTypeRepository(db)
		log.Println("connected to PostgreSQL repository")
	} else {
		repo = persistence.NewMemoryEnvironmentTypeRepository()
		log.Println("using in-memory repository with default seed data")
	}

	// 2. Application use case setup
	envTypeUseCase := usecase.NewEnvironmentTypeUseCase(repo)

	// 3. Security middleware setup
	jwtSecret := env("JWT_SECRET", "dev-secret-key")
	authMiddleware := middleware.NewAuthMiddleware(jwtSecret)

	// 4. HTTP Router and Handlers
	mux := http.NewServeMux()

	// Health check endpoint
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	// Register HU-ENV-001 routes
	envTypeHandler := httpAdapter.NewEnvironmentTypeHandler(envTypeUseCase)
	envTypeHandler.RegisterRoutes(mux, authMiddleware)

	addr := ":" + env("PORT", "8080")
	log.Printf("training-environment-service API listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}

func env(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
