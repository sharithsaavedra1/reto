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

type ReservationUseCase interface {
	Create(ctx context.Context, command CreateReservationCommand) (*model.Reservation, error)
}
