package persistence

import (
	"context"
	"database/sql"
	"errors"

	"github.com/lib/pq"

	"github.com/code-sena/design-software-training-environment-service/internal/application/port"
	"github.com/code-sena/design-software-training-environment-service/internal/application/usecase"
	"github.com/code-sena/design-software-training-environment-service/internal/domain/model"
)

type PostgresReservationRepository struct {
	db *sql.DB
}

func NewPostgresReservationRepository(db *sql.DB) port.ReservationRepository {
	return &PostgresReservationRepository{db: db}
}

func (r *PostgresReservationRepository) Create(ctx context.Context, reservation *model.Reservation) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO training_environment.reservation
		(id, environment_id, requester_id, reservation_date, start_time, end_time, reason, status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`,
		reservation.ID,
		reservation.EnvironmentID,
		reservation.RequesterID,
		reservation.ReservationDate.Format("2006-01-02"),
		reservation.StartTime,
		reservation.EndTime,
		reservation.Reason,
		reservation.Status,
		reservation.CreatedAt,
		reservation.UpdatedAt,
	)
	if err == nil {
		return nil
	}

	var postgresErr *pq.Error
	if errors.As(err, &postgresErr) {
		switch string(postgresErr.Code) {
		case "23P01": // exclusion_violation: excl_reservation_no_overlap
			return usecase.ErrReservationOverlap
		case "23503": // foreign_key_violation: environment_id
			return usecase.ErrReservationInvalidEnvironment
		case "23514": // check_violation: end_time > start_time / status
			return usecase.ErrReservationValidation
		case "22001": // string_data_right_truncation: reason
			return model.ErrReservationReasonTooLong
		}
	}
	return err
}
