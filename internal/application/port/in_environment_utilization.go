package port

import (
	"context"
	"time"

	"github.com/code-sena/design-software-training-environment-service/internal/domain/model"
)

type EnvironmentUtilizationQuery struct {
	Page              int
	PageSize          int
	From              time.Time
	To                time.Time
	TrainingCenterID  string
	EnvironmentID     string
	EnvironmentTypeID string
}

type EnvironmentUtilizationPage struct {
	Items      []model.EnvironmentUtilization
	Page       int
	PageSize   int
	TotalItems int
	TotalPages int
}

type EnvironmentUtilizationUseCase interface {
	Report(ctx context.Context, query EnvironmentUtilizationQuery) (*EnvironmentUtilizationPage, error)
}
