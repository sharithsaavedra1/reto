package usecase

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/code-sena/design-software-training-environment-service/internal/application/port"
	"github.com/code-sena/design-software-training-environment-service/internal/domain/model"
)

var (
	ErrMaintenanceNotFound   = errors.New("maintenance period not found")
	ErrMaintenanceValidation = errors.New("invalid maintenance period")
	ErrMaintenanceQuery      = errors.New("invalid maintenance query")
)

type maintenanceUseCase struct {
	repo port.MaintenanceRepository
}

func NewMaintenanceUseCase(repo port.MaintenanceRepository) port.MaintenanceUseCase {
	return &maintenanceUseCase{repo: repo}
}

func (u *maintenanceUseCase) List(ctx context.Context, query port.MaintenanceListQuery) ([]model.Maintenance, error) {
	if !model.IsValidUUID(query.EnvironmentID) {
		return nil, ErrMaintenanceNotFound
	}
	if query.From != nil && query.To != nil && !query.To.After(*query.From) {
		return nil, ErrMaintenanceQuery
	}
	return u.repo.List(ctx, query)
}

func (u *maintenanceUseCase) Get(ctx context.Context, environmentID, maintenanceID string) (*model.Maintenance, error) {
	if !model.IsValidUUID(environmentID) || !model.IsValidUUID(maintenanceID) {
		return nil, ErrMaintenanceNotFound
	}
	return u.repo.Get(ctx, environmentID, maintenanceID)
}

func (u *maintenanceUseCase) Create(ctx context.Context, command port.MaintenanceCreateCommand) (*model.Maintenance, error) {
	maintenance := &model.Maintenance{
		ID:            newUUID(),
		EnvironmentID: command.EnvironmentID,
		StartDate:     command.StartDate,
		EndDate:       command.EndDate,
		Reason:        command.Reason,
		CreatedBy:     command.CreatedBy,
		CreatedAt:     time.Now().UTC(),
	}
	if err := maintenance.Validate(); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrMaintenanceValidation, err)
	}
	maintenance.StartDate = dateOnly(maintenance.StartDate)
	maintenance.EndDate = dateOnly(maintenance.EndDate)
	if err := u.repo.Create(ctx, maintenance); err != nil {
		return nil, err
	}
	return maintenance, nil
}

func dateOnly(value time.Time) time.Time {
	return time.Date(value.UTC().Year(), value.UTC().Month(), value.UTC().Day(), 0, 0, 0, 0, time.UTC)
}
