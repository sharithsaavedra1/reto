package port

import (
	"context"
	"time"

	"github.com/code-sena/design-software-training-environment-service/internal/domain/model"
)

// EnvironmentListQuery contains the supported filters and offset pagination
// parameters for GET /training-environments.
type EnvironmentListQuery struct {
	Page              int
	PageSize          int
	SortField         string
	SortDescending    bool
	EnvironmentTypeID string
	TrainingCenterID  string
	IsActive          *bool
	AvailableDate     *time.Time
	AvailableStart    *time.Time
	AvailableEnd      *time.Time
}

// EnvironmentPage is the paginated result returned by the environment query.
type EnvironmentPage struct {
	Items      []model.Environment
	Page       int
	PageSize   int
	TotalItems int
	TotalPages int
}

// EnvironmentRepository defines the outbound port for environment persistence.
type EnvironmentRepository interface {
	Save(ctx context.Context, entity *model.Environment) error
	FindByID(ctx context.Context, id string) (*model.Environment, error)
	FindAll(ctx context.Context) ([]model.Environment, error)
	Query(ctx context.Context, query EnvironmentListQuery) (*EnvironmentPage, error)
}
