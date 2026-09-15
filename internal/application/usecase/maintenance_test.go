package usecase

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/code-sena/design-software-training-environment-service/internal/application/port"
	"github.com/code-sena/design-software-training-environment-service/internal/domain/model"
)

type maintenanceRepositoryStub struct {
	items       []model.Maintenance
	created     *model.Maintenance
	createCalls int
}

func (r *maintenanceRepositoryStub) List(_ context.Context, _ port.MaintenanceListQuery) ([]model.Maintenance, error) {
	return r.items, nil
}

func (r *maintenanceRepositoryStub) Get(_ context.Context, _, _ string) (*model.Maintenance, error) {
	if len(r.items) == 0 {
		return nil, ErrMaintenanceNotFound
	}
	return &r.items[0], nil
}

func (r *maintenanceRepositoryStub) Create(_ context.Context, item *model.Maintenance) error {
	r.createCalls++
	r.created = item
	return nil
}

func TestMaintenanceUseCaseCreate(t *testing.T) {
	repo := &maintenanceRepositoryStub{}
	uc := NewMaintenanceUseCase(repo)
	reason := "Preventive maintenance"
	created, err := uc.Create(context.Background(), port.MaintenanceCreateCommand{
		EnvironmentID: "78901234-5678-9012-3456-789012345678",
		StartDate:     time.Date(2026, time.June, 1, 14, 30, 0, 0, time.UTC),
		EndDate:       time.Date(2026, time.June, 2, 9, 0, 0, 0, time.UTC),
		Reason:        &reason,
		CreatedBy:     "42c78701-2b34-4a6f-b034-3f7a3eca57d8",
	})
	if err != nil {
		t.Fatalf("valid maintenance should be created: %v", err)
	}
	if repo.createCalls != 1 || created.ID == "" || !created.StartDate.Equal(time.Date(2026, time.June, 1, 0, 0, 0, 0, time.UTC)) || !created.EndDate.Equal(time.Date(2026, time.June, 2, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("unexpected created maintenance: %+v calls=%d", created, repo.createCalls)
	}
	if repo.created.Reason == nil || *repo.created.Reason != reason {
		t.Fatalf("reason was not preserved: %+v", repo.created)
	}
}

func TestMaintenanceUseCaseRejectsInvalidRangeBeforePersistence(t *testing.T) {
	repo := &maintenanceRepositoryStub{}
	uc := NewMaintenanceUseCase(repo)
	_, err := uc.Create(context.Background(), port.MaintenanceCreateCommand{
		EnvironmentID: "78901234-5678-9012-3456-789012345678",
		StartDate:     time.Date(2026, time.June, 2, 0, 0, 0, 0, time.UTC),
		EndDate:       time.Date(2026, time.June, 2, 0, 0, 0, 0, time.UTC),
		CreatedBy:     "42c78701-2b34-4a6f-b034-3f7a3eca57d8",
	})
	if !errors.Is(err, ErrMaintenanceValidation) {
		t.Fatalf("expected maintenance validation error, got %v", err)
	}
	if repo.createCalls != 0 {
		t.Fatalf("invalid maintenance must not reach persistence, calls=%d", repo.createCalls)
	}
}

func TestMaintenanceUseCaseRejectsInvalidFilterRange(t *testing.T) {
	repo := &maintenanceRepositoryStub{}
	uc := NewMaintenanceUseCase(repo)
	from := time.Date(2026, time.June, 10, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, time.June, 1, 0, 0, 0, 0, time.UTC)
	_, err := uc.List(context.Background(), port.MaintenanceListQuery{
		EnvironmentID: "78901234-5678-9012-3456-789012345678",
		From:          &from,
		To:            &to,
	})
	if !errors.Is(err, ErrMaintenanceQuery) {
		t.Fatalf("expected maintenance query error, got %v", err)
	}
}
