package persistence_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/lib/pq"

	"github.com/code-sena/design-software-training-environment-service/internal/adapter/out/persistence"
	"github.com/code-sena/design-software-training-environment-service/internal/application/port"
	"github.com/code-sena/design-software-training-environment-service/internal/application/usecase"
	"github.com/code-sena/design-software-training-environment-service/internal/domain/model"
)

func TestPostgresReservationRepository_HU007_RealExclusionConstraintAndSQLStateMapping(t *testing.T) {
	db := getTestPostgresDB(t)
	if db == nil {
		return
	}
	defer db.Close()

	ctx := context.Background()
	const (
		typeID        = "a1b2c3d4-e5f6-4789-9012-3456789ab701"
		environmentID = "a1b2c3d4-e5f6-4789-9012-3456789ab702"
		requesterID   = "a1b2c3d4-e5f6-4789-9012-3456789ab703"
		secondUserID  = "a1b2c3d4-e5f6-4789-9012-3456789ab704"
	)

	_, err := db.ExecContext(ctx, `
		CREATE EXTENSION IF NOT EXISTS btree_gist;
		CREATE SCHEMA IF NOT EXISTS training_environment;
		CREATE TABLE IF NOT EXISTS training_environment.environment_type (
			id UUID PRIMARY KEY,
			code VARCHAR(30) UNIQUE,
			name VARCHAR(100) NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);
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
		CREATE TABLE IF NOT EXISTS training_environment.reservation (
			id UUID PRIMARY KEY,
			environment_id UUID NOT NULL,
			requester_id UUID NOT NULL,
			reservation_date DATE NOT NULL,
			start_time TIME NOT NULL,
			end_time TIME NOT NULL,
			status VARCHAR(20) NOT NULL DEFAULT 'PENDING',
			created_at TIMESTAMP NOT NULL DEFAULT NOW(),
			reason VARCHAR(255),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);
		ALTER TABLE training_environment.reservation ADD COLUMN IF NOT EXISTS reason VARCHAR(255);
		ALTER TABLE training_environment.reservation ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW();
		DO $$
		BEGIN
			IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'chk_reservation_end_time') THEN
				ALTER TABLE training_environment.reservation
					ADD CONSTRAINT chk_reservation_end_time CHECK (end_time > start_time);
			END IF;
			IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'chk_reservation_status') THEN
				ALTER TABLE training_environment.reservation
					ADD CONSTRAINT chk_reservation_status CHECK (status IN ('PENDING','CONFIRMED','CANCELLED'));
			END IF;
			IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'fk_reservation_environment') THEN
				ALTER TABLE training_environment.reservation
					ADD CONSTRAINT fk_reservation_environment FOREIGN KEY (environment_id)
					REFERENCES training_environment.environment (id);
			END IF;
			IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'excl_reservation_no_overlap') THEN
				ALTER TABLE training_environment.reservation
					ADD CONSTRAINT excl_reservation_no_overlap
					EXCLUDE USING gist (
						environment_id WITH =,
						tsrange((reservation_date + start_time)::timestamp, (reservation_date + end_time)::timestamp) WITH &&
					) WHERE (status <> 'CANCELLED');
			END IF;
		END $$;
	`)
	if err != nil {
		skipOrFailPostgres(t, "unable to prepare HU-ENV-007 PostgreSQL fixture: %v", err)
		return
	}

	cleanup := func() {
		_, _ = db.ExecContext(ctx, `DELETE FROM training_environment.reservation WHERE environment_id = $1`, environmentID)
		_, _ = db.ExecContext(ctx, `DELETE FROM training_environment.environment WHERE id = $1`, environmentID)
		_, _ = db.ExecContext(ctx, `DELETE FROM training_environment.environment_type WHERE id = $1`, typeID)
	}
	cleanup()
	t.Cleanup(cleanup)

	if _, err := db.ExecContext(ctx, `
		INSERT INTO training_environment.environment_type (id, code, name)
		VALUES ($1, 'HU007-IT', 'HU007 Integration Type')`, typeID); err != nil {
		t.Fatalf("failed to insert environment type: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO training_environment.environment
		(id, environment_type_id, training_center_id, name, capacity)
		VALUES ($1, $2, 'c1d2e3f4-a5b6-7890-1234-56789abcdef0', 'HU007 Environment', 25)`, environmentID, typeID); err != nil {
		t.Fatalf("failed to insert environment: %v", err)
	}

	var constraintCount int
	if err := db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM pg_constraint
		WHERE conname = 'excl_reservation_no_overlap'
		  AND conrelid = 'training_environment.reservation'::regclass`).Scan(&constraintCount); err != nil || constraintCount != 1 {
		t.Fatalf("real exclusion constraint excl_reservation_no_overlap not present, count=%d err=%v", constraintCount, err)
	}

	repo := persistence.NewPostgresReservationRepository(db)
	uc := usecase.NewReservationUseCase(repo, persistence.NewPostgresEnvironmentRepository(db))
	first, err := uc.Create(ctx, port.CreateReservationCommand{
		EnvironmentID:   environmentID,
		ReservationDate: time.Date(2026, time.September, 25, 0, 0, 0, 0, time.UTC),
		StartTime:       "08:00",
		EndTime:         "10:00",
		RequesterID:     requesterID,
	})
	if err != nil {
		t.Fatalf("first reservation should be created: %v", err)
	}
	if first.Status != model.ReservationStatusPending {
		t.Fatalf("expected PENDING, got %s", first.Status)
	}

	_, err = uc.Create(ctx, port.CreateReservationCommand{
		EnvironmentID:   environmentID,
		ReservationDate: time.Date(2026, time.September, 25, 0, 0, 0, 0, time.UTC),
		StartTime:       "09:00",
		EndTime:         "11:00",
		RequesterID:     secondUserID,
	})
	if !errors.Is(err, usecase.ErrReservationOverlap) {
		t.Fatalf("expected SQLSTATE 23P01 to map to ErrReservationOverlap, got %v", err)
	}

	// Prove the database engine itself returns SQLSTATE 23P01 for the same real constraint.
	_, directErr := db.ExecContext(ctx, `
		INSERT INTO training_environment.reservation
		(id, environment_id, requester_id, reservation_date, start_time, end_time, status)
		VALUES ('a1b2c3d4-e5f6-4789-9012-3456789ab705', $1, $2, '2026-09-25', '09:30', '10:30', 'CONFIRMED')`, environmentID, secondUserID)
	var postgresErr *pq.Error
	if directErr == nil || !errors.As(directErr, &postgresErr) || string(postgresErr.Code) != "23P01" {
		t.Fatalf("expected direct PostgreSQL exclusion violation SQLSTATE 23P01, got %v", directErr)
	}

	invalidReference := &model.Reservation{
		ID:              "a1b2c3d4-e5f6-4789-9012-3456789ab707",
		EnvironmentID:   "a1b2c3d4-e5f6-4789-9012-3456789ab799",
		RequesterID:     requesterID,
		ReservationDate: time.Date(2026, time.September, 26, 0, 0, 0, 0, time.UTC),
		StartTime:       "08:00:00",
		EndTime:         "09:00:00",
		Status:          model.ReservationStatusPending,
		CreatedAt:       time.Now().UTC(),
		UpdatedAt:       time.Now().UTC(),
	}
	if err := repo.Create(ctx, invalidReference); !errors.Is(err, usecase.ErrReservationInvalidEnvironment) {
		t.Fatalf("expected PostgreSQL FK 23503 to map to invalid environment, got %v", err)
	}

	// CANCELLED rows are intentionally excluded by the partial EXCLUDE constraint.
	if _, err := db.ExecContext(ctx, `
		INSERT INTO training_environment.reservation
		(id, environment_id, requester_id, reservation_date, start_time, end_time, status)
		VALUES ('a1b2c3d4-e5f6-4789-9012-3456789ab706', $1, $2, '2026-09-25', '09:00', '11:00', 'CANCELLED')`, environmentID, secondUserID); err != nil {
		t.Fatalf("CANCELLED overlapping reservation must be allowed by the real constraint: %v", err)
	}

	// Adjacent [10:00,11:00) starts exactly when the first reservation ends and is not an overlap.
	if _, err := uc.Create(ctx, port.CreateReservationCommand{
		EnvironmentID:   environmentID,
		ReservationDate: time.Date(2026, time.September, 25, 0, 0, 0, 0, time.UTC),
		StartTime:       "10:00",
		EndTime:         "11:00",
		RequesterID:     secondUserID,
	}); err != nil {
		t.Fatalf("adjacent reservation should be accepted: %v", err)
	}
}
