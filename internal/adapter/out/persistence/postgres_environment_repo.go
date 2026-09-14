package persistence

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	_ "github.com/lib/pq"

	"github.com/code-sena/design-software-training-environment-service/internal/application/port"
	"github.com/code-sena/design-software-training-environment-service/internal/application/usecase"
	"github.com/code-sena/design-software-training-environment-service/internal/domain/model"
)

type PostgresEnvironmentRepository struct {
	db *sql.DB
}

// NewPostgresEnvironmentRepository creates a new PostgreSQL repository for environments.
func NewPostgresEnvironmentRepository(db *sql.DB) port.EnvironmentRepository {
	return &PostgresEnvironmentRepository{db: db}
}

func (r *PostgresEnvironmentRepository) Save(ctx context.Context, entity *model.Environment) error {
	query := `
		INSERT INTO training_environment.environment
		(id, environment_type_id, training_center_id, name, capacity, location, is_active, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`
	_, err := r.db.ExecContext(ctx, query,
		entity.ID,
		entity.EnvironmentTypeID,
		entity.TrainingCenterID,
		entity.Name,
		entity.Capacity,
		entity.Location,
		entity.IsActive,
		entity.CreatedAt,
		entity.UpdatedAt,
	)
	if err != nil {
		errMsg := strings.ToLower(err.Error())
		if strings.Contains(errMsg, "fk_environment_environment_type") || strings.Contains(errMsg, "23503") {
			return usecase.ErrInvalidEnvironmentType
		}
		if strings.Contains(errMsg, "chk_environment_capacity") || strings.Contains(errMsg, "23514") {
			return model.ErrInvalidCapacity
		}
		if strings.Contains(errMsg, "value too long") || strings.Contains(errMsg, "22001") || strings.Contains(errMsg, "string_data_right_truncation") {
			return model.ErrLocationTooLong
		}
		return err
	}
	return nil
}

func (r *PostgresEnvironmentRepository) Update(ctx context.Context, entity *model.Environment) error {
	query := `
		UPDATE training_environment.environment
		SET environment_type_id = $1,
			name = $2,
			capacity = $3,
			location = $4,
			is_active = $5,
			updated_at = $6
		WHERE id = $7
	`
	_, err := r.db.ExecContext(ctx, query,
		entity.EnvironmentTypeID,
		entity.Name,
		entity.Capacity,
		entity.Location,
		entity.IsActive,
		entity.UpdatedAt,
		entity.ID,
	)
	if err != nil {
		errMsg := strings.ToLower(err.Error())
		if strings.Contains(errMsg, "fk_environment_environment_type") || strings.Contains(errMsg, "23503") {
			return usecase.ErrInvalidEnvironmentType
		}
		if strings.Contains(errMsg, "chk_environment_capacity") || strings.Contains(errMsg, "23514") {
			return model.ErrInvalidCapacity
		}
		if strings.Contains(errMsg, "value too long") || strings.Contains(errMsg, "22001") || strings.Contains(errMsg, "string_data_right_truncation") {
			return model.ErrLocationTooLong
		}
		return err
	}
	return nil
}

func (r *PostgresEnvironmentRepository) Deactivate(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE training_environment.environment
		SET is_active = FALSE, updated_at = NOW()
		WHERE id = $1
	`, id)
	return err
}

func (r *PostgresEnvironmentRepository) FindByID(ctx context.Context, id string) (*model.Environment, error) {
	query := `
		SELECT id, environment_type_id, training_center_id, name, capacity, location, is_active, created_at, updated_at
		FROM training_environment.environment
		WHERE id = $1
	`
	row := r.db.QueryRowContext(ctx, query, id)

	var item model.Environment
	var loc sql.NullString
	if err := row.Scan(
		&item.ID,
		&item.EnvironmentTypeID,
		&item.TrainingCenterID,
		&item.Name,
		&item.Capacity,
		&loc,
		&item.IsActive,
		&item.CreatedAt,
		&item.UpdatedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	if loc.Valid {
		item.Location = &loc.String
	}
	return &item, nil
}

func (r *PostgresEnvironmentRepository) FindAll(ctx context.Context) ([]model.Environment, error) {
	query := `
		SELECT id, environment_type_id, training_center_id, name, capacity, location, is_active, created_at, updated_at
		FROM training_environment.environment
		ORDER BY created_at ASC
	`
	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []model.Environment
	for rows.Next() {
		var item model.Environment
		var loc sql.NullString
		if err := rows.Scan(
			&item.ID,
			&item.EnvironmentTypeID,
			&item.TrainingCenterID,
			&item.Name,
			&item.Capacity,
			&loc,
			&item.IsActive,
			&item.CreatedAt,
			&item.UpdatedAt,
		); err != nil {
			return nil, err
		}
		if loc.Valid {
			item.Location = &loc.String
		}
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if result == nil {
		result = []model.Environment{}
	}
	return result, nil
}

// Query returns environments using SQL-side filtering, pagination and
// availability predicates so PostgreSQL can use the documented indexes.
func (r *PostgresEnvironmentRepository) Query(ctx context.Context, query port.EnvironmentListQuery) (*port.EnvironmentPage, error) {
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
	args := make([]interface{}, 0, 6)
	addArg := func(clause string, value interface{}) {
		args = append(args, value)
		where = append(where, fmt.Sprintf(clause, len(args)))
	}

	if query.EnvironmentTypeID != "" {
		addArg("e.environment_type_id = $%d", query.EnvironmentTypeID)
	}
	if query.TrainingCenterID != "" {
		addArg("e.training_center_id = $%d", query.TrainingCenterID)
	}
	if query.IsActive != nil {
		addArg("e.is_active = $%d", *query.IsActive)
	}
	if query.AvailableDate != nil && query.AvailableStart != nil && query.AvailableEnd != nil {
		dateIndex := len(args) + 1
		startIndex := dateIndex + 1
		endIndex := dateIndex + 2
		args = append(args,
			query.AvailableDate.UTC().Format(time.DateOnly),
			query.AvailableStart.Format("15:04:05"),
			query.AvailableEnd.Format("15:04:05"),
		)
		where = append(where, fmt.Sprintf(`
			EXISTS (
				SELECT 1
				FROM training_environment.availability_rule ar
				WHERE ar.environment_id = e.id
				  AND ar.day_of_week = EXTRACT(ISODOW FROM $%d::date)::smallint
				  AND ar.start_time <= $%d::time
				  AND ar.end_time >= $%d::time
				  AND ar.effective_from <= $%d::date
				  AND (ar.effective_until IS NULL OR ar.effective_until >= $%d::date)
			)
			AND NOT EXISTS (
				SELECT 1
				FROM training_environment.maintenance m
				WHERE m.environment_id = e.id
				  AND m.start_date < ($%d::date + INTERVAL '1 day')
				  AND m.end_date > $%d::date
			)
			AND NOT EXISTS (
				SELECT 1
				FROM training_environment.reservation res
				WHERE res.environment_id = e.id
				  AND res.reservation_date = $%d::date
				  AND res.status <> 'CANCELLED'
				  AND res.start_time < $%d::time
				  AND res.end_time > $%d::time
			)`, dateIndex, startIndex, endIndex, dateIndex, dateIndex, dateIndex, dateIndex, dateIndex, endIndex, startIndex))
	}

	whereSQL := strings.Join(where, " AND ")
	countQuery := `SELECT COUNT(*) FROM training_environment.environment e WHERE ` + whereSQL
	var totalItems int
	if err := r.db.QueryRowContext(ctx, countQuery, args...).Scan(&totalItems); err != nil {
		return nil, err
	}

	sortColumns := map[string]string{
		"id":         "e.id",
		"name":       "e.name",
		"capacity":   "e.capacity",
		"created_at": "e.created_at",
		"updated_at": "e.updated_at",
		"is_active":  "e.is_active",
	}
	sortColumn := sortColumns[query.SortField]
	if sortColumn == "" {
		sortColumn = sortColumns["created_at"]
	}
	direction := "ASC"
	if query.SortDescending {
		direction = "DESC"
	}

	dataQuery := `
		SELECT e.id, e.environment_type_id, e.training_center_id, e.name, e.capacity, e.location, e.is_active, e.created_at, e.updated_at
		FROM training_environment.environment e
		WHERE ` + whereSQL + `
		ORDER BY ` + sortColumn + ` ` + direction + `, e.id ASC
		LIMIT $` + strconv.Itoa(len(args)+1) + ` OFFSET $` + strconv.Itoa(len(args)+2)
	dataArgs := append(append([]interface{}{}, args...), query.PageSize, (query.Page-1)*query.PageSize)
	rows, err := r.db.QueryContext(ctx, dataQuery, dataArgs...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]model.Environment, 0)
	for rows.Next() {
		var item model.Environment
		var loc sql.NullString
		if err := rows.Scan(&item.ID, &item.EnvironmentTypeID, &item.TrainingCenterID, &item.Name, &item.Capacity, &loc, &item.IsActive, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		if loc.Valid {
			item.Location = &loc.String
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	totalPages := 0
	if totalItems > 0 {
		totalPages = (totalItems + query.PageSize - 1) / query.PageSize
	}
	return &port.EnvironmentPage{Items: items, Page: query.Page, PageSize: query.PageSize, TotalItems: totalItems, TotalPages: totalPages}, nil
}
