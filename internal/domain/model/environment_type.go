package model

import (
	"errors"
	"strings"
	"time"
)

var (
	ErrEmptyName       = errors.New("name cannot be empty")
	ErrNameTooLong     = errors.New("name must be at most 100 characters")
	ErrCodeTooLong     = errors.New("code must be at most 20 characters")
	ErrDescTooLong     = errors.New("description must be at most 255 characters")
)

// EnvironmentType represents the domain entity for training environment categories.
type EnvironmentType struct {
	ID          string    `json:"id"`
	Code        string    `json:"code"`
	Name        string    `json:"name"`
	Description *string   `json:"description,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

// Validate checks business rules for the EnvironmentType entity according to the contract.
func (e *EnvironmentType) Validate() error {
	name := strings.TrimSpace(e.Name)
	if name == "" {
		return ErrEmptyName
	}
	if len(name) > 100 {
		return ErrNameTooLong
	}

	code := strings.TrimSpace(e.Code)
	if len(code) > 20 {
		return ErrCodeTooLong
	}

	if e.Description != nil && len(*e.Description) > 255 {
		return ErrDescTooLong
	}

	return nil
}
