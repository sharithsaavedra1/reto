package persistence_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/lib/pq"

	"github.com/code-sena/design-software-training-environment-service/internal/adapter/out/persistence"
	"github.com/code-sena/design-software-training-environment-service/internal/application/port"
	"github.com/code-sena/design-software-training-environment-service/internal/application/usecase"
)

func TestPostgresMaintenanceRepository_HU006_CRUDFiltersAndTransactionalOutbox(t *testing.T) {
	db := getTestPostgresDB(t)
	if db == nil {
		return
	}
	defer db.Close()

	ctx := context.Background()
	const (
		typeID        = "a1b2c3d4-e5f6-4789-9012-3456789abcde"
		environmentID = "a1b2c3d4-e5f6-4789-9012-3456789abc01"
		otherEnvID    = "a1b2c3d4-e5f6-4789-9012-3456789abc02"
		createdBy     = "42c78701-2b34-4a6f-b034-3f7a3eca57d8"
	)

	_, err := db.ExecContext(ctx, `
		CREATE SCHEMA IF NOT EXISTS training_environment;
		CREATE TABLE IF NOT EXISTS training_environment.environment_type (
			id UUID PRIMARY KEY,
			code VARCHAR(30) UNIQUE NOT NULL,
			name VARCHAR(100) NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);
		CREATE TABLE IF NOT EXISTS training_environment.environment (
			id UUID PRIMARY KEY,
			environment_type_id UUID NOT NULL,
			training_center_id UUID NOT NULL,
			name VARCHAR(100) NOT NULL,
			capacity INT NOT NULL,
			is_active BOOLEAN NOT NULL DEFAULT TRUE,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);
		CREATE TABLE IF NOT EXISTS training_environment.maintenance (
			id UUID PRIMARY KEY,
			environment_id UUID NOT NULL,
			start_date DATE NOT NULL,
			end_date DATE NOT NULL,
			reason VARCHAR(255),
			created_by UUID NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			CONSTRAINT chk_maintenance_end_date_hu006 CHECK (end_date > start_date),
			CONSTRAINT fk_maintenance_environment_hu006 FOREIGN KEY (environment_id)
				REFERENCES training_environment.environment (id)
		);
		CREATE TABLE IF NOT EXISTS training_environment.outbox (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			event_id UUID NOT NULL UNIQUE,
			event_type VARCHAR(128) NOT NULL,
			payload JSONB NOT NULL,
			status VARCHAR(16) NOT NULL DEFAULT 'PENDING',
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			published_at TIMESTAMPTZ
		);
		ALTER TABLE training_environment.maintenance
			ADD COLUMN IF NOT EXISTS created_at TIMESTAMPTZ NOT NULL DEFAULT NOW();
		DO $$
		BEGIN
			IF NOT EXISTS (
				SELECT 1 FROM pg_constraint
				WHERE conname = 'chk_maintenance_end_date_hu006'
			) THEN
				ALTER TABLE training_environment.maintenance
					ADD CONSTRAINT chk_maintenance_end_date_hu006 CHECK (end_date > start_date);
			END IF;
			IF NOT EXISTS (
				SELECT 1 FROM pg_constraint
				WHERE conname = 'fk_maintenance_environment_hu006'
			) THEN
				ALTER TABLE training_environment.maintenance
					ADD CONSTRAINT fk_maintenance_environment_hu006
					FOREIGN KEY (environment_id) REFERENCES training_environment.environment (id);
			END IF;
		END $$;
	`)
	if err != nil {
		skipOrFailPostgres(t, "unable to prepare HU-ENV-006 PostgreSQL tables: %v", err)
		return
	}

	cleanup := func() {
		_, _ = db.ExecContext(ctx, `DELETE FROM training_environment.outbox WHERE payload->'payload'->>'environment_id' = $1`, environmentID)
		_, _ = db.ExecContext(ctx, `DELETE FROM training_environment.maintenance WHERE environment_id IN ($1, $2)`, environmentID, otherEnvID)
		_, _ = db.ExecContext(ctx, `DELETE FROM training_environment.environment WHERE id IN ($1, $2)`, environmentID, otherEnvID)
		_, _ = db.ExecContext(ctx, `DELETE FROM training_environment.environment_type WHERE id = $1`, typeID)
	}
	cleanup()
	t.Cleanup(cleanup)

	if _, err := db.ExecContext(ctx, `
		INSERT INTO training_environment.environment_type (id, code, name)
		VALUES ($1, 'HU006-IT', 'HU006 Integration Type')`, typeID); err != nil {
		t.Fatalf("failed to insert environment type: %v", err)
	}
	for _, id := range []string{environmentID, otherEnvID} {
		if _, err := db.ExecContext(ctx, `
			INSERT INTO training_environment.environment
			(id, environment_type_id, training_center_id, name, capacity)
			VALUES ($1, $2, 'c1d2e3f4-a5b6-7890-1234-56789abcdef0', 'HU006 Environment', 25)`, id, typeID); err != nil {
			t.Fatalf("failed to insert environment %s: %v", id, err)
		}
	}

	repo := persistence.NewPostgresMaintenanceRepository(db)
	uc := usecase.NewMaintenanceUseCase(repo)
	reason := "Preventive electrical maintenance"
	first, err := uc.Create(ctx, port.MaintenanceCreateCommand{
		EnvironmentID: environmentID,
		StartDate:     time.Date(2026, time.June, 1, 0, 0, 0, 0, time.UTC),
		EndDate:       time.Date(2026, time.June, 10, 0, 0, 0, 0, time.UTC),
		Reason:        &reason,
		CreatedBy:     createdBy,
	})
	if err != nil {
		t.Fatalf("failed to create maintenance in PostgreSQL: %v", err)
	}
	assertMaintenanceOutboxEvent(t, ctx, db, environmentID, first.ID, createdBy)

	second, err := uc.Create(ctx, port.MaintenanceCreateCommand{
		EnvironmentID: environmentID,
		StartDate:     time.Date(2026, time.June, 20, 0, 0, 0, 0, time.UTC),
		EndDate:       time.Date(2026, time.June, 30, 0, 0, 0, 0, time.UTC),
		CreatedBy:     createdBy,
	})
	if err != nil {
		t.Fatalf("failed to create maintenance without reason: %v", err)
	}

	items, err := repo.List(ctx, port.MaintenanceListQuery{EnvironmentID: environmentID, From: datePointer(2026, time.June, 5), To: datePointer(2026, time.June, 15)})
	if err != nil || len(items) != 1 || items[0].ID != first.ID {
		t.Fatalf("expected only the first overlapping maintenance, items=%+v err=%v", items, err)
	}
	if items, err := repo.List(ctx, port.MaintenanceListQuery{EnvironmentID: environmentID, From: datePointer(2026, time.June, 10)}); err != nil || len(items) != 1 || items[0].ID != second.ID {
		t.Fatalf("expected boundary from=2026-06-10 to exclude the first period, items=%+v err=%v", items, err)
	}
	if items, err := repo.List(ctx, port.MaintenanceListQuery{EnvironmentID: environmentID, To: datePointer(2026, time.June, 1)}); err != nil || len(items) != 0 {
		t.Fatalf("expected boundary to=2026-06-01 to return no periods, items=%+v err=%v", items, err)
	}

	detail, err := repo.Get(ctx, environmentID, first.ID)
	if err != nil || detail.Reason == nil || *detail.Reason != reason {
		t.Fatalf("failed to retrieve maintenance detail, detail=%+v err=%v", detail, err)
	}
	if _, err := repo.Get(ctx, otherEnvID, first.ID); !errors.Is(err, usecase.ErrMaintenanceNotFound) {
		t.Fatalf("expected wrong-environment detail to return 404 error, got %v", err)
	}

	beforeInvalid := countMaintenanceOutbox(t, ctx, db, environmentID)
	if _, err := uc.Create(ctx, port.MaintenanceCreateCommand{
		EnvironmentID: environmentID,
		StartDate:     time.Date(2026, time.June, 10, 0, 0, 0, 0, time.UTC),
		EndDate:       time.Date(2026, time.June, 10, 0, 0, 0, 0, time.UTC),
		CreatedBy:     createdBy,
	}); !errors.Is(err, usecase.ErrMaintenanceValidation) {
		t.Fatalf("expected invalid date range error, got %v", err)
	}
	if afterInvalid := countMaintenanceOutbox(t, ctx, db, environmentID); afterInvalid != beforeInvalid {
		t.Fatalf("invalid creation generated an outbox event: before=%d after=%d", beforeInvalid, afterInvalid)
	}

	if _, err := db.ExecContext(ctx, `
		INSERT INTO training_environment.maintenance
		(id, environment_id, start_date, end_date, reason, created_by)
		VALUES ('a1b2c3d4-e5f6-4789-9012-3456789abd01', $1, '2026-07-01', '2026-07-01', NULL, $2)`, environmentID, createdBy); err == nil || postgresErrorCode(err) != "23514" {
		t.Fatalf("expected PostgreSQL chk_maintenance_end_date violation, got %v", err)
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO training_environment.maintenance
		(id, environment_id, start_date, end_date, reason, created_by)
		VALUES ('a1b2c3d4-e5f6-4789-9012-3456789abd02', 'a1b2c3d4-e5f6-4789-9012-3456789abc99', '2026-07-01', '2026-07-02', NULL, $1)`, createdBy); err == nil || postgresErrorCode(err) != "23503" {
		t.Fatalf("expected PostgreSQL maintenance FK violation, got %v", err)
	}

	var persisted int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM training_environment.maintenance WHERE environment_id = $1`, environmentID).Scan(&persisted); err != nil {
		t.Fatalf("failed to verify persisted maintenance rows: %v", err)
	}
	if persisted != 2 || countMaintenanceOutbox(t, ctx, db, environmentID) != 2 || second.ID == first.ID {
		t.Fatalf("expected two persisted maintenances and two outbox events, rows=%d events=%d", persisted, countMaintenanceOutbox(t, ctx, db, environmentID))
	}
}

func assertMaintenanceOutboxEvent(t *testing.T, ctx context.Context, db *sql.DB, environmentID, maintenanceID, createdBy string) {
	t.Helper()
	var eventType string
	var raw []byte
	if err := db.QueryRowContext(ctx, `
		SELECT event_type, payload
		FROM training_environment.outbox
		WHERE event_type = 'environment.maintenance.started'
		  AND payload->'payload'->>'environment_id' = $1
		  AND payload->'payload'->>'maintenance_id' = $2
		ORDER BY created_at DESC LIMIT 1`, environmentID, maintenanceID).Scan(&eventType, &raw); err != nil {
		t.Fatalf("expected maintenance.started outbox event: %v", err)
	}
	var envelope map[string]interface{}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatalf("invalid maintenance event JSON: %v", err)
	}
	payload, ok := envelope["payload"].(map[string]interface{})
	if eventType != "environment.maintenance.started" || envelope["event_type"] != eventType || !ok || payload["created_by"] != createdBy {
		t.Fatalf("unexpected maintenance event: type=%s payload=%s", eventType, string(raw))
	}
}

func countMaintenanceOutbox(t *testing.T, ctx context.Context, db *sql.DB, environmentID string) int {
	t.Helper()
	var count int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM training_environment.outbox WHERE event_type = 'environment.maintenance.started' AND payload->'payload'->>'environment_id' = $1`, environmentID).Scan(&count); err != nil {
		t.Fatalf("failed to count maintenance outbox events: %v", err)
	}
	return count
}

func datePointer(year int, month time.Month, day int) *time.Time {
	value := time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
	return &value
}

func postgresErrorCode(err error) string {
	var postgresErr *pq.Error
	if errors.As(err, &postgresErr) {
		return string(postgresErr.Code)
	}
	return ""
}
