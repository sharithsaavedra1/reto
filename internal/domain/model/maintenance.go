package model

import (
	"errors"
	"strings"
	"time"
	"unicode/utf8"
)

const MaxMaintenanceReasonLength = 255

var (
	ErrInvalidMaintenanceEnvironmentID = errors.New("environment_id must be a valid RFC 4122 UUID")
	ErrInvalidMaintenanceID            = errors.New("maintenance_id must be a valid RFC 4122 UUID")
	ErrInvalidMaintenanceCreatedBy     = errors.New("created_by must be a valid RFC 4122 UUID")
	ErrInvalidMaintenanceDateRange     = errors.New("end_date must be after start_date")
	ErrMaintenanceReasonTooLong        = errors.New("reason must be at most 255 characters")
)

// Maintenance represents an append-only period during which an environment is unavailable.
type Maintenance struct {
	ID            string    `json:"id"`
	EnvironmentID string    `json:"environment_id"`
	StartDate     time.Time `json:"start_date"`
	EndDate       time.Time `json:"end_date"`
	Reason        *string   `json:"reason,omitempty"`
	CreatedBy     string    `json:"created_by"`
	CreatedAt     time.Time `json:"created_at"`
}

func (m *Maintenance) Validate() error {
	if !IsValidUUID(m.EnvironmentID) {
		return ErrInvalidMaintenanceEnvironmentID
	}
	if !IsValidUUID(m.CreatedBy) {
		return ErrInvalidMaintenanceCreatedBy
	}
	if m.StartDate.IsZero() || m.EndDate.IsZero() || !m.EndDate.After(m.StartDate) {
		return ErrInvalidMaintenanceDateRange
	}
	if m.Reason != nil && utf8.RuneCountInString(strings.TrimSpace(*m.Reason)) > MaxMaintenanceReasonLength {
		return ErrMaintenanceReasonTooLong
	}
	return nil
}
