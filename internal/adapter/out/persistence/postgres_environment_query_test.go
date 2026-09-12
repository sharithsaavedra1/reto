package persistence_test

import (
	"context"
	"testing"
	"time"

	"github.com/code-sena/design-software-training-environment-service/internal/adapter/out/persistence"
	"github.com/code-sena/design-software-training-environment-service/internal/application/port"
)

func TestPostgresEnvironmentRepository_QueryIntegration(t *testing.T) {
	db := getTestPostgresDB(t)
	if db == nil {
		return
	}
	defer db.Close()

	ctx := context.Background()
	_, err := db.ExecContext(ctx, `
		CREATE SCHEMA IF NOT EXISTS training_environment;
		CREATE TABLE IF NOT EXISTS training_environment.environment_type (
			id VARCHAR(36) PRIMARY KEY,
			code VARCHAR(20) UNIQUE NOT NULL,
			name VARCHAR(100) NOT NULL,
			description VARCHAR(255),
			created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
		);
		CREATE TABLE IF NOT EXISTS training_environment.environment (
			id VARCHAR(36) PRIMARY KEY,
			environment_type_id VARCHAR(36) NOT NULL,
			training_center_id VARCHAR(36) NOT NULL,
			name VARCHAR(100) NOT NULL,
			capacity INT NOT NULL,
			location VARCHAR(200),
			is_active BOOLEAN NOT NULL DEFAULT TRUE,
			created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
			updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
		);
		CREATE TABLE IF NOT EXISTS training_environment.availability_rule (
			id VARCHAR(36) PRIMARY KEY,
			environment_id VARCHAR(36) NOT NULL,
			day_of_week SMALLINT NOT NULL,
			start_time TIME NOT NULL,
			end_time TIME NOT NULL,
			effective_from DATE NOT NULL,
			effective_until DATE
		);
		CREATE TABLE IF NOT EXISTS training_environment.maintenance (
			id VARCHAR(36) PRIMARY KEY,
			environment_id VARCHAR(36) NOT NULL,
			start_date DATE NOT NULL,
			end_date DATE NOT NULL,
			reason TEXT
		);
		CREATE TABLE IF NOT EXISTS training_environment.reservation (
			id VARCHAR(36) PRIMARY KEY,
			environment_id VARCHAR(36) NOT NULL,
			reservation_date DATE NOT NULL,
			start_time TIME NOT NULL,
			end_time TIME NOT NULL,
			status VARCHAR(20) NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_availability_rule_environment_day
			ON training_environment.availability_rule (environment_id, day_of_week);
		CREATE INDEX IF NOT EXISTS idx_maintenance_environment_dates
			ON training_environment.maintenance (environment_id, start_date, end_date);
		CREATE INDEX IF NOT EXISTS idx_reservation_environment_date
			ON training_environment.reservation (environment_id, reservation_date);
	`)
	if err != nil {
		t.Skipf("Skipping PostgreSQL query integration test: unable to prepare tables: %v", err)
		return
	}

	const (
		typeID          = "f1e2d3c4-b5a6-4789-9012-3456789abcde"
		envAvailableID  = "f1e2d3c4-b5a6-4789-9012-3456789abc01"
		envBlockedID    = "f1e2d3c4-b5a6-4789-9012-3456789abc02"
		ruleAvailableID = "f1e2d3c4-b5a6-4789-9012-3456789abd01"
		ruleBlockedID   = "f1e2d3c4-b5a6-4789-9012-3456789abd02"
		maintenanceID   = "f1e2d3c4-b5a6-4789-9012-3456789abe01"
		reservationID   = "f1e2d3c4-b5a6-4789-9012-3456789abe02"
	)
	_, _ = db.ExecContext(ctx, `DELETE FROM training_environment.reservation WHERE environment_id IN ($1, $2)`, envAvailableID, envBlockedID)
	_, _ = db.ExecContext(ctx, `DELETE FROM training_environment.maintenance WHERE environment_id IN ($1, $2)`, envAvailableID, envBlockedID)
	_, _ = db.ExecContext(ctx, `DELETE FROM training_environment.availability_rule WHERE environment_id IN ($1, $2)`, envAvailableID, envBlockedID)
	_, _ = db.ExecContext(ctx, `DELETE FROM training_environment.environment WHERE id IN ($1, $2)`, envAvailableID, envBlockedID)
	_, _ = db.ExecContext(ctx, `DELETE FROM training_environment.environment_type WHERE id = $1`, typeID)

	if _, err := db.ExecContext(ctx, `
		INSERT INTO training_environment.environment_type (id, code, name, description)
		VALUES ($1, 'HU003-Q', 'HU003 Query Type', 'query integration')`, typeID); err != nil {
		t.Fatalf("failed to insert query test environment type: %v", err)
	}
	for _, environment := range []struct {
		id     string
		name   string
		ruleID string
	}{
		{envAvailableID, "HU003 Available", ruleAvailableID},
		{envBlockedID, "HU003 Blocked", ruleBlockedID},
	} {
		if _, err := db.ExecContext(ctx, `
			INSERT INTO training_environment.environment
			(id, environment_type_id, training_center_id, name, capacity, location, is_active, created_at, updated_at)
			VALUES ($1, $2, 'c1d2e3f4-a5b6-7890-1234-56789abcdef0', $3, 20, NULL, TRUE, NOW(), NOW())`, environment.id, typeID, environment.name); err != nil {
			t.Fatalf("failed to insert query test environment: %v", err)
		}
		if _, err := db.ExecContext(ctx, `
			INSERT INTO training_environment.availability_rule
			(id, environment_id, day_of_week, start_time, end_time, effective_from, effective_until)
			VALUES ($1, $2, 2, '08:00', '18:00', '2020-01-01', NULL)`, environment.ruleID, environment.id); err != nil {
			t.Fatalf("failed to insert availability rule: %v", err)
		}
	}

	if _, err := db.ExecContext(ctx, `
		INSERT INTO training_environment.maintenance
		(id, environment_id, start_date, end_date, reason)
		VALUES ($1, $2, '2026-09-15', '2026-09-16', 'HU003 maintenance')`, maintenanceID, envBlockedID); err != nil {
		t.Fatalf("failed to insert maintenance fixture: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO training_environment.reservation
		(id, environment_id, reservation_date, start_time, end_time, status)
		VALUES ($1, $2, '2026-09-16', '10:00', '12:00', 'PENDING')`, reservationID, envAvailableID); err != nil {
		t.Fatalf("failed to insert reservation fixture: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.ExecContext(ctx, `DELETE FROM training_environment.reservation WHERE environment_id IN ($1, $2)`, envAvailableID, envBlockedID)
		_, _ = db.ExecContext(ctx, `DELETE FROM training_environment.maintenance WHERE environment_id IN ($1, $2)`, envAvailableID, envBlockedID)
		_, _ = db.ExecContext(ctx, `DELETE FROM training_environment.availability_rule WHERE environment_id IN ($1, $2)`, envAvailableID, envBlockedID)
		_, _ = db.ExecContext(ctx, `DELETE FROM training_environment.environment WHERE id IN ($1, $2)`, envAvailableID, envBlockedID)
		_, _ = db.ExecContext(ctx, `DELETE FROM training_environment.environment_type WHERE id = $1`, typeID)
	})

	repo := persistence.NewPostgresEnvironmentRepository(db)
	date := time.Date(2026, time.September, 15, 0, 0, 0, 0, time.UTC)
	start := time.Date(0, time.January, 1, 9, 0, 0, 0, time.UTC)
	end := time.Date(0, time.January, 1, 10, 0, 0, 0, time.UTC)
	page, err := repo.Query(ctx, port.EnvironmentListQuery{
		Page:              1,
		PageSize:          10,
		SortField:         "name",
		EnvironmentTypeID: typeID,
		AvailableDate:     &date,
		AvailableStart:    &start,
		AvailableEnd:      &end,
	})
	if err != nil {
		t.Fatalf("failed to query available environments: %v", err)
	}
	if page.TotalItems != 1 || len(page.Items) != 1 || page.Items[0].ID != envAvailableID {
		t.Fatalf("expected only the environment without maintenance, got: %+v", page)
	}

	date = time.Date(2026, time.September, 16, 0, 0, 0, 0, time.UTC)
	start = time.Date(0, time.January, 1, 10, 30, 0, 0, time.UTC)
	end = time.Date(0, time.January, 1, 11, 0, 0, 0, time.UTC)
	page, err = repo.Query(ctx, port.EnvironmentListQuery{
		Page:           1,
		PageSize:       10,
		SortField:      "name",
		AvailableDate:  &date,
		AvailableStart: &start,
		AvailableEnd:   &end,
	})
	if err != nil {
		t.Fatalf("failed to query reservation availability: %v", err)
	}
	for _, item := range page.Items {
		if item.ID == envAvailableID {
			t.Fatalf("pending reservation was not excluded: %+v", page.Items)
		}
	}
}
