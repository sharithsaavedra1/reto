package persistence

import (
	"context"
	"errors"
	"sort"
	"sync"
	"time"

	"github.com/code-sena/design-software-training-environment-service/internal/application/port"
	"github.com/code-sena/design-software-training-environment-service/internal/domain/model"
)

type MemoryEnvironmentRepository struct {
	mu           sync.RWMutex
	items        map[string]model.Environment
	rules        []memoryAvailabilityRule
	maintenance  []memoryMaintenance
	reservations []memoryReservation
}

type memoryAvailabilityRule struct {
	environmentID  string
	dayOfWeek      int
	start          time.Duration
	end            time.Duration
	effectiveFrom  time.Time
	effectiveUntil *time.Time
}

type memoryMaintenance struct {
	environmentID string
	startDate     time.Time
	endDate       time.Time
}

type memoryReservation struct {
	environmentID string
	date          time.Time
	start         time.Duration
	end           time.Duration
	status        string
}

// NewMemoryEnvironmentRepository creates a new thread-safe in-memory repository pre-populated with seed environments.
func NewMemoryEnvironmentRepository() port.EnvironmentRepository {
	repo := &MemoryEnvironmentRepository{
		items: make(map[string]model.Environment),
	}

	loc1 := "Piso 1, Bloque A"
	loc2 := "Piso 3, Bloque B"
	now := time.Now().UTC()

	seeds := []model.Environment{
		{
			ID:                "78901234-5678-9012-3456-789012345678",
			EnvironmentTypeID: "347d47f9-1ba5-4b9c-89b7-892112a23d90", // Aula
			TrainingCenterID:  "c1d2e3f4-a5b6-7890-1234-56789abcdef0",
			Name:              "Aula 101 - Informática",
			Capacity:          30,
			Location:          &loc1,
			IsActive:          true,
			CreatedAt:         now,
			UpdatedAt:         now,
		},
		{
			ID:                "89012345-6789-0123-4567-890123456789",
			EnvironmentTypeID: "9518271b-529f-4942-b7d3-1271827cc812", // Lab
			TrainingCenterID:  "c1d2e3f4-a5b6-7890-1234-56789abcdef0",
			Name:              "Laboratorio de Software 302",
			Capacity:          25,
			Location:          &loc2,
			IsActive:          true,
			CreatedAt:         now,
			UpdatedAt:         now,
		},
	}

	for _, s := range seeds {
		repo.items[s.ID] = s
	}

	// Deterministic read-model fixtures used by local query tests. Production
	// availability comes from the corresponding PostgreSQL tables.
	effectiveFrom := time.Date(2020, time.January, 1, 0, 0, 0, 0, time.UTC)
	for day := 1; day <= 7; day++ {
		repo.rules = append(repo.rules,
			memoryAvailabilityRule{environmentID: seeds[0].ID, dayOfWeek: day, start: 0, end: 24 * time.Hour, effectiveFrom: effectiveFrom},
			memoryAvailabilityRule{environmentID: seeds[1].ID, dayOfWeek: day, start: 0, end: 24 * time.Hour, effectiveFrom: effectiveFrom},
		)
	}
	repo.maintenance = append(repo.maintenance, memoryMaintenance{
		environmentID: seeds[1].ID,
		startDate:     time.Date(2026, time.September, 15, 0, 0, 0, 0, time.UTC),
		endDate:       time.Date(2026, time.September, 16, 0, 0, 0, 0, time.UTC),
	})
	repo.reservations = append(repo.reservations, memoryReservation{
		environmentID: seeds[0].ID,
		date:          time.Date(2026, time.September, 16, 0, 0, 0, 0, time.UTC),
		start:         10 * time.Hour,
		end:           12 * time.Hour,
		status:        "PENDING",
	})

	return repo
}

func (r *MemoryEnvironmentRepository) Save(_ context.Context, entity *model.Environment) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.items[entity.ID] = *entity
	return nil
}

func (r *MemoryEnvironmentRepository) FindByID(_ context.Context, id string) (*model.Environment, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	item, exists := r.items[id]
	if !exists {
		return nil, nil
	}
	return &item, nil
}

func (r *MemoryEnvironmentRepository) FindAll(_ context.Context) ([]model.Environment, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]model.Environment, 0, len(r.items))
	for _, item := range r.items {
		result = append(result, item)
	}
	return result, nil
}

func (r *MemoryEnvironmentRepository) Query(_ context.Context, query port.EnvironmentListQuery) (*port.EnvironmentPage, error) {
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
	items := make([]model.Environment, 0, len(r.items))
	for _, item := range r.items {
		if query.EnvironmentTypeID != "" && item.EnvironmentTypeID != query.EnvironmentTypeID {
			continue
		}
		if query.TrainingCenterID != "" && item.TrainingCenterID != query.TrainingCenterID {
			continue
		}
		if query.IsActive != nil && item.IsActive != *query.IsActive {
			continue
		}
		if query.AvailableDate != nil && !r.isAvailable(item.ID, query) {
			continue
		}
		items = append(items, item)
	}
	r.mu.RUnlock()

	sort.SliceStable(items, func(i, j int) bool {
		less := false
		switch query.SortField {
		case "id":
			less = items[i].ID < items[j].ID
		case "name":
			less = items[i].Name < items[j].Name
		case "capacity":
			less = items[i].Capacity < items[j].Capacity
		case "updated_at":
			less = items[i].UpdatedAt.Before(items[j].UpdatedAt)
		case "is_active":
			less = !items[i].IsActive && items[j].IsActive
		default:
			less = items[i].CreatedAt.Before(items[j].CreatedAt)
		}
		if query.SortDescending {
			return !less && !memoryEnvironmentSortEqual(items[i], items[j], query.SortField)
		}
		return less
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
	pageItems := append([]model.Environment(nil), items[start:end]...)
	totalPages := 0
	if total > 0 {
		totalPages = (total + query.PageSize - 1) / query.PageSize
	}
	return &port.EnvironmentPage{Items: pageItems, Page: query.Page, PageSize: query.PageSize, TotalItems: total, TotalPages: totalPages}, nil
}

func (r *MemoryEnvironmentRepository) isAvailable(environmentID string, query port.EnvironmentListQuery) bool {
	date := dateOnly(*query.AvailableDate)
	dayOfWeek := int(date.Weekday())
	if dayOfWeek == 0 {
		dayOfWeek = 7
	}
	requestedStart := clockDuration(*query.AvailableStart)
	requestedEnd := clockDuration(*query.AvailableEnd)

	hasRule := false
	for _, rule := range r.rules {
		if rule.environmentID == environmentID && rule.dayOfWeek == dayOfWeek &&
			!date.Before(dateOnly(rule.effectiveFrom)) &&
			(rule.effectiveUntil == nil || date.Before(dateOnly(*rule.effectiveUntil))) &&
			rule.start <= requestedStart && rule.end >= requestedEnd {
			hasRule = true
			break
		}
	}
	if !hasRule {
		return false
	}

	for _, block := range r.maintenance {
		if block.environmentID == environmentID && !date.Before(dateOnly(block.startDate)) && date.Before(dateOnly(block.endDate)) {
			return false
		}
	}
	for _, reservation := range r.reservations {
		if reservation.environmentID == environmentID && reservation.status != "CANCELLED" && date.Equal(dateOnly(reservation.date)) &&
			reservation.start < requestedEnd && reservation.end > requestedStart {
			return false
		}
	}
	return true
}

func dateOnly(value time.Time) time.Time {
	return time.Date(value.UTC().Year(), value.UTC().Month(), value.UTC().Day(), 0, 0, 0, 0, time.UTC)
}

func clockDuration(value time.Time) time.Duration {
	return time.Duration(value.Hour())*time.Hour + time.Duration(value.Minute())*time.Minute + time.Duration(value.Second())*time.Second
}

func memoryEnvironmentSortEqual(a, b model.Environment, field string) bool {
	switch field {
	case "id":
		return a.ID == b.ID
	case "name":
		return a.Name == b.Name
	case "capacity":
		return a.Capacity == b.Capacity
	case "updated_at":
		return a.UpdatedAt.Equal(b.UpdatedAt)
	case "is_active":
		return a.IsActive == b.IsActive
	default:
		return a.CreatedAt.Equal(b.CreatedAt)
	}
}
