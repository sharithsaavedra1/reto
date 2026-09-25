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

	_, err := db.ExecContext(ctx, `
		CREATE SCHEMA IF NOT EXISTS training_environment;
		CREATE TABLE IF NOT EXISTS training_environment.environment_type (
			id VARCHAR(36) PRIMARY KEY,
			code VARCHAR(30) UNIQUE NOT NULL,
			name VARCHAR(100) NOT NULL,
			description VARCHAR(255),
			created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
		);
	`)
	if err != nil {
		t.Skipf("Skipping PostgreSQL integration test: unable to prepare tables: %v", err)
		return
	}

	repo := persistence.NewPostgresEnvironmentTypeRepository(db)

	desc := "Descripción tipo de prueba"
	typeID := "d4e5f6a7-8901-2345-6789-012345678901"
	cleanup := func() {
		_, _ = db.ExecContext(ctx, `
			DELETE FROM training_environment.environment_type
			WHERE id = $1 OR code = $2 OR name = $3
		`, typeID, "LAB-TEST", "Laboratorio Test IT")
	}
	cleanup()
	t.Cleanup(cleanup)

	entity := &model.EnvironmentType{
		ID:          typeID,
		Code:        "LAB-TEST",
		Name:        "Laboratorio Test IT",
		Description: &desc,
		CreatedAt:   time.Now().UTC().Truncate(time.Microsecond),
	}

	// 1. Test Save
	if err := repo.Save(ctx, entity); err != nil {
		t.Fatalf("failed to save environment type: %v", err)
	}

	// 2. Test FindByID
	found, err := repo.FindByID(ctx, typeID)
	if err != nil {
		t.Fatalf("failed to find environment type by ID: %v", err)
	}
	if found == nil || found.Code != "LAB-TEST" || found.Name != "Laboratorio Test IT" {
		t.Fatalf("mismatched found environment type: %+v", found)
	}

	// 3. Test FindByName
	byName, err := repo.FindByName(ctx, "Laboratorio Test IT")
	if err != nil || byName == nil {
		t.Fatalf("failed to find environment type by name: %v, found: %+v", err, byName)
	}

	// 4. Test FindByCode
	byCode, err := repo.FindByCode(ctx, "LAB-TEST")
	if err != nil || byCode == nil {
		t.Fatalf("failed to find environment type by code: %v, found: %+v", err, byCode)
	}

	// 5. Test Duplicate Code Conflict mapping (E2)
	dupEntity := &model.EnvironmentType{
		ID:        "e5f6a7b8-9012-3456-7890-123456789012",
		Code:      "LAB-TEST", // Duplicate code
		Name:      "Otro Laboratorio",
		CreatedAt: time.Now().UTC(),
	}
	dupErr := repo.Save(ctx, dupEntity)
	if dupErr != usecase.ErrConflictCode && dupErr != usecase.ErrConflictName {
		t.Fatalf("expected ErrConflictCode or ErrConflictName for duplicate code, got: %v", dupErr)
	}

	// 6. Test FindAll
	items, err := repo.FindAll(ctx)
	if err != nil {
		t.Fatalf("failed to list environment types: %v", err)
	}
	if len(items) == 0 {
		t.Fatalf("expected at least 1 item in FindAll")
	}
	for _, it := range items {
		if it.ID == "" || it.Code == "" || it.Name == "" {
			t.Fatalf("found item with empty mandatory field: %+v", it)
		}
	}
}
