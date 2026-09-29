package persistence_test

import (
	"context"
	"testing"
	"time"

	"github.com/code-sena/design-software-training-environment-service/internal/adapter/out/persistence"
	"github.com/code-sena/design-software-training-environment-service/internal/application/usecase"
	"github.com/code-sena/design-software-training-environment-service/internal/domain/model"
)

func TestPostgresEnvironmentTypeRepository_Integration(t *testing.T) {
	db := getTestPostgresDB(t)
	if db == nil {
		return
	}
	defer db.Close()

	ctx := context.Background()

	// Step 1: Initialize table structure with nullable code (state after remediation-training-960)
	_, err := db.ExecContext(ctx, `
		CREATE SCHEMA IF NOT EXISTS training_environment;
		CREATE TABLE IF NOT EXISTS training_environment.environment_type (
			id VARCHAR(36) PRIMARY KEY,
			code VARCHAR(30),
			name VARCHAR(100) NOT NULL,
			description VARCHAR(255),
			created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
		);
		CREATE UNIQUE INDEX IF NOT EXISTS uq_environment_type_code ON training_environment.environment_type (code);
	`)
	if err != nil {
		t.Skipf("Skipping PostgreSQL integration test: unable to prepare tables: %v", err)
		return
	}

	repo := persistence.NewPostgresEnvironmentTypeRepository(db)

	seedAulaID := "347d47f9-1ba5-4b9c-89b7-892112a23d90"
	seedLabID := "9518271b-529f-4942-b7d3-1271827cc812"
	seedTallerID := "4d3be94b-349d-4209-a052-8f3d7e7ce814"
	typeID := "d4e5f6a7-8901-2345-6789-012345678901"

	cleanup := func() {
		_, _ = db.ExecContext(ctx, `
			DELETE FROM training_environment.environment_type
			WHERE id IN ($1, $2, $3, $4) OR code IN ('LAB-TEST', 'CLASSROOM', 'LAB', 'WORKSHOP')
		`, typeID, seedAulaID, seedLabID, seedTallerID)
		_, _ = db.ExecContext(ctx, `ALTER TABLE training_environment.environment_type ALTER COLUMN code SET NOT NULL;`)
	}
	cleanup()
	t.Cleanup(cleanup)

	// Ensure column allows NULL for the pre-migration test state
	_, _ = db.ExecContext(ctx, `ALTER TABLE training_environment.environment_type ALTER COLUMN code DROP NOT NULL;`)

	// Step 2: Insert initial seed records with code = NULL (pre-migration state)
	_, err = db.ExecContext(ctx, `
		INSERT INTO training_environment.environment_type (id, name, description)
		VALUES 
			($1, 'Aula', 'Espacio de formación teórica.'),
			($2, 'Laboratorio', 'Espacio dotado con equipos especializados.'),
			($3, 'Taller', 'Espacio para prácticas manuales.')
		ON CONFLICT (id) DO UPDATE SET code = NULL;
	`, seedAulaID, seedLabID, seedTallerID)
	if err != nil {
		t.Fatalf("failed to insert initial seed records with NULL code: %v", err)
	}

	// Step 3: Execute HU-ENV-015 backfill migration script
	_, err = db.ExecContext(ctx, `
		UPDATE training_environment.environment_type SET code = 'CLASSROOM' WHERE name = 'Aula' AND (code IS NULL OR code = '');
		UPDATE training_environment.environment_type SET code = 'LAB' WHERE name = 'Laboratorio' AND (code IS NULL OR code = '');
		UPDATE training_environment.environment_type SET code = 'WORKSHOP' WHERE name = 'Taller' AND (code IS NULL OR code = '');
	`)
	if err != nil {
		t.Fatalf("failed to execute backfill migration: %v", err)
	}

	// Step 4: Apply NOT NULL constraint (HU-ENV-015)
	_, err = db.ExecContext(ctx, `
		ALTER TABLE training_environment.environment_type ALTER COLUMN code SET NOT NULL;
	`)
	if err != nil {
		t.Fatalf("failed to alter column code SET NOT NULL: %v", err)
	}

	// Step 5: Verify seed records via FindAll
	seeds, err := repo.FindAll(ctx)
	if err != nil {
		t.Fatalf("failed to list backfilled environment types: %v", err)
	}
	if len(seeds) < 3 {
		t.Fatalf("expected at least 3 seed items, got %d", len(seeds))
	}
	seedCodes := make(map[string]bool)
	for _, s := range seeds {
		if s.Code == "" {
			t.Fatalf("found seed item with empty code after backfill: %+v", s)
		}
		seedCodes[s.Code] = true
	}
	if !seedCodes["CLASSROOM"] || !seedCodes["LAB"] || !seedCodes["WORKSHOP"] {
		t.Fatalf("missing expected backfilled seed codes CLASSROOM, LAB, WORKSHOP. Got: %+v", seedCodes)
	}

	// Step 6: Test Save with new valid entity
	desc := "Descripción tipo de prueba"
	entity := &model.EnvironmentType{
		ID:          typeID,
		Code:        "LAB-TEST",
		Name:        "Laboratorio Test IT",
		Description: &desc,
		CreatedAt:   time.Now().UTC().Truncate(time.Microsecond),
	}
	if err := repo.Save(ctx, entity); err != nil {
		t.Fatalf("failed to save environment type: %v", err)
	}

	// Step 7: Test FindByID, FindByName, FindByCode
	found, err := repo.FindByID(ctx, typeID)
	if err != nil || found == nil || found.Code != "LAB-TEST" || found.Name != "Laboratorio Test IT" {
		t.Fatalf("failed to find by ID or mismatched item: %+v, err: %v", found, err)
	}

	byName, err := repo.FindByName(ctx, "Laboratorio Test IT")
	if err != nil || byName == nil || byName.Code != "LAB-TEST" {
		t.Fatalf("failed to find by name: %+v, err: %v", byName, err)
	}

	byCode, err := repo.FindByCode(ctx, "LAB-TEST")
	if err != nil || byCode == nil || byCode.ID != typeID {
		t.Fatalf("failed to find by code: %+v, err: %v", byCode, err)
	}

	// Step 8: Test Duplicate Code Conflict mapping (E2)
	dupEntity := &model.EnvironmentType{
		ID:        "e5f6a7b8-9012-3456-7890-123456789012",
		Code:      "LAB-TEST", // Duplicate code
		Name:      "Otro Laboratorio",
		CreatedAt: time.Now().UTC(),
	}
	dupErr := repo.Save(ctx, dupEntity)
	if dupErr != usecase.ErrConflictCode && dupErr != usecase.ErrConflictName {
		t.Fatalf("expected ErrConflictCode for duplicate code, got: %v", dupErr)
	}

	// Step 9: Verify NULL code is rejected by PostgreSQL NOT NULL constraint (E3)
	_, nullErr := db.ExecContext(ctx, `
		INSERT INTO training_environment.environment_type (id, code, name)
		VALUES ('00000000-0000-0000-0000-000000000099', NULL, 'Tipo Sin Codigo')
	`)
	if nullErr == nil {
		t.Fatalf("expected PostgreSQL NOT NULL constraint violation for NULL code, got nil")
	}
}
