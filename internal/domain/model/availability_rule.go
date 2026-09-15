package model

import (
	"errors"
	"time"
)

const MaxAvailabilityRules = 24

var (
	ErrInvalidAvailabilityDay       = errors.New("day_of_week must be between 1 and 7")
	ErrInvalidAvailabilityTimeRange = errors.New("end_time must be after start_time")
	ErrInvalidAvailabilityDateRange = errors.New("effective_until must be on or after effective_from")
)

// AvailabilityRule represents a weekly recurring availability window.
type AvailabilityRule struct {
	ID             string     `json:"id"`
	EnvironmentID  string     `json:"environment_id"`
	DayOfWeek      int        `json:"day_of_week"`
	StartTime      time.Time  `json:"start_time"`
	EndTime        time.Time  `json:"end_time"`
	EffectiveFrom  time.Time  `json:"effective_from"`
	EffectiveUntil *time.Time `json:"effective_until,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
}

// Validate applies the application-level rules that complement PostgreSQL checks.
func (r *AvailabilityRule) Validate() error {
	if !IsValidUUID(r.EnvironmentID) {
		return ErrInvalidTrainingCenterID
	}
	if r.DayOfWeek < 1 || r.DayOfWeek > 7 {
		return ErrInvalidAvailabilityDay
	}
	if availabilityClock(r.EndTime) <= availabilityClock(r.StartTime) {
		return ErrInvalidAvailabilityTimeRange
	}
	if r.EffectiveFrom.IsZero() {
		return ErrInvalidAvailabilityDateRange
	}
	if r.EffectiveUntil != nil && dateOnly(r.EffectiveUntil).Before(dateOnly(&r.EffectiveFrom)) {
		return ErrInvalidAvailabilityDateRange
	}
	return nil
}

func availabilityClock(value time.Time) time.Duration {
	return time.Duration(value.Hour())*time.Hour +
		time.Duration(value.Minute())*time.Minute +
		time.Duration(value.Second())*time.Second +
		time.Duration(value.Nanosecond())
}

func dateOnly(value *time.Time) time.Time {
	return time.Date(value.UTC().Year(), value.UTC().Month(), value.UTC().Day(), 0, 0, 0, 0, time.UTC)
}
