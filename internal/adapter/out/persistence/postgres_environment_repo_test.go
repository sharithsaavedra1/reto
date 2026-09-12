package persistence_test

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	_ "github.com/lib/pq"

	"github.com/code-sena/design-software-training-environment-service/internal/adapter/out/persistence"
	"github.com/code-sena/design-software-training-environment-service/internal/application/usecase"
	"github.com/code-sena/design-software-training-environment-service/internal/domain/model"
)

func getTestPostgresDB(t *testing.T) *sql.DB {
	dbURL := os.Getenv("TEST_DATABASE_URL")
	if dbURL == "" {
		dbURL = os.Getenv("DATABASE_URL")
	}
	if dbURL == "" {
		t.Skip("Skipping PostgreSQL integration test: DATABASE_URL / TEST_DATABASE_URL not set")
		return nil
	}

	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		t.Skipf("Skipping PostgreSQL integration test: unable to open connection: %v", err)
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		t.Skipf("Skipping PostgreSQL integration test: ping failed: %v", err)
		return nil
	}

	return db
}

func TestPostgresEnvironmentRepository_Integration(t *testing.T) {
	db := getTestPostgresDB(t)
	if db == nil {
		return
	}
	defer db.Close()

	ctx := context.Background()

	// Ensure schema & table exists for integration testing
	_, err := db.ExecContext(ctx, `
		CREATE SCHEMA IF NOT EXISTS training_environment;
		CREATE TABLE IF NOT EXISTS training_environment.environment_type (
			id VARCHAR(36) PRIMARY KEY,
			code VARCHAR(20) UNIQUE NOT NULL,
			name VARCHAR(100) NOT NULL,
			description VARCHAR(255),
			created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
		);
		CREATE TABLE IF NOT EXISTS training_environment.environment (
			id VARCHAR(36) PRIMARY KEY,
			environment_type_id VARCHAR(36) NOT NULL,
			training_center_id VARCHAR(36) NOT NULL,
			name VARCHAR(100) NOT NULL,
			capacity INT NOT NULL,
			location VARCHAR(200),
			is_active BOOLEAN NOT NULL DEFAULT TRUE,
			created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
			updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
			CONSTRAINT chk_environment_capacity CHECK (capacity > 0),
			CONSTRAINT fk_environment_environment_type FOREIGN KEY (environment_type_id) REFERENCES training_environment.environment_type(id)
		);
	`)
	if err != nil {
		t.Skipf("Skipping PostgreSQL integration test: unable to prepare tables: %v", err)
		return
	}

	// Insert test environment_type seed
	typeID := "e3f2a1b0-1234-5678-90ab-cdef12345678"
	_, _ = db.ExecContext(ctx, `
		INSERT INTO training_environment.environment_type (id, code, name, description)
		VALUES ($1, 'IT-TEST', 'Tipo Test IT', 'Descripcion Test')
		ON CONFLICT (id) DO NOTHING;
	`, typeID)

	repo := persistence.NewPostgresEnvironmentRepository(db)

	loc := "Piso 4, Laboratorio 401"
	envID := "a1b2c3d4-e5f6-7890-abcd-ef1234567890"
	now := time.Now().UTC().Truncate(time.Microsecond)

	entity := &model.Environment{
		ID:                envID,
		EnvironmentTypeID: typeID,
		TrainingCenterID:  "c1d2e3f4-a5b6-7890-1234-56789abcdef0",
		Name:              "Ambiente Integración PostgreSQL",
		Capacity:          35,
		Location:          &loc,
		IsActive:          true,
		CreatedAt:         now,
		UpdatedAt:         now,
	}

	// 1. Test Save
	if err := repo.Save(ctx, entity); err != nil {
		t.Fatalf("failed to save environment in PostgreSQL: %v", err)
	}

	// 2. Test FindByID
	found, err := repo.FindByID(ctx, envID)
	if err != nil {
		t.Fatalf("failed to find environment by ID in PostgreSQL: %v", err)
	}
	if found == nil {
		t.Fatalf("expected to find saved environment, got nil")
	}
	if found.ID != envID || found.Name != entity.Name || found.Capacity != 35 || found.Location == nil || *found.Location != loc {
		t.Fatalf("mismatched saved environment fields: %+v", found)
	}

	// 3. Test FindByID not found
	missing, err := repo.FindByID(ctx, "00000000-0000-0000-0000-000000000000")
	if err != nil {
		t.Fatalf("unexpected error on missing ID: %v", err)
	}
	if missing != nil {
		t.Fatalf("expected nil for missing ID, got: %+v", missing)
	}

	// 4. Test FindAll
	all, err := repo.FindAll(ctx)
	if err != nil {
		t.Fatalf("failed to list environments from PostgreSQL: %v", err)
	}
	if len(all) == 0 {
		t.Fatalf("expected at least 1 environment in FindAll, got 0")
	}

	// 5. Test FK Violation Error Mapping
	invalidFKEntity := &model.Environment{
		ID:                "b2c3d4e5-f6a7-8901-bcde-f12345678901",
		EnvironmentTypeID: "ffffffff-ffff-ffff-ffff-ffffffffffff", // Non-existent FK
		TrainingCenterID:  "c1d2e3f4-a5b6-7890-1234-56789abcdef0",
		Name:              "Ambiente con FK Invalida",
		Capacity:          20,
		IsActive:          true,
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	fkErr := repo.Save(ctx, invalidFKEntity)
	if fkErr == nil {
		t.Fatalf("expected FK violation error, got nil")
	}
	if fkErr != usecase.ErrInvalidEnvironmentType {
		t.Fatalf("expected ErrInvalidEnvironmentType, got: %v", fkErr)
	}
}
