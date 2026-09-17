package persistence_test

import (
	"context"
	"testing"
	"time"

	"github.com/code-sena/design-software-training-environment-service/internal/adapter/out/persistence"
	"github.com/code-sena/design-software-training-environment-service/internal/application/port"
	"github.com/code-sena/design-software-training-environment-service/internal/domain/model"
)

func TestMemoryEnvironmentUtilizationRepository_CalculatesPhysicalUtilization(t *testing.T) {
	ctx := context.Background()
	envRepo := persistence.NewMemoryEnvironmentRepository()
	typeRepo := persistence.NewMemoryEnvironmentTypeRepository()
	ruleRepo := persistence.NewMemoryAvailabilityRuleRepository(envRepo)
	maintenanceRepo := persistence.NewMemoryMaintenanceRepository(envRepo)
	reservationRepo := persistence.NewMemoryReservationRepository(envRepo)

	const environmentID = "78901234-5678-9012-3456-789012345678"
	if err := ruleRepo.Create(ctx, &model.AvailabilityRule{
		ID: "11111111-1111-4111-8111-111111111111", EnvironmentID: environmentID,
		DayOfWeek: 1, StartTime: time.Date(0, 1, 1, 8, 0, 0, 0, time.UTC), EndTime: time.Date(0, 1, 1, 16, 0, 0, 0, time.UTC),
		EffectiveFrom: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatal(err)
	}
	if err := maintenanceRepo.Create(ctx, &model.Maintenance{
		ID: "22222222-2222-4222-8222-222222222222", EnvironmentID: environmentID,
		StartDate: time.Date(2026, 9, 14, 0, 0, 0, 0, time.UTC), EndDate: time.Date(2026, 9, 15, 0, 0, 0, 0, time.UTC),
		CreatedBy: "33333333-3333-4333-8333-333333333333", CreatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
	if err := reservationRepo.Create(ctx, &model.Reservation{
		ID: "44444444-4444-4444-8444-444444444444", EnvironmentID: environmentID,
		RequesterID: "55555555-5555-4555-8555-555555555555", ReservationDate: time.Date(2026, 9, 14, 0, 0, 0, 0, time.UTC),
		StartTime: "10:00:00", EndTime: "12:00:00", Status: model.ReservationStatusPending,
	}); err != nil {
		t.Fatal(err)
	}

	repo := persistence.NewMemoryEnvironmentUtilizationRepository(envRepo, typeRepo, ruleRepo, maintenanceRepo, reservationRepo)
	page, err := repo.Report(ctx, port.EnvironmentUtilizationQuery{
		Page: 1, PageSize: 20,
		From: time.Date(2026, 9, 14, 0, 0, 0, 0, time.UTC), To: time.Date(2026, 9, 16, 0, 0, 0, 0, time.UTC),
		EnvironmentID: environmentID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 {
		t.Fatalf("expected one environment, got %d", len(page.Items))
	}
	item := page.Items[0]
	if item.AvailableHours != 8 || item.ReservedHours != 2 || item.MaintenanceHours != 8 || item.UtilizationPercentage != 25 {
		t.Fatalf("unexpected utilization: %+v", item)
	}
	if item.EnvironmentTypeCode != "CLASSROOM" || item.PeriodTo.Format(time.DateOnly) != "2026-09-16" {
		t.Fatalf("unexpected identity/range: %+v", item)
	}
}

func TestMemoryEnvironmentUtilizationRepository_ZeroAvailabilityReturnsZeroPercent(t *testing.T) {
	ctx := context.Background()
	envRepo := persistence.NewMemoryEnvironmentRepository()
	typeRepo := persistence.NewMemoryEnvironmentTypeRepository()
	ruleRepo := persistence.NewMemoryAvailabilityRuleRepository(envRepo)
	maintenanceRepo := persistence.NewMemoryMaintenanceRepository(envRepo)
	reservationRepo := persistence.NewMemoryReservationRepository(envRepo)
	repo := persistence.NewMemoryEnvironmentUtilizationRepository(envRepo, typeRepo, ruleRepo, maintenanceRepo, reservationRepo)

	page, err := repo.Report(ctx, port.EnvironmentUtilizationQuery{
		Page: 1, PageSize: 20,
		From: time.Date(2026, 9, 14, 0, 0, 0, 0, time.UTC), To: time.Date(2026, 9, 15, 0, 0, 0, 0, time.UTC),
		EnvironmentID: "89012345-6789-0123-4567-890123456789",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Items[0].AvailableHours != 0 || page.Items[0].UtilizationPercentage != 0 {
		t.Fatalf("expected zero availability to produce zero percent: %+v", page.Items)
	}
}
