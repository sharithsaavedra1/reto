package model_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/code-sena/design-software-training-environment-service/internal/domain/model"
)

func validReservation() model.Reservation {
	return model.Reservation{
		ID:              "7d536ffd-8ee3-4882-865d-4926ee17737b",
		EnvironmentID:   "6f674be8-24a4-4759-9226-9b9574ba9643",
		RequesterID:     "d8231174-567b-40b0-8ab5-4af992406cf6",
		ReservationDate: time.Date(2026, time.September, 20, 0, 0, 0, 0, time.UTC),
		StartTime:       "08:00:00",
		EndTime:         "10:00:00",
		Status:          model.ReservationStatusPending,
		CreatedAt:       time.Now().UTC(),
		UpdatedAt:       time.Now().UTC(),
	}
}

func TestReservationValidate_HU007(t *testing.T) {
	t.Run("valid reservation", func(t *testing.T) {
		item := validReservation()
		if err := item.Validate(); err != nil {
			t.Fatalf("expected valid reservation, got %v", err)
		}
	})

	tests := []struct {
		name   string
		mutate func(*model.Reservation)
		want   error
	}{
		{"invalid environment id", func(r *model.Reservation) { r.EnvironmentID = "bad-id" }, model.ErrInvalidReservationEnvironmentID},
		{"invalid requester id", func(r *model.Reservation) { r.RequesterID = "user-123" }, model.ErrInvalidReservationRequesterID},
		{"missing date", func(r *model.Reservation) { r.ReservationDate = time.Time{} }, model.ErrInvalidReservationDate},
		{"invalid start", func(r *model.Reservation) { r.StartTime = "25:00" }, model.ErrInvalidReservationStartTime},
		{"invalid end", func(r *model.Reservation) { r.EndTime = "x" }, model.ErrInvalidReservationEndTime},
		{"equal times", func(r *model.Reservation) { r.EndTime = r.StartTime }, model.ErrInvalidReservationTimeRange},
		{"reversed times", func(r *model.Reservation) { r.StartTime, r.EndTime = "11:00", "10:00" }, model.ErrInvalidReservationTimeRange},
		{"reason too long", func(r *model.Reservation) { value := strings.Repeat("x", 256); r.Reason = &value }, model.ErrReservationReasonTooLong},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			item := validReservation()
			tc.mutate(&item)
			if err := item.Validate(); !errors.Is(err, tc.want) {
				t.Fatalf("expected %v, got %v", tc.want, err)
			}
		})
	}
}

func TestNormalizeReservationClock_HU007(t *testing.T) {
	for input, expected := range map[string]string{"08:15": "08:15:00", "08:15:30": "08:15:30"} {
		actual, err := model.NormalizeReservationClock(input)
		if err != nil || actual != expected {
			t.Fatalf("input %s expected %s, got %s err=%v", input, expected, actual, err)
		}
	}
}
