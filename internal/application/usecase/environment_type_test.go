package usecase_test

import (
	"context"
	"testing"

	"github.com/code-sena/design-software-training-environment-service/internal/adapter/out/persistence"
	"github.com/code-sena/design-software-training-environment-service/internal/application/port"
	"github.com/code-sena/design-software-training-environment-service/internal/application/usecase"
)

func TestEnvironmentTypeUseCase_List(t *testing.T) {
	repo := persistence.NewMemoryEnvironmentTypeRepository()
	uc := usecase.NewEnvironmentTypeUseCase(repo)

	items, err := uc.List(context.Background())
	if err != nil {
		t.Fatalf("unexpected error listing environment types: %v", err)
	}

	if len(items) != 3 {
		t.Fatalf("expected 3 seed items, got %d", len(items))
	}
}

func TestEnvironmentTypeUseCase_GetByID(t *testing.T) {
	repo := persistence.NewMemoryEnvironmentTypeRepository()
	uc := usecase.NewEnvironmentTypeUseCase(repo)

	// Existing seed
	item, err := uc.GetByID(context.Background(), "347d47f9-1ba5-4b9c-89b7-892112a23d90")
	if err != nil {
		t.Fatalf("expected to find seed item, got: %v", err)
	}
	if item.Code != "CLASSROOM" || item.Name != "Aula" {
		t.Fatalf("unexpected item content: %+v", item)
	}

	// Non-existing
	_, err = uc.GetByID(context.Background(), "00000000-0000-0000-0000-000000000000")
	if err != usecase.ErrNotFound {
		t.Fatalf("expected ErrNotFound for unknown ID, got: %v", err)
	}
}

func TestEnvironmentTypeUseCase_Create(t *testing.T) {
	repo := persistence.NewMemoryEnvironmentTypeRepository()
	uc := usecase.NewEnvironmentTypeUseCase(repo)

	desc := "Auditorio principal de eventos"
	cmd := port.CreateEnvironmentTypeCommand{
		Code:        "AUDITORIUM",
		Name:        "Auditorio",
		Description: &desc,
	}

	created, err := uc.Create(context.Background(), cmd)
	if err != nil {
		t.Fatalf("expected successful creation, got: %v", err)
	}

	if created.ID == "" || created.Code != "AUDITORIUM" || created.Name != "Auditorio" {
		t.Fatalf("unexpected created entity: %+v", created)
	}

	// Duplicate Name conflict
	_, err = uc.Create(context.Background(), port.CreateEnvironmentTypeCommand{
		Code: "AUD-2",
		Name: "Auditorio",
	})
	if err != usecase.ErrConflictName {
		t.Fatalf("expected ErrConflictName, got: %v", err)
	}

	// Duplicate Code conflict
	_, err = uc.Create(context.Background(), port.CreateEnvironmentTypeCommand{
		Code: "AUDITORIUM",
		Name: "Auditorio 2",
	})
	if err != usecase.ErrConflictCode {
		t.Fatalf("expected ErrConflictCode, got: %v", err)
	}

	// Empty Code validation
	_, err = uc.Create(context.Background(), port.CreateEnvironmentTypeCommand{
		Code: "   ",
		Name: "Auditorio 3",
	})
	if err == nil {
		t.Fatalf("expected error for empty code, got nil")
	}
}
