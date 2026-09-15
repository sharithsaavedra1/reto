package persistence

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	_ "github.com/lib/pq"

	"github.com/code-sena/design-software-training-environment-service/internal/application/usecase"
	"github.com/code-sena/design-software-training-environment-service/internal/domain/model"
)

type PostgresAvailabilityRuleRepository struct {
	db *sql.DB
}

type availabilityChangedEvent struct {
	EventID       string                       `json:"event_id"`
	EventType     string                       `json:"event_type"`
	Version       string                       `json:"version"`
	Timestamp     time.Time                    `json:"timestamp"`
	SourceService string                       `json:"source_service"`
	CorrelationID string                       `json:"correlation_id"`
	Payload       availabilityChangedEventData `json:"payload"`
}

type availabilityChangedEventData struct {
	EnvironmentID  string    `json:"environment_id"`
	RuleID         string    `json:"rule_id"`
	ChangeType     string    `json:"change_type"`
	AffectedDays   []int     `json:"affected_days"`
	EffectiveFrom  string    `json:"effective_from"`
	EffectiveUntil *string   `json:"effective_until,omitempty"`
	StartTime      string    `json:"start_time"`
	EndTime        string    `json:"end_time"`
	ChangedAt      time.Time `json:"changed_at"`
}

func NewPostgresAvailabilityRuleRepository(db *sql.DB) *PostgresAvailabilityRuleRepository {
	return &PostgresAvailabilityRuleRepository{db: db}
}

func (r *PostgresAvailabilityRuleRepository) List(ctx context.Context, environmentID string, dayOfWeek *int) ([]model.AvailabilityRule, error) {
	if err := r.ensureEnvironment(ctx, r.db, environmentID); err != nil {
		return nil, err
	}
	query := `
		SELECT id, environment_id, day_of_week, start_time, end_time,
		       effective_from, effective_until, created_at
		FROM training_environment.availability_rule
		WHERE environment_id = $1`
	args := []interface{}{environmentID}
	if dayOfWeek != nil {
		query += " AND day_of_week = $2"
		args = append(args, *dayOfWeek)
	}
	query += " ORDER BY day_of_week, start_time, id"

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make([]model.AvailabilityRule, 0)
	for rows.Next() {
		rule, err := scanAvailabilityRule(rows.Scan)
		if err != nil {
			return nil, err
		}
		result = append(result, *rule)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

func (r *PostgresAvailabilityRuleRepository) Get(ctx context.Context, environmentID, ruleID string) (*model.AvailabilityRule, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, environment_id, day_of_week, start_time, end_time,
		       effective_from, effective_until, created_at
		FROM training_environment.availability_rule
		WHERE id = $1 AND environment_id = $2`, ruleID, environmentID)
	rule, err := scanAvailabilityRule(row.Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, usecase.ErrAvailabilityRuleNotFound
	}
	if err != nil {
		return nil, err
	}
	return rule, nil
}

func (r *PostgresAvailabilityRuleRepository) Count(ctx context.Context, environmentID string) (int, error) {
	if err := r.ensureEnvironment(ctx, r.db, environmentID); err != nil {
		return 0, err
	}
	var count int
	if err := r.db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM training_environment.availability_rule
		WHERE environment_id = $1`, environmentID).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func (r *PostgresAvailabilityRuleRepository) Create(ctx context.Context, rule *model.AvailabilityRule) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if err := r.lockEnvironment(ctx, tx, rule.EnvironmentID); err != nil {
		return err
	}
	var count int
	if err := tx.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM training_environment.availability_rule
		WHERE environment_id = $1`, rule.EnvironmentID).Scan(&count); err != nil {
		return err
	}
	if count >= model.MaxAvailabilityRules {
		return usecase.ErrAvailabilityRuleLimit
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO training_environment.availability_rule
		(id, environment_id, day_of_week, start_time, end_time, effective_from, effective_until, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		rule.ID, rule.EnvironmentID, rule.DayOfWeek, clockString(rule.StartTime), clockString(rule.EndTime),
		dateString(rule.EffectiveFrom), nullableDateString(rule.EffectiveUntil), rule.CreatedAt); err != nil {
		return mapAvailabilityPersistenceError(err)
	}
	if err := insertAvailabilityEvent(ctx, tx, rule, "RULE_ADDED"); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *PostgresAvailabilityRuleRepository) Update(ctx context.Context, rule *model.AvailabilityRule) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	result, err := tx.ExecContext(ctx, `
		UPDATE training_environment.availability_rule
		SET day_of_week = $1, start_time = $2, end_time = $3,
		    effective_from = $4, effective_until = $5
		WHERE id = $6 AND environment_id = $7`,
		rule.DayOfWeek, clockString(rule.StartTime), clockString(rule.EndTime),
		dateString(rule.EffectiveFrom), nullableDateString(rule.EffectiveUntil), rule.ID, rule.EnvironmentID)
	if err != nil {
		return mapAvailabilityPersistenceError(err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return usecase.ErrAvailabilityRuleNotFound
	}
	if err := insertAvailabilityEvent(ctx, tx, rule, "RULE_MODIFIED"); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *PostgresAvailabilityRuleRepository) Delete(ctx context.Context, environmentID, ruleID string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var rule model.AvailabilityRule
	var startRaw, endRaw string
	var effectiveUntil sql.NullTime
	if err := tx.QueryRowContext(ctx, `
		SELECT id, environment_id, day_of_week, start_time, end_time,
		       effective_from, effective_until, created_at
		FROM training_environment.availability_rule
		WHERE id = $1 AND environment_id = $2
		FOR UPDATE`, ruleID, environmentID).Scan(
		&rule.ID, &rule.EnvironmentID, &rule.DayOfWeek, &startRaw, &endRaw,
		&rule.EffectiveFrom, &effectiveUntil, &rule.CreatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return usecase.ErrAvailabilityRuleNotFound
		}
		return err
	}
	var parseErr error
	rule.StartTime, parseErr = parseClock(startRaw)
	if parseErr != nil {
		return parseErr
	}
	rule.EndTime, parseErr = parseClock(endRaw)
	if parseErr != nil {
		return parseErr
	}
	if effectiveUntil.Valid {
		value := effectiveUntil.Time
		rule.EffectiveUntil = &value
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM training_environment.availability_rule WHERE id = $1 AND environment_id = $2`, ruleID, environmentID); err != nil {
		return err
	}
	if err := insertAvailabilityEvent(ctx, tx, &rule, "RULE_REMOVED"); err != nil {
		return err
	}
	return tx.Commit()
}

type rowScanner func(dest ...interface{}) error

func scanAvailabilityRule(scan rowScanner) (*model.AvailabilityRule, error) {
	var rule model.AvailabilityRule
	var startRaw, endRaw string
	var effectiveUntil sql.NullTime
	if err := scan(&rule.ID, &rule.EnvironmentID, &rule.DayOfWeek, &startRaw, &endRaw,
		&rule.EffectiveFrom, &effectiveUntil, &rule.CreatedAt); err != nil {
		return nil, err
	}
	var err error
	rule.StartTime, err = parseClock(startRaw)
	if err != nil {
		return nil, err
	}
	rule.EndTime, err = parseClock(endRaw)
	if err != nil {
		return nil, err
	}
	if effectiveUntil.Valid {
		value := effectiveUntil.Time
		rule.EffectiveUntil = &value
	}
	return &rule, nil
}

func (r *PostgresAvailabilityRuleRepository) ensureEnvironment(ctx context.Context, querier interface {
	QueryRowContext(context.Context, string, ...interface{}) *sql.Row
}, environmentID string) error {
	var exists bool
	if err := querier.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM training_environment.environment WHERE id = $1)`, environmentID).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return usecase.ErrEnvironmentNotFound
	}
	return nil
}

func (r *PostgresAvailabilityRuleRepository) lockEnvironment(ctx context.Context, tx *sql.Tx, environmentID string) error {
	var id string
	if err := tx.QueryRowContext(ctx, `
		SELECT id
		FROM training_environment.environment
		WHERE id = $1
		FOR UPDATE`, environmentID).Scan(&id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return usecase.ErrEnvironmentNotFound
		}
		return err
	}
	return nil
}

func insertAvailabilityEvent(ctx context.Context, tx *sql.Tx, rule *model.AvailabilityRule, changeType string) error {
	eventID := newPersistenceUUID()
	effectiveUntil := (*string)(nil)
	if rule.EffectiveUntil != nil {
		value := dateString(*rule.EffectiveUntil)
		effectiveUntil = &value
	}
	event := availabilityChangedEvent{
		EventID: eventID, EventType: "environment.availability.changed", Version: "1.0",
		Timestamp: time.Now().UTC(), SourceService: "training-environment-service", CorrelationID: eventID,
		Payload: availabilityChangedEventData{
			EnvironmentID: rule.EnvironmentID, RuleID: rule.ID, ChangeType: changeType,
			AffectedDays: []int{rule.DayOfWeek}, EffectiveFrom: dateString(rule.EffectiveFrom), EffectiveUntil: effectiveUntil,
			StartTime: clockString(rule.StartTime), EndTime: clockString(rule.EndTime), ChangedAt: time.Now().UTC(),
		},
	}
	payload, err := json.Marshal(event)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO training_environment.outbox (event_id, event_type, payload, status)
		VALUES ($1, $2, $3, 'PENDING')`, eventID, event.EventType, payload)
	return err
}

func mapAvailabilityPersistenceError(err error) error {
	message := err.Error()
	if containsPostgresCode(message, "23503") || containsPostgresText(message, "fk_availability_rule_environment") {
		return usecase.ErrEnvironmentNotFound
	}
	if containsPostgresCode(message, "23514") {
		return model.ErrInvalidAvailabilityDay
	}
	return err
}

func containsPostgresCode(message, code string) bool { return containsPostgresText(message, code) }

func containsPostgresText(message, value string) bool {
	return len(message) >= len(value) && stringContains(message, value)
}

func stringContains(value, target string) bool {
	for i := 0; i+len(target) <= len(value); i++ {
		if value[i:i+len(target)] == target {
			return true
		}
	}
	return false
}

func parseClock(value string) (time.Time, error) {
	for _, layout := range []string{time.RFC3339Nano, "15:04:05.999999999", "15:04:05"} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed, nil
		}
	}
	return time.Time{}, fmt.Errorf("invalid PostgreSQL time value %q", value)
}

func clockString(value time.Time) string { return value.Format("15:04:05") }

func dateString(value time.Time) string { return value.UTC().Format("2006-01-02") }

func nullableDateString(value *time.Time) interface{} {
	if value == nil {
		return nil
	}
	return dateString(*value)
}

func newPersistenceUUID() string {
	var buf [16]byte
	_, _ = rand.Read(buf[:])
	buf[6] = (buf[6] & 0x0f) | 0x40
	buf[8] = (buf[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", buf[0:4], buf[4:6], buf[6:8], buf[8:10], buf[10:16])
}
