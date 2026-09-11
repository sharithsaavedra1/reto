package model

import (
	"errors"
	"strings"
	"time"
	"unicode/utf8"
)

var (
	ErrInvalidEnvironmentTypeID = errors.New("environment_type_id must be a valid UUID")
	ErrInvalidTrainingCenterID  = errors.New("training_center_id must be a valid UUID")
	ErrEmptyEnvironmentName     = errors.New("name cannot be empty")
	ErrEnvironmentNameTooLong   = errors.New("name must be at most 100 characters")
	ErrInvalidCapacity          = errors.New("capacity must be a positive integer greater than 0")
	ErrLocationTooLong          = errors.New("location must be at most 200 characters")
)

// Environment represents a physical training environment (classroom, lab, workshop, etc.).
type Environment struct {
	ID                 string    `json:"id"`
	EnvironmentTypeID  string    `json:"environment_type_id"`
	TrainingCenterID   string    `json:"training_center_id"`
	Name               string    `json:"name"`
	Capacity           int       `json:"capacity"`
	Location           *string   `json:"location,omitempty"`
	IsActive           bool      `json:"is_active"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

// Validate verifies domain rules for the Environment entity according to the contract and business constraints.
func (e *Environment) Validate() error {
	typeID := strings.TrimSpace(e.EnvironmentTypeID)
	if typeID == "" || len(typeID) < 32 {
		return ErrInvalidEnvironmentTypeID
	}

	centerID := strings.TrimSpace(e.TrainingCenterID)
	if centerID == "" || len(centerID) < 32 {
		return ErrInvalidTrainingCenterID
	}

	name := strings.TrimSpace(e.Name)
	if name == "" {
		return ErrEmptyEnvironmentName
	}
	if utf8.RuneCountInString(name) > 100 {
		return ErrEnvironmentNameTooLong
	}

	if e.Capacity <= 0 {
		return ErrInvalidCapacity
	}

	if e.Location != nil && utf8.RuneCountInString(*e.Location) > 200 {
		return ErrLocationTooLong
	}

	return nil
}
