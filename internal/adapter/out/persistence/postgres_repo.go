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

type PostgresEnvironmentTypeRepository struct {
	db *sql.DB
}

// NewPostgresEnvironmentTypeRepository creates a repository for PostgreSQL.
func NewPostgresEnvironmentTypeRepository(db *sql.DB) port.EnvironmentTypeRepository {
	return &PostgresEnvironmentTypeRepository{db: db}
}

func (r *PostgresEnvironmentTypeRepository) FindAll(ctx context.Context) ([]model.EnvironmentType, error) {
	query := `
		SELECT id, code, name, description, created_at
		FROM training_environment.environment_type
		ORDER BY created_at ASC
	`
	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []model.EnvironmentType
	for rows.Next() {
		var item model.EnvironmentType
		var desc sql.NullString
		if err := rows.Scan(&item.ID, &item.Code, &item.Name, &desc, &item.CreatedAt); err != nil {
			return nil, err
		}
		if desc.Valid {
			item.Description = &desc.String
		}
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if result == nil {
		result = []model.EnvironmentType{}
	}
	return result, nil
}

func (r *PostgresEnvironmentTypeRepository) FindByID(ctx context.Context, id string) (*model.EnvironmentType, error) {
	query := `
		SELECT id, code, name, description, created_at
		FROM training_environment.environment_type
		WHERE id = $1
	`
	row := r.db.QueryRowContext(ctx, query, id)

	var item model.EnvironmentType
	var desc sql.NullString
	if err := row.Scan(&item.ID, &item.Code, &item.Name, &desc, &item.CreatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	if desc.Valid {
		item.Description = &desc.String
	}
	return &item, nil
}

func (r *PostgresEnvironmentTypeRepository) FindByName(ctx context.Context, name string) (*model.EnvironmentType, error) {
	query := `
		SELECT id, code, name, description, created_at
		FROM training_environment.environment_type
		WHERE LOWER(name) = LOWER($1)
	`
	row := r.db.QueryRowContext(ctx, query, strings.TrimSpace(name))

	var item model.EnvironmentType
	var desc sql.NullString
	if err := row.Scan(&item.ID, &item.Code, &item.Name, &desc, &item.CreatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	if desc.Valid {
		item.Description = &desc.String
	}
	return &item, nil
}

func (r *PostgresEnvironmentTypeRepository) FindByCode(ctx context.Context, code string) (*model.EnvironmentType, error) {
	query := `
		SELECT id, code, name, description, created_at
		FROM training_environment.environment_type
		WHERE LOWER(code) = LOWER($1)
	`
	row := r.db.QueryRowContext(ctx, query, strings.TrimSpace(code))

	var item model.EnvironmentType
	var desc sql.NullString
	if err := row.Scan(&item.ID, &item.Code, &item.Name, &desc, &item.CreatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	if desc.Valid {
		item.Description = &desc.String
	}
	return &item, nil
}

func (r *PostgresEnvironmentTypeRepository) Save(ctx context.Context, entity *model.EnvironmentType) error {
	query := `
		INSERT INTO training_environment.environment_type (id, code, name, description, created_at)
		VALUES ($1, $2, $3, $4, $5)
	`
	_, err := r.db.ExecContext(ctx, query, entity.ID, entity.Code, entity.Name, entity.Description, entity.CreatedAt)
	if err != nil {
		errMsg := strings.ToLower(err.Error())
		if strings.Contains(errMsg, "unique") || strings.Contains(errMsg, "23505") || strings.Contains(errMsg, "uq_environment_type_code") || strings.Contains(errMsg, "uq_environment_type_name") {
			if strings.Contains(errMsg, "code") || strings.Contains(errMsg, "uq_environment_type_code") {
				return usecase.ErrConflictCode
			}
			return usecase.ErrConflictName
		}
		return err
	}
	return nil
}
