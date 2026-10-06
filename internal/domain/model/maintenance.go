package model

import (
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

const MaxMaintenanceReasonLength = 255

var (
	ErrInvalidMaintenanceEnvironmentID = errors.New("environment_id must be a valid RFC 4122 UUID")
	ErrInvalidMaintenanceID            = errors.New("maintenance_id must be a valid RFC 4122 UUID")
	ErrInvalidMaintenanceCreatedBy     = errors.New("created_by must be a valid RFC 4122 UUID")
	ErrInvalidMaintenanceDateRange     = errors.New("end_date must be on or after start_date")
	ErrInvalidMaintenanceStartTime     = errors.New("start_time must be a valid time (HH:MM or HH:MM:SS)")
	ErrInvalidMaintenanceEndTime       = errors.New("end_time must be a valid time (HH:MM or HH:MM:SS)")
	ErrInvalidMaintenanceTimeRange     = errors.New("for same-day maintenance end_time must be after start_time")
	ErrMaintenanceReasonTooLong        = errors.New("reason must be at most 255 characters")
)

// Maintenance represents an append-only period during which an environment is unavailable.
// Dates define the calendar range and times define the exact temporal boundaries.
type Maintenance struct {
	ID            string    `json:"id"`
	EnvironmentID string    `json:"environment_id"`
	StartDate     time.Time `json:"start_date"`
	EndDate       time.Time `json:"end_date"`
	StartTime     string    `json:"start_time"`
	EndTime       string    `json:"end_time"`
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
	if m.StartDate.IsZero() || m.EndDate.IsZero() || maintenanceDateOnly(m.EndDate).Before(maintenanceDateOnly(m.StartDate)) {
		return ErrInvalidMaintenanceDateRange
	}

	start, err := ParseMaintenanceClock(m.StartTime)
	if err != nil {
		return ErrInvalidMaintenanceStartTime
	}
	end, err := ParseMaintenanceClock(m.EndTime)
	if err != nil {
		return ErrInvalidMaintenanceEndTime
	}
	if sameMaintenanceDate(m.StartDate, m.EndDate) && end <= start {
		return ErrInvalidMaintenanceTimeRange
	}
	if m.Reason != nil && utf8.RuneCountInString(strings.TrimSpace(*m.Reason)) > MaxMaintenanceReasonLength {
		return ErrMaintenanceReasonTooLong
	}
	return nil
}

// ParseMaintenanceClock validates HH:MM or HH:MM:SS and returns the offset from midnight.
func ParseMaintenanceClock(value string) (time.Duration, error) {
	value = strings.TrimSpace(value)
	for _, layout := range []string{"15:04:05", "15:04"} {
		parsed, err := time.Parse(layout, value)
		if err == nil {
			return time.Duration(parsed.Hour())*time.Hour +
				time.Duration(parsed.Minute())*time.Minute +
				time.Duration(parsed.Second())*time.Second, nil
		}
	}
	return 0, fmt.Errorf("invalid clock value %q", value)
}

// NormalizeMaintenanceClock returns a canonical HH:MM:SS representation.
func NormalizeMaintenanceClock(value string) (string, error) {
	d, err := ParseMaintenanceClock(value)
	if err != nil {
		return "", err
	}
	hours := int(d / time.Hour)
	minutes := int((d % time.Hour) / time.Minute)
	seconds := int((d % time.Minute) / time.Second)
	return fmt.Sprintf("%02d:%02d:%02d", hours, minutes, seconds), nil
}

// MaintenanceBounds returns absolute UTC boundaries for overlap calculations.
func MaintenanceBounds(m Maintenance) (time.Time, time.Time, error) {
	startClock, err := ParseMaintenanceClock(m.StartTime)
	if err != nil {
		return time.Time{}, time.Time{}, ErrInvalidMaintenanceStartTime
	}
	endClock, err := ParseMaintenanceClock(m.EndTime)
	if err != nil {
		return time.Time{}, time.Time{}, ErrInvalidMaintenanceEndTime
	}
	startDate := maintenanceDateOnly(m.StartDate)
	endDate := maintenanceDateOnly(m.EndDate)
	return startDate.Add(startClock), endDate.Add(endClock), nil
}

func sameMaintenanceDate(a, b time.Time) bool {
	return maintenanceDateOnly(a).Equal(maintenanceDateOnly(b))
}

func maintenanceDateOnly(value time.Time) time.Time {
	value = value.UTC()
	return time.Date(value.Year(), value.Month(), value.Day(), 0, 0, 0, 0, time.UTC)
}
