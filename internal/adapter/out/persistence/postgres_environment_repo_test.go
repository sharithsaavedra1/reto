package persistence_test

import (
	"context"
	"database/sql"
	"os"
	"strings"
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
		skipOrFailPostgres(t, "PostgreSQL integration requires TEST_DATABASE_URL or DATABASE_URL")
		return nil
	}

	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		skipOrFailPostgres(t, "unable to open PostgreSQL integration connection: %v", err)
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		db.Close()
		skipOrFailPostgres(t, "PostgreSQL integration ping failed: %v", err)
		return nil
	}

	return db
}

func skipOrFailPostgres(t *testing.T, format string, args ...interface{}) {
	t.Helper()
	if os.Getenv("REQUIRE_POSTGRES_INTEGRATION") == "1" {
		t.Fatalf(format, args...)
		return
	}
	t.Skipf("Skipping PostgreSQL integration test: "+format, args...)
}

func getRequiredTestPostgresDB(t *testing.T) *sql.DB {
	t.Helper()
	dbURL := os.Getenv("TEST_DATABASE_URL")
	if dbURL == "" {
		dbURL = os.Getenv("DATABASE_URL")
	}
	if dbURL == "" {
		t.Fatal("performance measurement requires TEST_DATABASE_URL or DATABASE_URL")
	}

	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		t.Fatalf("unable to open PostgreSQL performance connection: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		t.Fatalf("PostgreSQL performance connection ping failed: %v", err)
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

	// Ensure schema & table exists for integration testing with the OpenAPI VARCHAR(200) contract.
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
		ALTER TABLE training_environment.environment ADD COLUMN IF NOT EXISTS location VARCHAR(200);
		ALTER TABLE training_environment.environment ADD COLUMN IF NOT EXISTS is_active BOOLEAN NOT NULL DEFAULT TRUE;
		ALTER TABLE training_environment.environment ADD COLUMN IF NOT EXISTS created_at TIMESTAMPTZ NOT NULL DEFAULT NOW();
		ALTER TABLE training_environment.environment ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW();
		DO $$
		BEGIN
			IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'chk_environment_capacity') THEN
				ALTER TABLE training_environment.environment
					ADD CONSTRAINT chk_environment_capacity CHECK (capacity > 0);
			END IF;
			IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'fk_environment_environment_type') THEN
				ALTER TABLE training_environment.environment
					ADD CONSTRAINT fk_environment_environment_type
					FOREIGN KEY (environment_type_id) REFERENCES training_environment.environment_type(id);
			END IF;
		END $$;
	`)
	if err != nil {
		skipOrFailPostgres(t, "unable to prepare PostgreSQL integration tables: %v", err)
		return
	}

	// Insert test environment_type seed
	typeID := "e3f2a1b0-1234-5678-90ab-cdef12345678"
	environmentIDs := []string{
		"a1b2c3d4-e5f6-7890-abcd-ef1234567890",
		"b2c3d4e5-f6a7-8901-bcde-f12345678901",
		"c3d4e5f6-a7b8-9012-cdef-123456789012",
		"d4e5f6a7-b8c9-0123-def1-234567890123",
		"e5f6a7b8-c9d0-1234-ef12-345678901234",
		"f6a7b8c9-d0e1-2345-f012-345678901235",
		"a7b8c9d0-e1f2-3456-0123-456789012346",
		"b8c9d0e1-f2a3-4567-1234-567890123457",
		"c9d0e1f2-a3b4-5678-2345-678901234568",
	}
	cleanup := func() {
		_, _ = db.ExecContext(ctx, `
			DELETE FROM training_environment.environment
			WHERE id IN ($1, $2, $3, $4, $5)
		`, environmentIDs[0], environmentIDs[1], environmentIDs[2], environmentIDs[3], environmentIDs[4])
		_, _ = db.ExecContext(ctx, `
			DELETE FROM training_environment.environment
			WHERE id IN ($1, $2, $3, $4)
		`, environmentIDs[5], environmentIDs[6], environmentIDs[7], environmentIDs[8])
		_, _ = db.ExecContext(ctx, `DELETE FROM training_environment.environment_type WHERE id = $1`, typeID)
	}
	cleanup()
	t.Cleanup(cleanup)

	_, _ = db.ExecContext(ctx, `
		INSERT INTO training_environment.environment_type (id, code, name, description)
		VALUES ($1, 'IT-TEST', 'Tipo Test IT', 'Descripcion Test')
		ON CONFLICT (id) DO NOTHING;
	`, typeID)

	repo := persistence.NewPostgresEnvironmentRepository(db)

	loc := "Piso 4, Laboratorio 401"
	envID := environmentIDs[0]
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
		ID:                environmentIDs[1],
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

	// 6. Test all OpenAPI boundary values and verify PostgreSQL returns the exact value.
	for index, length := range []int{150, 151, 199, 200} {
		location := strings.Repeat("L", length)
		boundaryEntity := &model.Environment{
			ID:                environmentIDs[index+2],
			EnvironmentTypeID: typeID,
			TrainingCenterID:  "c1d2e3f4-a5b6-7890-1234-56789abcdef0",
			Name:              "Ambiente Ubicacion Limite",
			Capacity:          25,
			Location:          &location,
			IsActive:          true,
			CreatedAt:         now,
			UpdatedAt:         now,
		}
		if err := repo.Save(ctx, boundaryEntity); err != nil {
			t.Fatalf("failed to save environment with %d rune location: %v", length, err)
		}
		foundBoundary, err := repo.FindByID(ctx, boundaryEntity.ID)
		if err != nil {
			t.Fatalf("failed to read environment with %d rune location: %v", length, err)
		}
		if foundBoundary == nil || foundBoundary.Location == nil || *foundBoundary.Location != location {
			t.Fatalf("location was truncated or changed for %d runes: got=%v want=%d runes", length, foundBoundary.Location, length)
		}
	}

	// 7. Test location exceeding 200 characters mapped to ErrLocationTooLong (22001)
	locExceeded := strings.Repeat("X", 201)
	tooLongLocEntity := &model.Environment{
		ID:                environmentIDs[6],
		EnvironmentTypeID: typeID,
		TrainingCenterID:  "c1d2e3f4-a5b6-7890-1234-56789abcdef0",
		Name:              "Ambiente Ubicacion Excedida",
		Capacity:          25,
		Location:          &locExceeded,
		IsActive:          true,
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	longErr := repo.Save(ctx, tooLongLocEntity)
	if longErr != model.ErrLocationTooLong {
		t.Fatalf("expected ErrLocationTooLong for > 200 characters, got: %v", longErr)
	}

	// 8. Test CHECK Constraint on capacity <= 0 mapped to ErrInvalidCapacity (23514)
	invalidCapEntity := &model.Environment{
		ID:                environmentIDs[7],
		EnvironmentTypeID: typeID,
		TrainingCenterID:  "c1d2e3f4-a5b6-7890-1234-56789abcdef0",
		Name:              "Ambiente Capacidad Invalida",
		Capacity:          0, // Violates chk_environment_capacity
		IsActive:          true,
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	capErr := repo.Save(ctx, invalidCapEntity)
	if capErr != model.ErrInvalidCapacity {
		t.Fatalf("expected ErrInvalidCapacity for capacity <= 0, got: %v", capErr)
	}
}

func TestPostgresEnvironmentRepository_HU004_UpdateAndLogicalDeactivate(t *testing.T) {
	db := getTestPostgresDB(t)
	if db == nil {
		return
	}
	defer db.Close()

	ctx := context.Background()
	const (
		typeID = "f4e3d2c1-b0a9-4876-9012-3456789abcde"
		envID  = "f4e3d2c1-b0a9-4876-9012-3456789abc01"
	)

	cleanup := func() {
		_, _ = db.ExecContext(ctx, `DELETE FROM training_environment.environment WHERE id = $1`, envID)
		_, _ = db.ExecContext(ctx, `DELETE FROM training_environment.environment_type WHERE id = $1`, typeID)
	}
	cleanup()
	t.Cleanup(cleanup)

	if _, err := db.ExecContext(ctx, `
		INSERT INTO training_environment.environment_type (id, code, name, description)
		VALUES ($1, 'HU004-IT', 'HU004 Integration Type', 'HU004 update test')
	`, typeID); err != nil {
		t.Fatalf("failed to insert HU004 environment type: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO training_environment.environment
		(id, environment_type_id, training_center_id, name, capacity, location, is_active, created_at, updated_at)
		VALUES ($1, $2, 'c1d2e3f4-a5b6-7890-1234-56789abcdef0', 'HU004 Original', 20, 'Piso 1', TRUE, NOW(), NOW())
	`, envID, typeID); err != nil {
		t.Fatalf("failed to insert HU004 environment: %v", err)
	}

	location := strings.Repeat("U", 200)
	entity := &model.Environment{
		ID:                envID,
		EnvironmentTypeID: typeID,
		TrainingCenterID:  "c1d2e3f4-a5b6-7890-1234-56789abcdef0",
		Name:              "HU004 Updated",
		Capacity:          30,
		Location:          &location,
		IsActive:          true,
		CreatedAt:         time.Now().UTC(),
		UpdatedAt:         time.Now().UTC(),
	}

	repo := persistence.NewPostgresEnvironmentRepository(db)
	if err := repo.Update(ctx, entity); err != nil {
		t.Fatalf("failed to update HU004 environment: %v", err)
	}
	found, err := repo.FindByID(ctx, envID)
	if err != nil {
		t.Fatalf("failed to read updated HU004 environment: %v", err)
	}
	if found == nil || found.Name != "HU004 Updated" || found.Capacity != 30 || found.Location == nil || *found.Location != location || !found.IsActive {
		t.Fatalf("unexpected updated HU004 environment: %+v", found)
	}

	if err := repo.Deactivate(ctx, envID); err != nil {
		t.Fatalf("failed to logically deactivate HU004 environment: %v", err)
	}
	if err := repo.Deactivate(ctx, envID); err != nil {
		t.Fatalf("repeated logical deactivation should be idempotent: %v", err)
	}
	found, err = repo.FindByID(ctx, envID)
	if err != nil {
		t.Fatalf("failed to read logically deactivated HU004 environment: %v", err)
	}
	if found == nil || found.IsActive {
		t.Fatalf("expected inactive row to remain in PostgreSQL: %+v", found)
	}
}
