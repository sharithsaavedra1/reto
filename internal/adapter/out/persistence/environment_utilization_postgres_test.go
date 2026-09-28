package persistence_test

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	_ "github.com/lib/pq"

	"github.com/code-sena/design-software-training-environment-service/internal/adapter/out/persistence"
	"github.com/code-sena/design-software-training-environment-service/internal/application/port"
)

// This integration test is read-only: it never creates, updates, or deletes
// tables or rows in the configured database.
func TestPostgresEnvironmentUtilizationRepository_Integration(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		databaseURL = os.Getenv("DATABASE_URL")
	}
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL or DATABASE_URL is not configured")
	}

	db, err := sql.Open("postgres", databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		t.Fatal(err)
	}

	_, _ = db.ExecContext(ctx, `
		CREATE SCHEMA IF NOT EXISTS training_environment;
		CREATE TABLE IF NOT EXISTS training_environment.environment_type (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			code VARCHAR(30) UNIQUE NOT NULL,
			name VARCHAR(100) NOT NULL,
			description VARCHAR(255),
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);
		CREATE TABLE IF NOT EXISTS training_environment.environment (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			environment_type_id UUID NOT NULL,
			training_center_id UUID NOT NULL,
			name VARCHAR(100) NOT NULL,
			capacity INT NOT NULL,
			location VARCHAR(200),
			is_active BOOLEAN NOT NULL DEFAULT TRUE,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);
		CREATE TABLE IF NOT EXISTS training_environment.availability_rule (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			environment_id UUID NOT NULL,
			day_of_week SMALLINT NOT NULL,
			start_time TIME NOT NULL,
			end_time TIME NOT NULL,
			effective_from DATE,
			effective_until DATE,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);
		CREATE TABLE IF NOT EXISTS training_environment.maintenance (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			environment_id UUID NOT NULL,
			start_date DATE NOT NULL,
			end_date DATE NOT NULL,
			reason VARCHAR(255),
			created_by UUID NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);
		CREATE TABLE IF NOT EXISTS training_environment.reservation (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			environment_id UUID NOT NULL,
			requester_id UUID NOT NULL,
			reservation_date DATE NOT NULL,
			start_time TIME NOT NULL,
			end_time TIME NOT NULL,
			status VARCHAR(20) NOT NULL DEFAULT 'PENDING',
			reason VARCHAR(255),
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);
	`)

	repo := persistence.NewPostgresEnvironmentUtilizationRepository(db)
	page, err := repo.Report(ctx, port.EnvironmentUtilizationQuery{
		Page: 1, PageSize: 20,
		From: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		To:   time.Date(2026, 1, 8, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	if page.Page != 1 || page.PageSize != 20 {
		t.Fatalf("unexpected pagination: %+v", page)
	}
}
