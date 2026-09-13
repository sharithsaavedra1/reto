package persistence

import (
	"context"
	"testing"
	"time"

	"github.com/code-sena/design-software-training-environment-service/internal/application/port"
	"github.com/code-sena/design-software-training-environment-service/internal/domain/model"
)

func TestMemoryEnvironmentRepository_EffectiveUntilIsInclusive(t *testing.T) {
	effectiveUntil := time.Date(2026, time.September, 15, 0, 0, 0, 0, time.UTC)
	repo := &MemoryEnvironmentRepository{
		items: map[string]model.Environment{
			"env-1": {ID: "env-1", IsActive: true},
		},
		rules: []memoryAvailabilityRule{
			{
				environmentID:  "env-1",
				dayOfWeek:      2,
				start:          8 * time.Hour,
				end:            18 * time.Hour,
				effectiveFrom:  time.Date(2020, time.January, 1, 0, 0, 0, 0, time.UTC),
				effectiveUntil: &effectiveUntil,
			},
		},
	}

	date := effectiveUntil
	start := time.Date(0, time.January, 1, 9, 0, 0, 0, time.UTC)
	end := time.Date(0, time.January, 1, 10, 0, 0, 0, time.UTC)
	page, err := repo.Query(context.Background(), port.EnvironmentListQuery{
		Page:           1,
		PageSize:       10,
		AvailableDate:  &date,
		AvailableStart: &start,
		AvailableEnd:   &end,
	})
	if err != nil {
		t.Fatalf("unexpected query error: %v", err)
	}
	if page.TotalItems != 1 || len(page.Items) != 1 || page.Items[0].ID != "env-1" {
		t.Fatalf("expected the rule to apply on effective_until, got: %+v", page)
	}
}
