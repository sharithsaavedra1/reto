package model

import (
	"errors"
	"strings"
	"time"
	"unicode/utf8"
)

var (
	ErrEmptyName   = errors.New("name cannot be empty")
	ErrNameTooLong = errors.New("name must be at most 100 characters")
	ErrEmptyCode   = errors.New("code cannot be empty")
	ErrCodeTooLong = errors.New("code must be at most 20 characters")
	ErrDescTooLong = errors.New("description must be at most 255 characters")
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
// Uses utf8.RuneCountInString to count characters rather than raw bytes (supports accented/multibyte characters).
func (e *EnvironmentType) Validate() error {
	name := strings.TrimSpace(e.Name)
	if name == "" {
		return ErrEmptyName
	}
	if utf8.RuneCountInString(name) > 100 {
		return ErrNameTooLong
	}

	code := strings.TrimSpace(e.Code)
	if code == "" {
		return ErrEmptyCode
	}
	if utf8.RuneCountInString(code) > 20 {
		return ErrCodeTooLong
	}

	if e.Description != nil && utf8.RuneCountInString(*e.Description) > 255 {
		return ErrDescTooLong
	}

	return nil
}
