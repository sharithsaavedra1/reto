package persistence_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/code-sena/design-software-training-environment-service/internal/adapter/out/persistence"
	"github.com/code-sena/design-software-training-environment-service/internal/application/port"
	"github.com/code-sena/design-software-training-environment-service/internal/application/usecase"
	"github.com/code-sena/design-software-training-environment-service/internal/domain/model"
)

func TestPostgresReservationRepository_HU019_IndexesFiltersAndExclusion(t *testing.T) {
	db := getTestPostgresDB(t)
	if db == nil {
		return
	}
	defer db.Close()

	ctx := context.Background()

	expectedIndexes := []string{
		"idx_reservation_environment_date",
		"idx_reservation_requester_created_at",
		"idx_reservation_requester_date",
		"idx_reservation_status_date",
	}
	for _, indexName := range expectedIndexes {
		var count int
		if err := db.QueryRowContext(ctx, `
            SELECT COUNT(*)
            FROM pg_indexes
            WHERE schemaname = 'training_environment'
              AND tablename = 'reservation'
              AND indexname = $1`, indexName).Scan(&count); err != nil {
			t.Fatalf("inspect index %s: %v", indexName, err)
		}
		if count != 1 {
			t.Fatalf("expected exactly one %s index, got %d", indexName, count)
		}
	}

	var exclusionCount int
	if err := db.QueryRowContext(ctx, `
        SELECT COUNT(*)
        FROM pg_constraint
        WHERE conrelid = 'training_environment.reservation'::regclass
          AND conname = 'excl_reservation_no_overlap'`).Scan(&exclusionCount); err != nil {
		t.Fatalf("inspect exclusion constraint: %v", err)
	}
	if exclusionCount != 1 {
		t.Fatalf("excl_reservation_no_overlap must remain present, got %d", exclusionCount)
	}

	const (
		typeID      = "f1900000-0000-4000-8000-000000000001"
		environment = "f1900000-0000-4000-8000-000000000002"
		centerID    = "f1900000-0000-4000-8000-000000000003"
		requester   = "f1900000-0000-4000-8000-000000000004"
		activeID    = "f1900000-0000-4000-8000-000000000011"
		overlapID   = "f1900000-0000-4000-8000-000000000012"
		cancelledID = "f1900000-0000-4000-8000-000000000013"
	)

	cleanup := func() {
		_, _ = db.ExecContext(ctx, `DELETE FROM training_environment.reservation WHERE id IN ($1,$2,$3)`, activeID, overlapID, cancelledID)
		_, _ = db.ExecContext(ctx, `DELETE FROM training_environment.environment WHERE id = $1`, environment)
		_, _ = db.ExecContext(ctx, `DELETE FROM training_environment.environment_type WHERE id = $1`, typeID)
	}
	cleanup()
	t.Cleanup(cleanup)

	if _, err := db.ExecContext(ctx, `
        INSERT INTO training_environment.environment_type (id, code, name, description)
        VALUES ($1, 'HU019-IT', 'HU019 Integration Type', 'HU019 index validation')`, typeID); err != nil {
		t.Fatalf("insert environment type: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
        INSERT INTO training_environment.environment
            (id, environment_type_id, training_center_id, name, capacity, location, is_active)
        VALUES ($1,$2,$3,'HU019 Environment',20,'HU019',TRUE)`, environment, typeID, centerID); err != nil {
		t.Fatalf("insert environment: %v", err)
	}

	repo := persistence.NewPostgresReservationRepository(db)
	base := model.Reservation{
		ID:              activeID,
		EnvironmentID:   environment,
		RequesterID:     requester,
		ReservationDate: time.Date(2026, 10, 10, 0, 0, 0, 0, time.UTC),
		StartTime:       "08:00:00",
		EndTime:         "10:00:00",
		Status:          model.ReservationStatusPending,
		CreatedAt:       time.Now().UTC(),
		UpdatedAt:       time.Now().UTC(),
	}
	if err := repo.Create(ctx, &base); err != nil {
		t.Fatalf("create baseline reservation: %v", err)
	}

	overlapping := base
	overlapping.ID = overlapID
	overlapping.StartTime = "09:00:00"
	overlapping.EndTime = "11:00:00"
	overlapping.Status = model.ReservationStatusConfirmed
	if err := repo.Create(ctx, &overlapping); !errors.Is(err, usecase.ErrReservationOverlap) {
		t.Fatalf("active overlap must still be rejected, got %v", err)
	}

	cancelled := overlapping
	cancelled.ID = cancelledID
	cancelled.Status = model.ReservationStatusCancelled
	if err := repo.Create(ctx, &cancelled); err != nil {
		t.Fatalf("CANCELLED overlap must keep historical behavior: %v", err)
	}

	from := time.Date(2026, 10, 10, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 10, 11, 0, 0, 0, 0, time.UTC)

	requesterPage, err := repo.Query(ctx, port.ReservationListQuery{
		Page: 1, PageSize: 20, SortField: "reservation_date",
		RequesterID: requester, From: &from, To: &to,
	})
	if err != nil || requesterPage.TotalItems != 2 {
		t.Fatalf("requester/date filter changed behavior: page=%+v err=%v", requesterPage, err)
	}

	cancelledPage, err := repo.Query(ctx, port.ReservationListQuery{
		Page: 1, PageSize: 20, SortField: "reservation_date",
		RequesterID: requester, Status: model.ReservationStatusCancelled, From: &from, To: &to,
	})
	if err != nil || cancelledPage.TotalItems != 1 || len(cancelledPage.Items) != 1 || cancelledPage.Items[0].ID != cancelledID {
		t.Fatalf("status/date filter must return CANCELLED rows normally: page=%+v err=%v", cancelledPage, err)
	}
}
