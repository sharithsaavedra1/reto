package port

import (
	"context"

	"github.com/code-sena/design-software-training-environment-service/internal/domain/model"
)

type ReservationRepository interface {
	Create(ctx context.Context, reservation *model.Reservation) error
}
