package model_test

import (
	"strings"
	"testing"
	"time"

	"github.com/code-sena/design-software-training-environment-service/internal/domain/model"
)

func TestEnvironmentType_Validate(t *testing.T) {
	desc := "A valid description"
	valid := model.EnvironmentType{
		ID:          "347d47f9-1ba5-4b9c-89b7-892112a23d90",
		Code:        "CLASSROOM",
		Name:        "Aula",
		Description: &desc,
		CreatedAt:   time.Now(),
	}

	if err := valid.Validate(); err != nil {
		t.Fatalf("expected valid entity, got error: %v", err)
	}

	// Empty name
	emptyName := valid
	emptyName.Name = "   "
	if err := emptyName.Validate(); err != model.ErrEmptyName {
		t.Fatalf("expected ErrEmptyName, got: %v", err)
	}

	// Name too long (> 100)
	longName := valid
	longName.Name = strings.Repeat("A", 101)
	if err := longName.Validate(); err != model.ErrNameTooLong {
		t.Fatalf("expected ErrNameTooLong, got: %v", err)
	}

	// Code too long (> 20)
	longCode := valid
	longCode.Code = strings.Repeat("C", 21)
	if err := longCode.Validate(); err != model.ErrCodeTooLong {
		t.Fatalf("expected ErrCodeTooLong, got: %v", err)
	}

	// Description too long (> 255)
	longDesc := valid
	tooLong := strings.Repeat("D", 256)
	longDesc.Description = &tooLong
	if err := longDesc.Validate(); err != model.ErrDescTooLong {
		t.Fatalf("expected ErrDescTooLong, got: %v", err)
	}
}
