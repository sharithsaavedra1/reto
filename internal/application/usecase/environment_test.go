package usecase_test

import (
	"context"
	"testing"

	"github.com/code-sena/design-software-training-environment-service/internal/adapter/out/persistence"
	"github.com/code-sena/design-software-training-environment-service/internal/application/port"
	"github.com/code-sena/design-software-training-environment-service/internal/application/usecase"
)

func TestEnvironmentUseCase_Create(t *testing.T) {
	envRepo := persistence.NewMemoryEnvironmentRepository()
	envTypeRepo := persistence.NewMemoryEnvironmentTypeRepository()
	uc := usecase.NewEnvironmentUseCase(envRepo, envTypeRepo)

	loc := "Torre A, Piso 3"
	cmd := port.CreateEnvironmentCommand{
		EnvironmentTypeID: "347d47f9-1ba5-4b9c-89b7-892112a23d90", // Aula seed
		TrainingCenterID:  "c1d2e3f4-a5b6-7890-1234-56789abcdef0",
		Name:              "Aula 301 - Sistemas",
		Capacity:          35,
		Location:          &loc,
	}

	created, err := uc.Create(context.Background(), cmd)
	if err != nil {
		t.Fatalf("expected successful creation, got: %v", err)
	}

	if created.ID == "" || !created.IsActive || created.CreatedAt.IsZero() || created.UpdatedAt.IsZero() {
		t.Fatalf("unexpected created environment fields: %+v", created)
	}
	if created.Name != "Aula 301 - Sistemas" || created.Capacity != 35 {
		t.Fatalf("mismatched values in created environment: %+v", created)
	}

	// E3: Invalid environment_type_id must fail with ErrInvalidEnvironmentType
	invalidTypeCmd := cmd
	invalidTypeCmd.EnvironmentTypeID = "99999999-9999-9999-9999-999999999999"
	_, err = uc.Create(context.Background(), invalidTypeCmd)
	if err != usecase.ErrInvalidEnvironmentType {
		t.Fatalf("expected ErrInvalidEnvironmentType for non-existent type, got: %v", err)
	}
}

func TestEnvironmentUseCase_GetByID(t *testing.T) {
	envRepo := persistence.NewMemoryEnvironmentRepository()
	envTypeRepo := persistence.NewMemoryEnvironmentTypeRepository()
	uc := usecase.NewEnvironmentUseCase(envRepo, envTypeRepo)

	cmd := port.CreateEnvironmentCommand{
		EnvironmentTypeID: "9518271b-529f-4942-b7d3-1271827cc812", // Lab seed
		TrainingCenterID:  "c1d2e3f4-a5b6-7890-1234-56789abcdef0",
		Name:              "Laboratorio de Electrónica",
		Capacity:          20,
	}
	created, _ := uc.Create(context.Background(), cmd)

	// Existing
	found, err := uc.GetByID(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("expected to find environment, got: %v", err)
	}
	if found.ID != created.ID || found.Name != "Laboratorio de Electrónica" {
		t.Fatalf("unexpected found item: %+v", found)
	}

	// Unknown
	_, err = uc.GetByID(context.Background(), "00000000-0000-0000-0000-000000000000")
	if err != usecase.ErrEnvironmentNotFound {
		t.Fatalf("expected ErrEnvironmentNotFound, got: %v", err)
	}
}
