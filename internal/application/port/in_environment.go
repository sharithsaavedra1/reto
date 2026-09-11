package port

import (
	"context"

	"github.com/code-sena/design-software-training-environment-service/internal/domain/model"
)

// CreateEnvironmentCommand carries the parameters needed to create a new Environment.
type CreateEnvironmentCommand struct {
	EnvironmentTypeID string  `json:"environment_type_id"`
	TrainingCenterID  string  `json:"training_center_id"`
	Name              string  `json:"name"`
	Capacity          int     `json:"capacity"`
	Location          *string `json:"location,omitempty"`
}

// EnvironmentUseCase defines the inbound port for environment use case operations.
type EnvironmentUseCase interface {
	Create(ctx context.Context, cmd CreateEnvironmentCommand) (*model.Environment, error)
	GetByID(ctx context.Context, id string) (*model.Environment, error)
	List(ctx context.Context) ([]model.Environment, error)
}
