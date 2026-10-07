package model_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/code-sena/design-software-training-environment-service/internal/domain/model"
)

func TestMaintenanceValidate_HU018_TemporalRules(t *testing.T) {
	base := model.Maintenance{
		EnvironmentID: "78901234-5678-9012-3456-789012345678",
		StartDate:     time.Date(2026, time.June, 1, 0, 0, 0, 0, time.UTC),
		EndDate:       time.Date(2026, time.June, 2, 0, 0, 0, 0, time.UTC),
		StartTime:     "22:00",
		EndTime:       "06:00",
		CreatedBy:     "42c78701-2b34-4a6f-b034-3f7a3eca57d8",
	}
	if err := base.Validate(); err != nil {
		t.Fatalf("multi-day maintenance with earlier end clock must be valid: %v", err)
	}

	sameDay := base
	sameDay.EndDate = sameDay.StartDate
	sameDay.StartTime = "08:00"
	sameDay.EndTime = "10:00"
	if err := sameDay.Validate(); err != nil {
		t.Fatalf("same-day maintenance with end_time after start_time must be valid: %v", err)
	}

	tests := []struct {
		name string
		item model.Maintenance
		want error
	}{
		{
			name: "invalid environment",
			item: func() model.Maintenance {
				item := base
				item.EnvironmentID = "invalid"
				return item
			}(),
			want: model.ErrInvalidMaintenanceEnvironmentID,
		},
		{
			name: "invalid creator",
			item: func() model.Maintenance {
				item := base
				item.CreatedBy = "user-123"
				return item
			}(),
			want: model.ErrInvalidMaintenanceCreatedBy,
		},
		{
			name: "end date before start date",
			item: func() model.Maintenance {
				item := base
				item.StartDate = time.Date(2026, time.June, 3, 0, 0, 0, 0, time.UTC)
				item.EndDate = time.Date(2026, time.June, 2, 0, 0, 0, 0, time.UTC)
				return item
			}(),
			want: model.ErrInvalidMaintenanceDateRange,
		},
		{
			name: "same day equal clocks",
			item: func() model.Maintenance {
				item := sameDay
				item.EndTime = item.StartTime
				return item
			}(),
			want: model.ErrInvalidMaintenanceTimeRange,
		},
		{
			name: "same day end clock before start clock",
			item: func() model.Maintenance {
				item := sameDay
				item.StartTime = "10:00"
				item.EndTime = "09:59:59"
				return item
			}(),
			want: model.ErrInvalidMaintenanceTimeRange,
		},
		{
			name: "invalid start time",
			item: func() model.Maintenance {
				item := base
				item.StartTime = "25:00"
				return item
			}(),
			want: model.ErrInvalidMaintenanceStartTime,
		},
		{
			name: "invalid end time",
			item: func() model.Maintenance {
				item := base
				item.EndTime = "not-a-time"
				return item
			}(),
			want: model.ErrInvalidMaintenanceEndTime,
		},
		{
			name: "reason too long",
			item: func() model.Maintenance {
				item := base
				item.Reason = stringPointer(strings.Repeat("r", 256))
				return item
			}(),
			want: model.ErrMaintenanceReasonTooLong,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			if err := testCase.item.Validate(); !errors.Is(err, testCase.want) {
				t.Fatalf("expected %v, got %v", testCase.want, err)
			}
		})
	}
}

func TestNormalizeMaintenanceClock(t *testing.T) {
	for input, expected := range map[string]string{
		"08:05":    "08:05:00",
		"18:30:45": "18:30:45",
	} {
		actual, err := model.NormalizeMaintenanceClock(input)
		if err != nil {
			t.Fatalf("normalize %q: %v", input, err)
		}
		if actual != expected {
			t.Fatalf("normalize %q: expected %q, got %q", input, expected, actual)
		}
	}
}

func stringPointer(value string) *string { return &value }
