package port

import (
	"context"
	"time"

	"github.com/code-sena/design-software-training-environment-service/internal/domain/model"
)

// ReservationListQuery contains the supported filters and offset pagination
// parameters for GET /reservations.
type ReservationListQuery struct {
	Page             int
	PageSize         int
	SortField        string
	SortDescending   bool
	EnvironmentID    string
	RequesterID      string
	Status           string
	From             *time.Time
	To               *time.Time
	TrainingCenterID string
}

// ReservationPage is the paginated result returned by the reservation query.
type ReservationPage struct {
	Items      []model.Reservation
	Page       int
	PageSize   int
	TotalItems int
	TotalPages int
}

type ReservationRepository interface {
	Create(ctx context.Context, reservation *model.Reservation) error
	FindByID(ctx context.Context, id, trainingCenterID string) (*model.Reservation, error)
	Query(ctx context.Context, query ReservationListQuery) (*ReservationPage, error)
}
