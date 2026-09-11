package model_test

import (
	"strings"
	"testing"
	"time"

	"github.com/code-sena/design-software-training-environment-service/internal/domain/model"
)

func TestEnvironment_Validate(t *testing.T) {
	loc := "Piso 2, Edificio de Informática"
	valid := model.Environment{
		ID:                "a1b2c3d4-e5f6-7a8b-9c0d-1e2f3a4b5c6d",
		EnvironmentTypeID: "347d47f9-1ba5-4b9c-89b7-892112a23d90",
		TrainingCenterID:  "f8e7d6c5-b4a3-2109-8765-43210fedcba9",
		Name:              "Laboratorio de Redes y Telecomunicaciones",
		Capacity:          30,
		Location:          &loc,
		IsActive:          true,
		CreatedAt:         time.Now(),
		UpdatedAt:         time.Now(),
	}

	if err := valid.Validate(); err != nil {
		t.Fatalf("expected valid entity, got: %v", err)
	}

	// Valid with 100 UTF-8 multibyte characters in name
	valid100Name := valid
	valid100Name.Name = strings.Repeat("ó", 100)
	if err := valid100Name.Validate(); err != nil {
		t.Fatalf("expected valid 100 rune name, got: %v", err)
	}

	// Capacity <= 0 must fail
	zeroCap := valid
	zeroCap.Capacity = 0
	if err := zeroCap.Validate(); err != model.ErrInvalidCapacity {
		t.Fatalf("expected ErrInvalidCapacity for 0, got: %v", err)
	}

	negCap := valid
	negCap.Capacity = -5
	if err := negCap.Validate(); err != model.ErrInvalidCapacity {
		t.Fatalf("expected ErrInvalidCapacity for negative, got: %v", err)
	}

	// Empty name
	emptyName := valid
	emptyName.Name = "   "
	if err := emptyName.Validate(); err != model.ErrEmptyEnvironmentName {
		t.Fatalf("expected ErrEmptyEnvironmentName, got: %v", err)
	}

	// Name > 100 runes
	longName := valid
	longName.Name = strings.Repeat("a", 101)
	if err := longName.Validate(); err != model.ErrEnvironmentNameTooLong {
		t.Fatalf("expected ErrEnvironmentNameTooLong, got: %v", err)
	}

	// Location > 200 runes
	longLoc := valid
	tooLongLoc := strings.Repeat("L", 201)
	longLoc.Location = &tooLongLoc
	if err := longLoc.Validate(); err != model.ErrLocationTooLong {
		t.Fatalf("expected ErrLocationTooLong, got: %v", err)
	}

	// Invalid EnvironmentTypeID
	badTypeID := valid
	badTypeID.EnvironmentTypeID = "invalid-id"
	if err := badTypeID.Validate(); err != model.ErrInvalidEnvironmentTypeID {
		t.Fatalf("expected ErrInvalidEnvironmentTypeID, got: %v", err)
	}

	// Invalid TrainingCenterID
	badCenterID := valid
	badCenterID.TrainingCenterID = ""
	if err := badCenterID.Validate(); err != model.ErrInvalidTrainingCenterID {
		t.Fatalf("expected ErrInvalidTrainingCenterID, got: %v", err)
	}
}
