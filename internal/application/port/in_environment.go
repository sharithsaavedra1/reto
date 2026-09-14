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

// UpdateEnvironmentCommand contains only fields that may be edited on an environment.
// Pointer fields preserve the distinction between an omitted field and a zero value.
type UpdateEnvironmentCommand struct {
	EnvironmentTypeID *string
	Name              *string
	Capacity          *int
	Location          *string
	LocationSet       bool
	IsActive          *bool
}

// EnvironmentUseCase defines the inbound port for environment use case operations.
type EnvironmentUseCase interface {
	Create(ctx context.Context, cmd CreateEnvironmentCommand) (*model.Environment, error)
	Update(ctx context.Context, id string, cmd UpdateEnvironmentCommand) (*model.Environment, error)
	Deactivate(ctx context.Context, id string) error
	GetByID(ctx context.Context, id string) (*model.Environment, error)
	List(ctx context.Context) ([]model.Environment, error)
	Query(ctx context.Context, query EnvironmentListQuery) (*EnvironmentPage, error)
}
