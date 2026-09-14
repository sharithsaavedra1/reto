package model

import (
	"errors"
	"strings"
	"time"
	"unicode/utf8"
)

const MaxLocationLength = 200

var (
	ErrInvalidEnvironmentTypeID = errors.New("environment_type_id must be a valid RFC 4122 UUID")
	ErrInvalidTrainingCenterID  = errors.New("training_center_id must be a valid RFC 4122 UUID")
	ErrEmptyEnvironmentName     = errors.New("name cannot be empty")
	ErrEnvironmentNameTooLong   = errors.New("name must be at most 100 characters")
	ErrInvalidCapacity          = errors.New("capacity must be a positive integer greater than 0")
	ErrLocationTooLong          = errors.New("location must be at most 200 characters")
)

// IsValidUUID checks if the string is a valid RFC 4122 UUID (8-4-4-4-12 hexadecimal string).
// Rejects malformed strings (such as 36 letters of 'x' or non-hex characters).
func IsValidUUID(s string) bool {
	s = strings.TrimSpace(s)
	if len(s) != 36 {
		return false
	}
	for i := 0; i < 36; i++ {
		c := s[i]
		switch i {
		case 8, 13, 18, 23:
			if c != '-' {
				return false
			}
		default:
			if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
				return false
			}
		}
	}
	return true
}

// Environment represents a physical training environment (classroom, lab, workshop, etc.).
// Note on TrainingCenterID: Follows ADR-002 (docs/adr/ADR-002-training-center-validation-strategy.md)
// for synchronous RFC 4122 syntax validation at ingestion and deferred/asynchronous existence validation across bounded contexts.
type Environment struct {
	ID                string    `json:"id"`
	EnvironmentTypeID string    `json:"environment_type_id"`
	TrainingCenterID  string    `json:"training_center_id"`
	Name              string    `json:"name"`
	Capacity          int       `json:"capacity"`
	Location          *string   `json:"location,omitempty"`
	IsActive          bool      `json:"is_active"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

// Validate verifies domain rules for the Environment entity according to the contract and business constraints.
func (e *Environment) Validate() error {
	if !IsValidUUID(e.EnvironmentTypeID) {
		return ErrInvalidEnvironmentTypeID
	}

	if !IsValidUUID(e.TrainingCenterID) {
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

	if e.Location != nil && utf8.RuneCountInString(*e.Location) > MaxLocationLength {
		return ErrLocationTooLong
	}

	return nil
}
