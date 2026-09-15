package persistence_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/code-sena/design-software-training-environment-service/internal/adapter/out/persistence"
	"github.com/code-sena/design-software-training-environment-service/internal/application/port"
	"github.com/code-sena/design-software-training-environment-service/internal/application/usecase"
	"github.com/code-sena/design-software-training-environment-service/internal/domain/model"
)

func TestPostgresAvailabilityRuleRepository_HU005_CRUDAndTransactionalOutbox(t *testing.T) {
	db := getTestPostgresDB(t)
	if db == nil {
		return
	}
	defer db.Close()

	ctx := context.Background()
	const (
		typeID        = "d1e2f3a4-b5c6-4789-9012-3456789abcde"
		environmentID = "d1e2f3a4-b5c6-4789-9012-3456789abc01"
		otherEnvID    = "d1e2f3a4-b5c6-4789-9012-3456789abc02"
	)

	_, err := db.ExecContext(ctx, `
		CREATE SCHEMA IF NOT EXISTS training_environment;
		CREATE TABLE IF NOT EXISTS training_environment.environment_type (
			id VARCHAR(36) PRIMARY KEY,
			code VARCHAR(30) UNIQUE NOT NULL,
			name VARCHAR(100) NOT NULL,
			description VARCHAR(255),
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);
		CREATE TABLE IF NOT EXISTS training_environment.environment (
			id VARCHAR(36) PRIMARY KEY,
			environment_type_id VARCHAR(36) NOT NULL,
			training_center_id VARCHAR(36) NOT NULL,
			name VARCHAR(100) NOT NULL,
			capacity INT NOT NULL,
			location VARCHAR(200),
			is_active BOOLEAN NOT NULL DEFAULT TRUE,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);
		CREATE TABLE IF NOT EXISTS training_environment.availability_rule (
			id VARCHAR(36) PRIMARY KEY,
			environment_id VARCHAR(36) NOT NULL,
			day_of_week SMALLINT NOT NULL,
			start_time TIME NOT NULL,
			end_time TIME NOT NULL,
			effective_from DATE NOT NULL,
			effective_until DATE,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			CONSTRAINT chk_hu005_day CHECK (day_of_week BETWEEN 1 AND 7),
			CONSTRAINT chk_hu005_time CHECK (end_time > start_time)
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
	`)
	if err != nil {
		skipOrFailPostgres(t, "unable to prepare HU-ENV-005 PostgreSQL tables: %v", err)
		return
	}

	cleanup := func() {
		_, _ = db.ExecContext(ctx, `DELETE FROM training_environment.outbox WHERE payload->'payload'->>'environment_id' = $1`, environmentID)
		_, _ = db.ExecContext(ctx, `DELETE FROM training_environment.availability_rule WHERE environment_id IN ($1, $2)`, environmentID, otherEnvID)
		_, _ = db.ExecContext(ctx, `DELETE FROM training_environment.environment WHERE id IN ($1, $2)`, environmentID, otherEnvID)
		_, _ = db.ExecContext(ctx, `DELETE FROM training_environment.environment_type WHERE id = $1`, typeID)
	}
	cleanup()
	t.Cleanup(cleanup)

	if _, err := db.ExecContext(ctx, `
		INSERT INTO training_environment.environment_type (id, code, name)
		VALUES ($1, 'HU005-IT', 'HU005 Integration Type')`, typeID); err != nil {
		t.Fatalf("failed to insert environment type: %v", err)
	}
	for _, environmentID := range []string{environmentID, otherEnvID} {
		if _, err := db.ExecContext(ctx, `
			INSERT INTO training_environment.environment
			(id, environment_type_id, training_center_id, name, capacity)
			VALUES ($1, $2, 'c1d2e3f4-a5b6-7890-1234-56789abcdef0', 'HU005 Environment', 25)`, environmentID, typeID); err != nil {
			t.Fatalf("failed to insert environment: %v", err)
		}
	}

	repo := persistence.NewPostgresAvailabilityRuleRepository(db)
	uc := usecase.NewAvailabilityRuleUseCase(repo)
	effectiveUntil := time.Date(2026, time.December, 31, 0, 0, 0, 0, time.UTC)
	created, err := uc.Create(ctx, port.AvailabilityRuleCreateCommand{
		EnvironmentID: environmentID, DayOfWeek: 2,
		StartTime:     time.Date(0, time.January, 1, 8, 0, 0, 0, time.UTC),
		EndTime:       time.Date(0, time.January, 1, 12, 0, 0, 0, time.UTC),
		EffectiveFrom: time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC), EffectiveUntil: &effectiveUntil,
	})
	if err != nil {
		t.Fatalf("failed to create availability rule: %v", err)
	}
	assertOutboxChange(t, ctx, db, environmentID, "RULE_ADDED", created.ID)

	rules, err := repo.List(ctx, environmentID, intPointer(2))
	if err != nil || len(rules) != 1 || rules[0].ID != created.ID {
		t.Fatalf("unexpected filtered list: rules=%+v err=%v", rules, err)
	}
	if _, err := repo.Get(ctx, otherEnvID, created.ID); !errors.Is(err, usecase.ErrAvailabilityRuleNotFound) {
		t.Fatalf("expected wrong-environment lookup to return 404 error, got %v", err)
	}

	updatedUntil := time.Date(2026, time.November, 30, 0, 0, 0, 0, time.UTC)
	updated, err := uc.Update(ctx, environmentID, created.ID, port.AvailabilityRuleUpdateCommand{
		EnvironmentID: environmentID, DayOfWeek: 3,
		StartTime:     time.Date(0, time.January, 1, 9, 0, 0, 0, time.UTC),
		EndTime:       time.Date(0, time.January, 1, 13, 0, 0, 0, time.UTC),
		EffectiveFrom: time.Date(2026, time.February, 1, 0, 0, 0, 0, time.UTC), EffectiveUntil: &updatedUntil,
	})
	if err != nil || updated.DayOfWeek != 3 {
		t.Fatalf("failed to update availability rule: rule=%+v err=%v", updated, err)
	}
	assertOutboxChange(t, ctx, db, environmentID, "RULE_MODIFIED", created.ID)

	beforeInvalid := outboxCount(t, ctx, db, environmentID)
	invalid := port.AvailabilityRuleUpdateCommand{
		EnvironmentID: environmentID, DayOfWeek: 8,
		StartTime:     time.Date(0, time.January, 1, 9, 0, 0, 0, time.UTC),
		EndTime:       time.Date(0, time.January, 1, 13, 0, 0, 0, time.UTC),
		EffectiveFrom: time.Date(2026, time.February, 1, 0, 0, 0, 0, time.UTC),
	}
	if _, err := uc.Update(ctx, environmentID, created.ID, invalid); !errors.Is(err, usecase.ErrAvailabilityValidation) {
		t.Fatalf("expected invalid update validation error, got %v", err)
	}
	if afterInvalid := outboxCount(t, ctx, db, environmentID); afterInvalid != beforeInvalid {
		t.Fatalf("invalid update created an outbox event: before=%d after=%d", beforeInvalid, afterInvalid)
	}

	if err := uc.Delete(ctx, environmentID, created.ID); err != nil {
		t.Fatalf("failed to physically delete availability rule: %v", err)
	}
	assertOutboxChange(t, ctx, db, environmentID, "RULE_REMOVED", created.ID)
	if _, err := repo.Get(ctx, environmentID, created.ID); !errors.Is(err, usecase.ErrAvailabilityRuleNotFound) {
		t.Fatalf("expected deleted rule to be absent, got %v", err)
	}
	var remaining int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM training_environment.availability_rule WHERE id = $1`, created.ID).Scan(&remaining); err != nil {
		t.Fatalf("failed to verify physical deletion: %v", err)
	}
	if remaining != 0 {
		t.Fatalf("expected physical deletion, found %d rows", remaining)
	}
}

func TestPostgresAvailabilityRuleRepository_HU005_Maximum24Rules(t *testing.T) {
	db := getTestPostgresDB(t)
	if db == nil {
		return
	}
	defer db.Close()

	ctx := context.Background()
	const (
		typeID        = "e1f2a3b4-c5d6-4789-9012-3456789abcde"
		environmentID = "e1f2a3b4-c5d6-4789-9012-3456789abc01"
	)

	_, err := db.ExecContext(ctx, `
		CREATE SCHEMA IF NOT EXISTS training_environment;
		CREATE TABLE IF NOT EXISTS training_environment.environment_type (
			id VARCHAR(36) PRIMARY KEY,
			code VARCHAR(30) UNIQUE NOT NULL,
			name VARCHAR(100) NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);
		CREATE TABLE IF NOT EXISTS training_environment.environment (
			id VARCHAR(36) PRIMARY KEY,
			environment_type_id VARCHAR(36) NOT NULL,
			training_center_id VARCHAR(36) NOT NULL,
			name VARCHAR(100) NOT NULL,
			capacity INT NOT NULL,
			is_active BOOLEAN NOT NULL DEFAULT TRUE,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);
		CREATE TABLE IF NOT EXISTS training_environment.availability_rule (
			id VARCHAR(36) PRIMARY KEY,
			environment_id VARCHAR(36) NOT NULL,
			day_of_week SMALLINT NOT NULL,
			start_time TIME NOT NULL,
			end_time TIME NOT NULL,
			effective_from DATE NOT NULL,
			effective_until DATE,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			CONSTRAINT chk_hu005_max_day CHECK (day_of_week BETWEEN 1 AND 7),
			CONSTRAINT chk_hu005_max_time CHECK (end_time > start_time)
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
	`)
	if err != nil {
		skipOrFailPostgres(t, "unable to prepare HU-ENV-005 maximum PostgreSQL tables: %v", err)
		return
	}

	cleanup := func() {
		_, _ = db.ExecContext(ctx, `DELETE FROM training_environment.outbox WHERE payload->'payload'->>'environment_id' = $1`, environmentID)
		_, _ = db.ExecContext(ctx, `DELETE FROM training_environment.availability_rule WHERE environment_id = $1`, environmentID)
		_, _ = db.ExecContext(ctx, `DELETE FROM training_environment.environment WHERE id = $1`, environmentID)
		_, _ = db.ExecContext(ctx, `DELETE FROM training_environment.environment_type WHERE id = $1`, typeID)
	}
	cleanup()
	t.Cleanup(cleanup)

	if _, err := db.ExecContext(ctx, `
		INSERT INTO training_environment.environment_type (id, code, name)
		VALUES ($1, 'HU005-MAX-IT', 'HU005 Maximum Integration Type')`, typeID); err != nil {
		t.Fatalf("failed to insert maximum-test environment type: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO training_environment.environment
		(id, environment_type_id, training_center_id, name, capacity)
		VALUES ($1, $2, 'c1d2e3f4-a5b6-7890-1234-56789abcdef0', 'HU005 Maximum Environment', 25)`, environmentID, typeID); err != nil {
		t.Fatalf("failed to insert maximum-test environment: %v", err)
	}

	repo := persistence.NewPostgresAvailabilityRuleRepository(db)
	uc := usecase.NewAvailabilityRuleUseCase(repo)
	for index := 0; index < 24; index++ {
		_, err := uc.Create(ctx, port.AvailabilityRuleCreateCommand{
			EnvironmentID: environmentID,
			DayOfWeek:     (index % 7) + 1,
			StartTime:     time.Date(0, time.January, 1, 8, 0, 0, 0, time.UTC),
			EndTime:       time.Date(0, time.January, 1, 12, 0, 0, 0, time.UTC),
			EffectiveFrom: time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC),
		})
		if err != nil {
			t.Fatalf("rule %d should be accepted within the 24-rule limit: %v", index+1, err)
		}
	}

	if _, err := uc.Create(ctx, port.AvailabilityRuleCreateCommand{
		EnvironmentID: environmentID,
		DayOfWeek:     1,
		StartTime:     time.Date(0, time.January, 1, 8, 0, 0, 0, time.UTC),
		EndTime:       time.Date(0, time.January, 1, 12, 0, 0, 0, time.UTC),
		EffectiveFrom: time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC),
	}); !errors.Is(err, usecase.ErrAvailabilityRuleLimit) {
		t.Fatalf("rule 25 should be rejected with the availability limit error, got %v", err)
	}

	count, err := repo.Count(ctx, environmentID)
	if err != nil {
		t.Fatalf("failed to count persisted availability rules: %v", err)
	}
	if count != model.MaxAvailabilityRules {
		t.Fatalf("expected exactly 24 persisted rules after rejecting rule 25, got %d", count)
	}
	if events := outboxCount(t, ctx, db, environmentID); events != model.MaxAvailabilityRules {
		t.Fatalf("expected 24 RULE_ADDED outbox events after rejecting rule 25, got %d", events)
	}
}

func assertOutboxChange(t *testing.T, ctx context.Context, db *sql.DB, environmentID, changeType, ruleID string) {
	t.Helper()
	var eventType string
	var raw []byte
	if err := db.QueryRowContext(ctx, `
		SELECT event_type, payload
		FROM training_environment.outbox
		WHERE event_type = 'environment.availability.changed'
		  AND payload->'payload'->>'environment_id' = $1
		  AND payload->'payload'->>'rule_id' = $2
		  AND payload->'payload'->>'change_type' = $3
		ORDER BY created_at DESC LIMIT 1`, environmentID, ruleID, changeType).Scan(&eventType, &raw); err != nil {
		t.Fatalf("expected %s outbox event: %v", changeType, err)
	}
	var envelope map[string]interface{}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatalf("invalid outbox event JSON: %v", err)
	}
	if eventType != "environment.availability.changed" || envelope["event_type"] != eventType {
		t.Fatalf("unexpected outbox envelope: type=%s payload=%s", eventType, string(raw))
	}
}

func outboxCount(t *testing.T, ctx context.Context, db *sql.DB, environmentID string) int {
	t.Helper()
	var count int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM training_environment.outbox WHERE payload->'payload'->>'environment_id' = $1`, environmentID).Scan(&count); err != nil {
		t.Fatalf("failed to count outbox events: %v", err)
	}
	return count
}

func intPointer(value int) *int { return &value }
