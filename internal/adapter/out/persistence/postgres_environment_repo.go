package persistence

import (
	"context"
	"database/sql"
	"errors"
	"strings"

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
		return err
	}
	return nil
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
