package port

import (
	"context"

	"github.com/code-sena/design-software-training-environment-service/internal/domain/model"
)

type MaintenanceRepository interface {
	List(ctx context.Context, query MaintenanceListQuery) ([]model.Maintenance, error)
	Get(ctx context.Context, environmentID, maintenanceID string) (*model.Maintenance, error)
	Create(ctx context.Context, maintenance *model.Maintenance) error
}
