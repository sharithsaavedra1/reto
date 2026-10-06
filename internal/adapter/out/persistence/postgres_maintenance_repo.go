package persistence

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/lib/pq"

	"github.com/code-sena/design-software-training-environment-service/internal/application/port"
	"github.com/code-sena/design-software-training-environment-service/internal/application/usecase"
	"github.com/code-sena/design-software-training-environment-service/internal/domain/model"
)

type PostgresMaintenanceRepository struct {
	db *sql.DB
}

type maintenanceStartedEvent struct {
	EventID       string                    `json:"event_id"`
	EventType     string                    `json:"event_type"`
	Version       string                    `json:"version"`
	Timestamp     time.Time                 `json:"timestamp"`
	SourceService string                    `json:"source_service"`
	CorrelationID string                    `json:"correlation_id"`
	Payload       maintenanceStartedPayload `json:"payload"`
}

type maintenanceStartedPayload struct {
	EnvironmentID string    `json:"environment_id"`
	MaintenanceID string    `json:"maintenance_id"`
	StartDate     string    `json:"start_date"`
	EndDate       string    `json:"end_date"`
	StartTime     string    `json:"start_time"`
	EndTime       string    `json:"end_time"`
	Description   string    `json:"description"`
	CreatedBy     string    `json:"created_by"`
	CreatedAt     time.Time `json:"created_at"`
}

func NewPostgresMaintenanceRepository(db *sql.DB) *PostgresMaintenanceRepository {
	return &PostgresMaintenanceRepository{db: db}
}

func (r *PostgresMaintenanceRepository) List(ctx context.Context, query port.MaintenanceListQuery) ([]model.Maintenance, error) {
	if err := r.ensureEnvironment(ctx, r.db, query.EnvironmentID); err != nil {
		return nil, err
	}
	where := []string{"environment_id = $1"}
	args := []interface{}{query.EnvironmentID}
	if query.From != nil {
		args = append(args, dateString(*query.From))
		where = append(where, fmt.Sprintf("(end_date + end_time) > $%d::date", len(args)))
	}
	if query.To != nil {
		args = append(args, dateString(*query.To))
		where = append(where, fmt.Sprintf("(start_date + start_time) < $%d::date", len(args)))
	}

	rows, err := r.db.QueryContext(ctx, `
		SELECT id, environment_id, start_date, end_date, start_time::text, end_time::text,
		       reason, created_by, created_at
		FROM training_environment.maintenance
		WHERE `+joinSQL(where, " AND ")+`
		ORDER BY start_date, start_time, id`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make([]model.Maintenance, 0)
	for rows.Next() {
		item, err := scanMaintenance(rows.Scan)
		if err != nil {
			return nil, err
		}
		result = append(result, *item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

func (r *PostgresMaintenanceRepository) Get(ctx context.Context, environmentID, maintenanceID string) (*model.Maintenance, error) {
	if err := r.ensureEnvironment(ctx, r.db, environmentID); err != nil {
		return nil, err
	}
	item, err := scanMaintenance(r.db.QueryRowContext(ctx, `
		SELECT id, environment_id, start_date, end_date, start_time::text, end_time::text,
		       reason, created_by, created_at
		FROM training_environment.maintenance
		WHERE id = $1 AND environment_id = $2`, maintenanceID, environmentID).Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, usecase.ErrMaintenanceNotFound
	}
	if err != nil {
		return nil, err
	}
	return item, nil
}

func (r *PostgresMaintenanceRepository) Create(ctx context.Context, maintenance *model.Maintenance) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if err := lockMaintenanceEnvironment(ctx, tx, maintenance.EnvironmentID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO training_environment.maintenance
		(id, environment_id, start_date, end_date, start_time, end_time, reason, created_by, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		maintenance.ID, maintenance.EnvironmentID, dateString(maintenance.StartDate), dateString(maintenance.EndDate),
		maintenance.StartTime, maintenance.EndTime, maintenance.Reason, maintenance.CreatedBy, maintenance.CreatedAt); err != nil {
		return mapMaintenancePersistenceError(err)
	}
	if err := insertMaintenanceEvent(ctx, tx, maintenance); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *PostgresMaintenanceRepository) ensureEnvironment(ctx context.Context, querier interface {
	QueryRowContext(context.Context, string, ...interface{}) *sql.Row
}, environmentID string) error {
	var exists bool
	if err := querier.QueryRowContext(ctx, `
		SELECT EXISTS (SELECT 1 FROM training_environment.environment WHERE id = $1)`, environmentID).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return usecase.ErrEnvironmentNotFound
	}
	return nil
}

func lockMaintenanceEnvironment(ctx context.Context, tx *sql.Tx, environmentID string) error {
	var id string
	if err := tx.QueryRowContext(ctx, `
		SELECT id FROM training_environment.environment WHERE id = $1 FOR UPDATE`, environmentID).Scan(&id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return usecase.ErrEnvironmentNotFound
		}
		return err
	}
	return nil
}

func scanMaintenance(scan func(...interface{}) error) (*model.Maintenance, error) {
	var item model.Maintenance
	var reason sql.NullString
	var startTimeRaw, endTimeRaw string
	if err := scan(&item.ID, &item.EnvironmentID, &item.StartDate, &item.EndDate,
		&startTimeRaw, &endTimeRaw, &reason, &item.CreatedBy, &item.CreatedAt); err != nil {
		return nil, err
	}
	var err error
	item.StartTime, err = model.NormalizeMaintenanceClock(startTimeRaw)
	if err != nil {
		return nil, err
	}
	item.EndTime, err = model.NormalizeMaintenanceClock(endTimeRaw)
	if err != nil {
		return nil, err
	}
	if reason.Valid {
		item.Reason = &reason.String
	}
	return &item, nil
}

func insertMaintenanceEvent(ctx context.Context, tx *sql.Tx, maintenance *model.Maintenance) error {
	eventID := newPersistenceUUID()
	event := maintenanceStartedEvent{
		EventID: eventID, EventType: "environment.maintenance.started", Version: "1.0",
		Timestamp: time.Now().UTC(), SourceService: "training-environment-service", CorrelationID: eventID,
		Payload: maintenanceStartedPayload{
			EnvironmentID: maintenance.EnvironmentID, MaintenanceID: maintenance.ID,
			StartDate: dateString(maintenance.StartDate), EndDate: dateString(maintenance.EndDate),
			StartTime: maintenance.StartTime, EndTime: maintenance.EndTime,
			Description: valueOrEmpty(maintenance.Reason), CreatedBy: maintenance.CreatedBy, CreatedAt: maintenance.CreatedAt,
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

func mapMaintenancePersistenceError(err error) error {
	var postgresErr *pq.Error
	if errors.As(err, &postgresErr) {
		switch string(postgresErr.Code) {
		case "23503":
			return usecase.ErrEnvironmentNotFound
		case "23514":
			return usecase.ErrMaintenanceValidation
		}
	}

	message := err.Error()
	if containsPostgresCode(message, "23503") || containsPostgresText(message, "fk_maintenance_environment") {
		return usecase.ErrEnvironmentNotFound
	}
	if containsPostgresCode(message, "23514") || containsPostgresText(message, "chk_maintenance_end_date") ||
		containsPostgresText(message, "chk_maintenance_date_range") ||
		containsPostgresText(message, "chk_maintenance_same_day_time_range") {
		return usecase.ErrMaintenanceValidation
	}
	return err
}

func joinSQL(parts []string, separator string) string {
	result := ""
	for index, part := range parts {
		if index > 0 {
			result += separator
		}
		result += part
	}
	return result
}
