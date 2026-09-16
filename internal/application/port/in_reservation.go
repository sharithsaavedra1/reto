package port

import (
	"context"
	"time"

	"github.com/code-sena/design-software-training-environment-service/internal/domain/model"
)

type CreateReservationCommand struct {
	EnvironmentID   string
	ReservationDate time.Time
	StartTime       string
	EndTime         string
	Reason          *string
	RequesterID     string
}

type UpdateReservationCommand struct {
	ID              string
	ReservationDate *time.Time
	StartTime       *string
	EndTime         *string
	Reason          *string
	Status          *string
}

type ReservationUseCase interface {
	Create(ctx context.Context, command CreateReservationCommand) (*model.Reservation, error)
	GetByID(ctx context.Context, id, trainingCenterID string) (*model.Reservation, error)
	Query(ctx context.Context, query ReservationListQuery) (*ReservationPage, error)
	Update(ctx context.Context, command UpdateReservationCommand) (*model.Reservation, error)
	Cancel(ctx context.Context, id string) error
}
