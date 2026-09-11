package port

import (
	"context"

	"github.com/code-sena/design-software-training-environment-service/internal/domain/model"
)

// EnvironmentTypeRepository defines the outbound port for environment type persistence.
type EnvironmentTypeRepository interface {
	FindAll(ctx context.Context) ([]model.EnvironmentType, error)
	FindByID(ctx context.Context, id string) (*model.EnvironmentType, error)
	FindByName(ctx context.Context, name string) (*model.EnvironmentType, error)
	FindByCode(ctx context.Context, code string) (*model.EnvironmentType, error)
	Save(ctx context.Context, entity *model.EnvironmentType) error
}
