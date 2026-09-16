package persistence

import (
	"context"
	"errors"
	"sort"
	"sync"
	"time"

	"github.com/code-sena/design-software-training-environment-service/internal/application/port"
	"github.com/code-sena/design-software-training-environment-service/internal/application/usecase"
	"github.com/code-sena/design-software-training-environment-service/internal/domain/model"
)

type MemoryReservationRepository struct {
	mu      sync.RWMutex
	items   map[string]model.Reservation
	envRepo port.EnvironmentRepository
}

func NewMemoryReservationRepository(envRepos ...port.EnvironmentRepository) port.ReservationRepository {
	var envRepo port.EnvironmentRepository
	if len(envRepos) > 0 {
		envRepo = envRepos[0]
	}
	return &MemoryReservationRepository{items: make(map[string]model.Reservation), envRepo: envRepo}
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

func (r *MemoryReservationRepository) FindByID(ctx context.Context, id, trainingCenterID string) (*model.Reservation, error) {
	r.mu.RLock()
	item, exists := r.items[id]
	r.mu.RUnlock()
	if !exists {
		return nil, nil
	}
	if trainingCenterID != "" {
		allowed, err := r.belongsToTrainingCenter(ctx, item.EnvironmentID, trainingCenterID)
		if err != nil || !allowed {
			return nil, err
		}
	}
	copy := item
	return &copy, nil
}

func (r *MemoryReservationRepository) Query(ctx context.Context, query port.ReservationListQuery) (*port.ReservationPage, error) {
	if query.Page == 0 {
		query.Page = 1
	}
	if query.PageSize == 0 {
		query.PageSize = 20
	}
	if query.Page < 1 || query.PageSize < 1 || query.PageSize > 100 {
		return nil, errors.New("invalid pagination")
	}

	r.mu.RLock()
	candidates := make([]model.Reservation, 0, len(r.items))
	for _, item := range r.items {
		candidates = append(candidates, item)
	}
	r.mu.RUnlock()

	items := make([]model.Reservation, 0, len(candidates))
	for _, item := range candidates {
		if query.EnvironmentID != "" && item.EnvironmentID != query.EnvironmentID {
			continue
		}
		if query.RequesterID != "" && item.RequesterID != query.RequesterID {
			continue
		}
		if query.Status != "" && item.Status != query.Status {
			continue
		}
		date := reservationDateOnly(item.ReservationDate)
		if query.From != nil && date.Before(reservationDateOnly(*query.From)) {
			continue
		}
		if query.To != nil && !date.Before(reservationDateOnly(*query.To)) {
			continue
		}
		if query.TrainingCenterID != "" {
			allowed, err := r.belongsToTrainingCenter(ctx, item.EnvironmentID, query.TrainingCenterID)
			if err != nil {
				return nil, err
			}
			if !allowed {
				continue
			}
		}
		items = append(items, item)
	}

	sort.SliceStable(items, func(i, j int) bool {
		cmp := compareReservations(items[i], items[j], query.SortField)
		if cmp == 0 {
			return items[i].ID < items[j].ID
		}
		if query.SortDescending {
			return cmp > 0
		}
		return cmp < 0
	})

	total := len(items)
	start := (query.Page - 1) * query.PageSize
	if start > total {
		start = total
	}
	end := start + query.PageSize
	if end > total {
		end = total
	}
	pageItems := append([]model.Reservation(nil), items[start:end]...)
	totalPages := 0
	if total > 0 {
		totalPages = (total + query.PageSize - 1) / query.PageSize
	}
	return &port.ReservationPage{Items: pageItems, Page: query.Page, PageSize: query.PageSize, TotalItems: total, TotalPages: totalPages}, nil
}

func (r *MemoryReservationRepository) belongsToTrainingCenter(ctx context.Context, environmentID, trainingCenterID string) (bool, error) {
	if r.envRepo == nil {
		return false, nil
	}
	environment, err := r.envRepo.FindByID(ctx, environmentID)
	if err != nil || environment == nil {
		return false, err
	}
	return environment.TrainingCenterID == trainingCenterID, nil
}

func compareReservations(a, b model.Reservation, field string) int {
	switch field {
	case "id":
		return compareString(a.ID, b.ID)
	case "environment_id":
		return compareString(a.EnvironmentID, b.EnvironmentID)
	case "requester_id":
		return compareString(a.RequesterID, b.RequesterID)
	case "reservation_date":
		return compareTime(a.ReservationDate, b.ReservationDate)
	case "start_time":
		return compareString(a.StartTime, b.StartTime)
	case "end_time":
		return compareString(a.EndTime, b.EndTime)
	case "status":
		return compareString(a.Status, b.Status)
	case "updated_at":
		return compareTime(a.UpdatedAt, b.UpdatedAt)
	default:
		return compareTime(a.CreatedAt, b.CreatedAt)
	}
}

func compareString(a, b string) int {
	if a < b {
		return -1
	}
	if a > b {
		return 1
	}
	return 0
}

func compareTime(a, b time.Time) int {
	if a.Before(b) {
		return -1
	}
	if a.After(b) {
		return 1
	}
	return 0
}

func sameReservationDate(a, b time.Time) bool {
	ay, am, ad := a.Date()
	by, bm, bd := b.Date()
	return ay == by && am == bm && ad == bd
}

func reservationDateOnly(value time.Time) time.Time {
	value = value.UTC()
	return time.Date(value.Year(), value.Month(), value.Day(), 0, 0, 0, 0, time.UTC)
}
