package persistence_test

import (
	"context"
	"testing"
	"time"

	"github.com/code-sena/design-software-training-environment-service/internal/adapter/out/persistence"
	"github.com/code-sena/design-software-training-environment-service/internal/application/port"
	"github.com/code-sena/design-software-training-environment-service/internal/domain/model"
)

func TestPostgresReservationRepository_HU008_QueryDetailFiltersScopeAndIndexes(t *testing.T) {
	db := getTestPostgresDB(t)
	if db == nil {
		return
	}
	defer db.Close()

	ctx := context.Background()
	const (
		typeID  = "b1b2c3d4-e5f6-4789-9012-3456789ab801"
		envA    = "b1b2c3d4-e5f6-4789-9012-3456789ab802"
		envB    = "b1b2c3d4-e5f6-4789-9012-3456789ab803"
		centerA = "e8a00001-1111-4111-8111-000000000001"
		centerB = "e8b00002-2222-4222-8222-000000000002"
		reqA    = "b1b2c3d4-e5f6-4789-9012-3456789ab804"
		reqB    = "b1b2c3d4-e5f6-4789-9012-3456789ab805"
		res1    = "b1b2c3d4-e5f6-4789-9012-3456789ab811"
		res2    = "b1b2c3d4-e5f6-4789-9012-3456789ab812"
		res3    = "b1b2c3d4-e5f6-4789-9012-3456789ab813"
		res4    = "b1b2c3d4-e5f6-4789-9012-3456789ab814"
	)

	_, err := db.ExecContext(ctx, `
        CREATE SCHEMA IF NOT EXISTS training_environment;
        CREATE TABLE IF NOT EXISTS training_environment.environment_type (
            id UUID PRIMARY KEY,
            name VARCHAR(100) NOT NULL,
            description VARCHAR(255),
            created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
        );
        ALTER TABLE training_environment.environment_type ADD COLUMN IF NOT EXISTS code VARCHAR(30);
        ALTER TABLE training_environment.environment_type ADD COLUMN IF NOT EXISTS description VARCHAR(255);

        CREATE TABLE IF NOT EXISTS training_environment.environment (
            id UUID PRIMARY KEY,
            environment_type_id UUID NOT NULL,
            training_center_id UUID NOT NULL,
            name VARCHAR(100) NOT NULL,
            capacity INT NOT NULL,
            location VARCHAR(200),
            is_active BOOLEAN NOT NULL DEFAULT TRUE,
            created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
            updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
        );
        ALTER TABLE training_environment.environment ADD COLUMN IF NOT EXISTS location VARCHAR(200);
        ALTER TABLE training_environment.environment ADD COLUMN IF NOT EXISTS is_active BOOLEAN NOT NULL DEFAULT TRUE;
        ALTER TABLE training_environment.environment ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW();

        CREATE TABLE IF NOT EXISTS training_environment.reservation (
            id UUID PRIMARY KEY,
            environment_id UUID NOT NULL,
            requester_id UUID NOT NULL,
            reservation_date DATE NOT NULL,
            start_time TIME NOT NULL,
            end_time TIME NOT NULL,
            status VARCHAR(20) NOT NULL DEFAULT 'PENDING',
            created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
            reason VARCHAR(255),
            updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
        );
        ALTER TABLE training_environment.reservation ADD COLUMN IF NOT EXISTS reason VARCHAR(255);
        ALTER TABLE training_environment.reservation ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW();
        CREATE INDEX IF NOT EXISTS idx_reservation_environment_date
            ON training_environment.reservation (environment_id, reservation_date);
        CREATE INDEX IF NOT EXISTS idx_reservation_requester_created_at
            ON training_environment.reservation (requester_id, created_at);
    `)
	if err != nil {
		skipOrFailPostgres(t, "unable to prepare HU-ENV-008 PostgreSQL fixture: %v", err)
		return
	}

	cleanup := func() {
		_, _ = db.ExecContext(ctx, `DELETE FROM training_environment.reservation WHERE id IN ($1,$2,$3,$4)`, res1, res2, res3, res4)
		_, _ = db.ExecContext(ctx, `DELETE FROM training_environment.environment WHERE id IN ($1,$2)`, envA, envB)
		_, _ = db.ExecContext(ctx, `DELETE FROM training_environment.environment_type WHERE id = $1`, typeID)
	}
	cleanup()
	t.Cleanup(cleanup)

	if _, err := db.ExecContext(ctx, `
        INSERT INTO training_environment.environment_type (id, code, name, description)
        VALUES ($1, 'HU008-IT', 'HU008 Integration Type', 'HU008')`, typeID); err != nil {
		t.Fatalf("insert environment type: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
        INSERT INTO training_environment.environment
        (id, environment_type_id, training_center_id, name, capacity, location)
        VALUES
        ($1,$3,$4,'HU008 Environment A',20,'A'),
        ($2,$3,$5,'HU008 Environment B',20,'B')`, envA, envB, typeID, centerA, centerB); err != nil {
		t.Fatalf("insert environments: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
        INSERT INTO training_environment.reservation
        (id, environment_id, requester_id, reservation_date, start_time, end_time, status, created_at, updated_at)
        VALUES
        ($1,$5,$7,'2026-09-20','08:00','09:00','PENDING','2026-09-15T10:00:00Z','2026-09-15T10:00:00Z'),
        ($2,$5,$8,'2026-09-21','10:00','11:00','CANCELLED','2026-09-15T11:00:00Z','2026-09-15T11:00:00Z'),
        ($3,$5,$7,'2026-09-22','12:00','13:00','CONFIRMED','2026-09-15T12:00:00Z','2026-09-15T12:00:00Z'),
        ($4,$6,$7,'2026-09-23','14:00','15:00','PENDING','2026-09-15T13:00:00Z','2026-09-15T13:00:00Z')`,
		res1, res2, res3, res4, envA, envB, reqA, reqB); err != nil {
		t.Fatalf("insert reservations: %v", err)
	}

	for _, indexName := range []string{"idx_reservation_environment_date", "idx_reservation_requester_created_at"} {
		var count int
		if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM pg_indexes WHERE schemaname='training_environment' AND indexname=$1`, indexName).Scan(&count); err != nil || count != 1 {
			t.Fatalf("required index %s not present: count=%d err=%v", indexName, count, err)
		}
	}

	repo := persistence.NewPostgresReservationRepository(db)
	from := time.Date(2026, 9, 20, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 9, 23, 0, 0, 0, 0, time.UTC)
	page, err := repo.Query(ctx, port.ReservationListQuery{
		Page: 1, PageSize: 10, SortField: "reservation_date",
		EnvironmentID: envA, From: &from, To: &to,
	})
	if err != nil {
		t.Fatalf("query reservations: %v", err)
	}
	if page.TotalItems != 3 || len(page.Items) != 3 {
		t.Fatalf("expected three envA reservations in [from,to), got %+v", page)
	}
	cancelled, err := repo.Query(ctx, port.ReservationListQuery{
		Page:          1,
		PageSize:      20,
		SortField:     "created_at",
		Status:        model.ReservationStatusCancelled,
		EnvironmentID: envA,
	})
	if err != nil || cancelled.TotalItems != 1 || cancelled.Items[0].ID != res2 {
		t.Fatalf("CANCELLED filter failed: %+v err=%v", cancelled, err)
	}

	scoped, err := repo.Query(ctx, port.ReservationListQuery{Page: 1, PageSize: 20, SortField: "created_at", TrainingCenterID: centerA})
	if err != nil || scoped.TotalItems != 3 {
		t.Fatalf("TRAINING_CENTER scope failed: %+v err=%v", scoped, err)
	}

	detail, err := repo.FindByID(ctx, res1, centerA)
	if err != nil || detail == nil || detail.ID != res1 {
		t.Fatalf("detail in scope failed: %+v err=%v", detail, err)
	}
	hidden, err := repo.FindByID(ctx, res4, centerA)
	if err != nil || hidden != nil {
		t.Fatalf("out-of-scope detail must be hidden as not found: %+v err=%v", hidden, err)
	}

	empty, err := repo.Query(ctx, port.ReservationListQuery{Page: 1, PageSize: 20, SortField: "created_at", RequesterID: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"})
	if err != nil || empty.TotalItems != 0 || len(empty.Items) != 0 || empty.TotalPages != 0 {
		t.Fatalf("empty result must be 200-compatible and coherent: %+v err=%v", empty, err)
	}
}
