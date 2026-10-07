package port

import (
	"context"
	"time"

	"github.com/code-sena/design-software-training-environment-service/internal/domain/model"
)

type MaintenanceListQuery struct {
	EnvironmentID string
	From          *time.Time
	To            *time.Time
}

type MaintenanceCreateCommand struct {
	EnvironmentID string
	StartDate     time.Time
	EndDate       time.Time
	StartTime     string
	EndTime       string
	Reason        *string
	CreatedBy     string
}

type MaintenanceUseCase interface {
	List(ctx context.Context, query MaintenanceListQuery) ([]model.Maintenance, error)
	Get(ctx context.Context, environmentID, maintenanceID string) (*model.Maintenance, error)
	Create(ctx context.Context, command MaintenanceCreateCommand) (*model.Maintenance, error)
}
