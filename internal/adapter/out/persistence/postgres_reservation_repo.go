package persistence

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"

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
		case "23P01":
			return usecase.ErrReservationOverlap
		case "23503":
			return usecase.ErrReservationInvalidEnvironment
		case "23514":
			return usecase.ErrReservationValidation
		case "22001":
			return model.ErrReservationReasonTooLong
		}
	}
	return err
}

func (r *PostgresReservationRepository) Update(ctx context.Context, reservation *model.Reservation) error {
	result, err := r.db.ExecContext(ctx, `
        UPDATE training_environment.reservation
        SET reservation_date = $1,
            start_time = $2,
            end_time = $3,
            reason = $4,
            status = $5,
            updated_at = $6
        WHERE id = $7
    `,
		reservation.ReservationDate.Format("2006-01-02"),
		reservation.StartTime,
		reservation.EndTime,
		reservation.Reason,
		reservation.Status,
		reservation.UpdatedAt,
		reservation.ID,
	)
	if err != nil {
		var postgresErr *pq.Error
		if errors.As(err, &postgresErr) {
			switch string(postgresErr.Code) {
			case "23P01":
				return usecase.ErrReservationOverlap
			case "23503":
				return usecase.ErrReservationInvalidEnvironment
			case "23514":
				return usecase.ErrReservationValidation
			case "22001":
				return model.ErrReservationReasonTooLong
			}
		}
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err == nil && rowsAffected == 0 {
		return usecase.ErrReservationNotFound
	}
	return nil
}

func (r *PostgresReservationRepository) FindByID(ctx context.Context, id, trainingCenterID string) (*model.Reservation, error) {
	args := []interface{}{id}
	where := "r.id = $1"
	if trainingCenterID != "" {
		args = append(args, trainingCenterID)
		where += " AND e.training_center_id = $2"
	}
	item, err := scanReservation(r.db.QueryRowContext(ctx, `
        SELECT r.id, r.environment_id, r.reservation_date, r.start_time::text, r.end_time::text,
               r.reason, r.requester_id, r.status, r.created_at, r.updated_at
        FROM training_environment.reservation r
        JOIN training_environment.environment e ON e.id = r.environment_id
        WHERE `+where, args...).Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return item, nil
}

func (r *PostgresReservationRepository) Query(ctx context.Context, query port.ReservationListQuery) (*port.ReservationPage, error) {
	if query.Page == 0 {
		query.Page = 1
	}
	if query.PageSize == 0 {
		query.PageSize = 20
	}
	if query.Page < 1 || query.PageSize < 1 || query.PageSize > 100 {
		return nil, errors.New("invalid pagination")
	}

	where := []string{"1=1"}
	args := make([]interface{}, 0, 7)
	add := func(template string, value interface{}) {
		args = append(args, value)
		where = append(where, fmt.Sprintf(template, len(args)))
	}
	if query.EnvironmentID != "" {
		add("r.environment_id = $%d", query.EnvironmentID)
	}
	if query.RequesterID != "" {
		add("r.requester_id = $%d", query.RequesterID)
	}
	if query.Status != "" {
		add("r.status = $%d", query.Status)
	}
	if query.From != nil {
		add("r.reservation_date >= $%d::date", query.From.UTC().Format("2006-01-02"))
	}
	if query.To != nil {
		add("r.reservation_date < $%d::date", query.To.UTC().Format("2006-01-02"))
	}
	if query.TrainingCenterID != "" {
		add("e.training_center_id = $%d", query.TrainingCenterID)
	}
	whereSQL := strings.Join(where, " AND ")

	var total int
	if err := r.db.QueryRowContext(ctx, `
        SELECT COUNT(*)
        FROM training_environment.reservation r
        JOIN training_environment.environment e ON e.id = r.environment_id
        WHERE `+whereSQL, args...).Scan(&total); err != nil {
		return nil, err
	}

	sortColumns := map[string]string{
		"id": "r.id", "environment_id": "r.environment_id", "requester_id": "r.requester_id",
		"reservation_date": "r.reservation_date", "start_time": "r.start_time", "end_time": "r.end_time",
		"status": "r.status", "created_at": "r.created_at", "updated_at": "r.updated_at",
	}
	sortColumn := sortColumns[query.SortField]
	if sortColumn == "" {
		sortColumn = "r.created_at"
	}
	direction := "ASC"
	if query.SortDescending {
		direction = "DESC"
	}

	dataQuery := `
        SELECT r.id, r.environment_id, r.reservation_date, r.start_time::text, r.end_time::text,
               r.reason, r.requester_id, r.status, r.created_at, r.updated_at
        FROM training_environment.reservation r
        JOIN training_environment.environment e ON e.id = r.environment_id
        WHERE ` + whereSQL + `
        ORDER BY ` + sortColumn + ` ` + direction + `, r.id ASC
        LIMIT $` + strconv.Itoa(len(args)+1) + ` OFFSET $` + strconv.Itoa(len(args)+2)
	dataArgs := append(append([]interface{}{}, args...), query.PageSize, (query.Page-1)*query.PageSize)
	rows, err := r.db.QueryContext(ctx, dataQuery, dataArgs...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]model.Reservation, 0)
	for rows.Next() {
		item, err := scanReservation(rows.Scan)
		if err != nil {
			return nil, err
		}
		items = append(items, *item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	totalPages := 0
	if total > 0 {
		totalPages = (total + query.PageSize - 1) / query.PageSize
	}
	return &port.ReservationPage{Items: items, Page: query.Page, PageSize: query.PageSize, TotalItems: total, TotalPages: totalPages}, nil
}

func scanReservation(scan func(...interface{}) error) (*model.Reservation, error) {
	var item model.Reservation
	var reason sql.NullString
	if err := scan(&item.ID, &item.EnvironmentID, &item.ReservationDate, &item.StartTime, &item.EndTime,
		&reason, &item.RequesterID, &item.Status, &item.CreatedAt, &item.UpdatedAt); err != nil {
		return nil, err
	}
	if reason.Valid {
		item.Reason = &reason.String
	}
	return &item, nil
}
