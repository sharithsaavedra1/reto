package persistence

import (
	"context"
	"sync"
	"time"

	"github.com/code-sena/design-software-training-environment-service/internal/application/port"
	"github.com/code-sena/design-software-training-environment-service/internal/application/usecase"
	"github.com/code-sena/design-software-training-environment-service/internal/domain/model"
)

type MemoryReservationRepository struct {
	mu    sync.RWMutex
	items map[string]model.Reservation
}

func NewMemoryReservationRepository() port.ReservationRepository {
	return &MemoryReservationRepository{items: make(map[string]model.Reservation)}
}

func (r *MemoryReservationRepository) Create(_ context.Context, reservation *model.Reservation) error {
	newStart, _ := model.ParseReservationClock(reservation.StartTime)
	newEnd, _ := model.ParseReservationClock(reservation.EndTime)

	r.mu.Lock()
	defer r.mu.Unlock()
	for _, existing := range r.items {
		if existing.EnvironmentID != reservation.EnvironmentID ||
			existing.Status == model.ReservationStatusCancelled ||
			!sameReservationDate(existing.ReservationDate, reservation.ReservationDate) {
			continue
		}
		existingStart, _ := model.ParseReservationClock(existing.StartTime)
		existingEnd, _ := model.ParseReservationClock(existing.EndTime)
		if newStart < existingEnd && existingStart < newEnd {
			return usecase.ErrReservationOverlap
		}
	}
	r.items[reservation.ID] = *reservation
	return nil
}

func sameReservationDate(a, b time.Time) bool {
	ay, am, ad := a.Date()
	by, bm, bd := b.Date()
	return ay == by && am == bm && ad == bd
}
