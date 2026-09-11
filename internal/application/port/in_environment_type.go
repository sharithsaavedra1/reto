package port

import (
	"context"

	"github.com/code-sena/design-software-training-environment-service/internal/domain/model"
)

// CreateEnvironmentTypeCommand carries data required to create an EnvironmentType.
type CreateEnvironmentTypeCommand struct {
	Code        string  `json:"code"`
	Name        string  `json:"name"`
	Description *string `json:"description,omitempty"`
}

// EnvironmentTypeUseCase defines the inbound port for environment type operations.
type EnvironmentTypeUseCase interface {
	List(ctx context.Context) ([]model.EnvironmentType, error)
	GetByID(ctx context.Context, id string) (*model.EnvironmentType, error)
	Create(ctx context.Context, cmd CreateEnvironmentTypeCommand) (*model.EnvironmentType, error)
}
