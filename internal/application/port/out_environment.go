package port

import (
	"context"

	"github.com/code-sena/design-software-training-environment-service/internal/domain/model"
)

// EnvironmentRepository defines the outbound port for environment persistence.
type EnvironmentRepository interface {
	Save(ctx context.Context, entity *model.Environment) error
	FindByID(ctx context.Context, id string) (*model.Environment, error)
	FindAll(ctx context.Context) ([]model.Environment, error)
}
