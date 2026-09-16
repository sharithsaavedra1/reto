package model

import (
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	ReservationStatusPending   = "PENDING"
	ReservationStatusConfirmed = "CONFIRMED"
	ReservationStatusCancelled = "CANCELLED"
	MaxReservationReasonLength = 255
)

var (
	ErrInvalidReservationID            = errors.New("reservation id must be a valid RFC 4122 UUID")
	ErrInvalidReservationEnvironmentID = errors.New("environment_id must be a valid RFC 4122 UUID")
	ErrInvalidReservationRequesterID   = errors.New("requester_id must be a valid RFC 4122 UUID")
	ErrInvalidReservationDate          = errors.New("reservation_date must be a valid ISO date (YYYY-MM-DD)")
	ErrInvalidReservationStartTime     = errors.New("start_time must be a valid time (HH:MM or HH:MM:SS)")
	ErrInvalidReservationEndTime       = errors.New("end_time must be a valid time (HH:MM or HH:MM:SS)")
	ErrInvalidReservationTimeRange     = errors.New("end_time must be after start_time")
	ErrInvalidReservationStatus        = errors.New("status must be PENDING, CONFIRMED or CANCELLED")
	ErrReservationReasonTooLong        = errors.New("reason must be at most 255 characters")
)

// Reservation represents a one-off booking for a training environment.
type Reservation struct {
	ID              string    `json:"id"`
	EnvironmentID   string    `json:"environment_id"`
	ReservationDate time.Time `json:"reservation_date"`
	StartTime       string    `json:"start_time"`
	EndTime         string    `json:"end_time"`
	Reason          *string   `json:"reason,omitempty"`
	RequesterID     string    `json:"requester_id"`
	Status          string    `json:"status"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

func (r *Reservation) Validate() error {
	if r.ID != "" && !IsValidUUID(r.ID) {
		return ErrInvalidReservationID
	}
	if !IsValidUUID(r.EnvironmentID) {
		return ErrInvalidReservationEnvironmentID
	}
	if !IsValidUUID(r.RequesterID) {
		return ErrInvalidReservationRequesterID
	}
	if r.ReservationDate.IsZero() {
		return ErrInvalidReservationDate
	}

	start, err := ParseReservationClock(r.StartTime)
	if err != nil {
		return ErrInvalidReservationStartTime
	}
	end, err := ParseReservationClock(r.EndTime)
	if err != nil {
		return ErrInvalidReservationEndTime
	}
	if end <= start {
		return ErrInvalidReservationTimeRange
	}

	switch r.Status {
	case ReservationStatusPending, ReservationStatusConfirmed, ReservationStatusCancelled:
	default:
		return ErrInvalidReservationStatus
	}

	if r.Reason != nil && utf8.RuneCountInString(*r.Reason) > MaxReservationReasonLength {
		return ErrReservationReasonTooLong
	}
	return nil
}

// ParseReservationClock validates HH:MM or HH:MM:SS and returns the offset from midnight.
func ParseReservationClock(value string) (time.Duration, error) {
	value = strings.TrimSpace(value)
	var parsed time.Time
	var err error
	for _, layout := range []string{"15:04:05", "15:04"} {
		parsed, err = time.Parse(layout, value)
		if err == nil {
			return time.Duration(parsed.Hour())*time.Hour + time.Duration(parsed.Minute())*time.Minute + time.Duration(parsed.Second())*time.Second, nil
		}
	}
	return 0, fmt.Errorf("invalid clock value %q", value)
}

// NormalizeReservationClock returns a canonical HH:MM:SS representation.
func NormalizeReservationClock(value string) (string, error) {
	d, err := ParseReservationClock(value)
	if err != nil {
		return "", err
	}
	hours := int(d / time.Hour)
	minutes := int((d % time.Hour) / time.Minute)
	seconds := int((d % time.Minute) / time.Second)
	return fmt.Sprintf("%02d:%02d:%02d", hours, minutes, seconds), nil
}
