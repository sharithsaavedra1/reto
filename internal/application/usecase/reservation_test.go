package usecase

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/code-sena/design-software-training-environment-service/internal/application/port"
	"github.com/code-sena/design-software-training-environment-service/internal/domain/model"
)

type reservationRepoStub struct {
	created *model.Reservation
	err     error
}

func (r *reservationRepoStub) Create(_ context.Context, reservation *model.Reservation) error {
	if r.err != nil {
		return r.err
	}
	copy := *reservation
	r.created = &copy
	return nil
}

type reservationEnvironmentRepoStub struct {
	items map[string]model.Environment
}

func (r *reservationEnvironmentRepoStub) Save(context.Context, *model.Environment) error { return nil }
func (r *reservationEnvironmentRepoStub) Update(context.Context, *model.Environment) error {
	return nil
}
func (r *reservationEnvironmentRepoStub) Deactivate(context.Context, string) error { return nil }
func (r *reservationEnvironmentRepoStub) FindAll(context.Context) ([]model.Environment, error) {
	return nil, nil
}
func (r *reservationEnvironmentRepoStub) Query(context.Context, port.EnvironmentListQuery) (*port.EnvironmentPage, error) {
	return nil, nil
}
func (r *reservationEnvironmentRepoStub) FindByID(_ context.Context, id string) (*model.Environment, error) {
	item, ok := r.items[id]
	if !ok {
		return nil, nil
	}
	return &item, nil
}

func TestReservationUseCase_HU007_CreatePendingReservation(t *testing.T) {
	const envID = "6f674be8-24a4-4759-9226-9b9574ba9643"
	repo := &reservationRepoStub{}
	envRepo := &reservationEnvironmentRepoStub{items: map[string]model.Environment{envID: {ID: envID}}}
	uc := NewReservationUseCase(repo, envRepo)

	reason := "Reunión de coordinación"
	created, err := uc.Create(context.Background(), port.CreateReservationCommand{
		EnvironmentID:   envID,
		ReservationDate: time.Date(2026, time.September, 20, 0, 0, 0, 0, time.UTC),
		StartTime:       "08:00",
		EndTime:         "10:00",
		Reason:          &reason,
		RequesterID:     "d8231174-567b-40b0-8ab5-4af992406cf6",
	})
	if err != nil {
		t.Fatalf("unexpected create error: %v", err)
	}
	if created.Status != model.ReservationStatusPending || created.StartTime != "08:00:00" || created.EndTime != "10:00:00" {
		t.Fatalf("unexpected created reservation: %+v", created)
	}
	if repo.created == nil || repo.created.ID != created.ID {
		t.Fatalf("repository did not receive created reservation")
	}
}

func TestReservationUseCase_HU007_EnvironmentAndOverlapErrors(t *testing.T) {
	const envID = "6f674be8-24a4-4759-9226-9b9574ba9643"
	command := port.CreateReservationCommand{
		EnvironmentID:   envID,
		ReservationDate: time.Date(2026, time.September, 20, 0, 0, 0, 0, time.UTC),
		StartTime:       "08:00",
		EndTime:         "10:00",
		RequesterID:     "d8231174-567b-40b0-8ab5-4af992406cf6",
	}

	t.Run("missing environment", func(t *testing.T) {
		uc := NewReservationUseCase(&reservationRepoStub{}, &reservationEnvironmentRepoStub{items: map[string]model.Environment{}})
		_, err := uc.Create(context.Background(), command)
		if !errors.Is(err, ErrReservationInvalidEnvironment) {
			t.Fatalf("expected invalid environment, got %v", err)
		}
	})

	t.Run("repository overlap", func(t *testing.T) {
		repo := &reservationRepoStub{err: ErrReservationOverlap}
		envRepo := &reservationEnvironmentRepoStub{items: map[string]model.Environment{envID: {ID: envID}}}
		uc := NewReservationUseCase(repo, envRepo)
		_, err := uc.Create(context.Background(), command)
		if !errors.Is(err, ErrReservationOverlap) {
			t.Fatalf("expected overlap, got %v", err)
		}
	})
}
