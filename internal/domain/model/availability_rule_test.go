package model_test

import (
	"testing"
	"time"

	"github.com/code-sena/design-software-training-environment-service/internal/domain/model"
)

func TestAvailabilityRule_Validate(t *testing.T) {
	rule := validAvailabilityRule()
	if err := rule.Validate(); err != nil {
		t.Fatalf("expected valid availability rule, got %v", err)
	}

	for _, day := range []int{0, 8} {
		invalid := rule
		invalid.DayOfWeek = day
		if err := invalid.Validate(); err != model.ErrInvalidAvailabilityDay {
			t.Errorf("day %d: expected ErrInvalidAvailabilityDay, got %v", day, err)
		}
	}

	invalidTime := rule
	invalidTime.EndTime = invalidTime.StartTime
	if err := invalidTime.Validate(); err != model.ErrInvalidAvailabilityTimeRange {
		t.Fatalf("expected ErrInvalidAvailabilityTimeRange, got %v", err)
	}

	invalidDates := rule
	until := time.Date(2025, time.December, 31, 0, 0, 0, 0, time.UTC)
	invalidDates.EffectiveUntil = &until
	if err := invalidDates.Validate(); err != model.ErrInvalidAvailabilityDateRange {
		t.Fatalf("expected ErrInvalidAvailabilityDateRange, got %v", err)
	}
}

func validAvailabilityRule() model.AvailabilityRule {
	return model.AvailabilityRule{
		ID: "a1b2c3d4-e5f6-4789-9012-3456789abcde", EnvironmentID: "78901234-5678-9012-3456-789012345678",
		DayOfWeek: 1, StartTime: time.Date(0, time.January, 1, 8, 0, 0, 0, time.UTC),
		EndTime:       time.Date(0, time.January, 1, 12, 0, 0, 0, time.UTC),
		EffectiveFrom: time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC),
	}
}
