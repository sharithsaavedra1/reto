//go:build performance

package persistence_test

import (
	"context"
	"math"
	"os"
	"sort"
	"strconv"
	"testing"
	"time"

	"github.com/code-sena/design-software-training-environment-service/internal/adapter/out/persistence"
	"github.com/code-sena/design-software-training-environment-service/internal/application/port"
)

func TestPostgresEnvironmentRepository_AvailabilityP95(t *testing.T) {
	db := getRequiredTestPostgresDB(t)
	defer db.Close()

	ctx := context.Background()
	var populatedEnvironments int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM training_environment.environment").Scan(&populatedEnvironments); err != nil {
		t.Fatalf("failed to verify populated PostgreSQL environments: %v", err)
	}
	var populatedRules int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM training_environment.availability_rule").Scan(&populatedRules); err != nil {
		t.Fatalf("failed to verify populated PostgreSQL availability rules: %v", err)
	}
	if populatedEnvironments == 0 || populatedRules == 0 {
		t.Fatal("the PostgreSQL performance measurement requires populated environment and availability_rule tables")
	}

	samples := performanceSampleCount(t)
	date := performanceDate(t)
	start := time.Date(0, time.January, 1, 9, 0, 0, 0, time.UTC)
	end := time.Date(0, time.January, 1, 10, 0, 0, 0, time.UTC)
	repo := persistence.NewPostgresEnvironmentRepository(db)
	durations := make([]time.Duration, 0, samples)

	for i := 0; i < samples; i++ {
		started := time.Now()
		if _, err := repo.Query(ctx, port.EnvironmentListQuery{
			Page:           1,
			PageSize:       20,
			SortField:      "id",
			AvailableDate:  &date,
			AvailableStart: &start,
			AvailableEnd:   &end,
		}); err != nil {
			t.Fatalf("availability query failed during performance measurement: %v", err)
		}
		durations = append(durations, time.Since(started))
	}

	sort.Slice(durations, func(i, j int) bool { return durations[i] < durations[j] })
	p95Index := int(math.Ceil(float64(len(durations))*0.95)) - 1
	p95 := durations[p95Index]
	t.Logf("availability query p95=%s samples=%d populated_environments=%d populated_rules=%d date=%s", p95, samples, populatedEnvironments, populatedRules, date.Format(time.DateOnly))
	if p95 >= 300*time.Millisecond {
		t.Fatalf("availability query p95 %s does not meet the <300ms criterion", p95)
	}
}

func performanceSampleCount(t *testing.T) int {
	value := os.Getenv("PERF_SAMPLES")
	if value == "" {
		return 100
	}
	samples, err := strconv.Atoi(value)
	if err != nil || samples < 20 {
		t.Fatalf("PERF_SAMPLES must be an integer greater than or equal to 20")
	}
	return samples
}

func performanceDate(t *testing.T) time.Time {
	value := os.Getenv("PERF_DATE")
	if value == "" {
		value = time.Now().UTC().Format(time.DateOnly)
	}
	date, err := time.Parse(time.DateOnly, value)
	if err != nil {
		t.Fatalf("PERF_DATE must use YYYY-MM-DD: %v", err)
	}
	return date
}
