package persistence

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/code-sena/design-software-training-environment-service/internal/application/port"
	"github.com/code-sena/design-software-training-environment-service/internal/application/usecase"
	"github.com/code-sena/design-software-training-environment-service/internal/domain/model"
)

type MemoryMaintenanceRepository struct {
	mu      sync.RWMutex
	envRepo port.EnvironmentRepository
	items   map[string]model.Maintenance
	outbox  []MemoryMaintenanceOutboxEvent
}

type MemoryMaintenanceOutboxEvent struct {
	EventType string
	Payload   map[string]interface{}
}

func NewMemoryMaintenanceRepository(envRepo port.EnvironmentRepository) *MemoryMaintenanceRepository {
	return &MemoryMaintenanceRepository{
		envRepo: envRepo,
		items:   make(map[string]model.Maintenance),
	}
}

func (r *MemoryMaintenanceRepository) List(ctx context.Context, query port.MaintenanceListQuery) ([]model.Maintenance, error) {
	if err := r.ensureEnvironment(ctx, query.EnvironmentID); err != nil {
		return nil, err
	}

	r.mu.RLock()
	result := make([]model.Maintenance, 0)
	for _, item := range r.items {
		if item.EnvironmentID != query.EnvironmentID || !maintenanceOverlaps(item, query.From, query.To) {
			continue
		}
		result = append(result, item)
	}
	r.mu.RUnlock()

	sort.SliceStable(result, func(i, j int) bool {
		startI, _, errI := model.MaintenanceBounds(result[i])
		startJ, _, errJ := model.MaintenanceBounds(result[j])
		if errI == nil && errJ == nil && !startI.Equal(startJ) {
			return startI.Before(startJ)
		}
		return result[i].ID < result[j].ID
	})
	return result, nil
}

func (r *MemoryMaintenanceRepository) Get(ctx context.Context, environmentID, maintenanceID string) (*model.Maintenance, error) {
	if err := r.ensureEnvironment(ctx, environmentID); err != nil {
		return nil, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	item, ok := r.items[maintenanceID]
	if !ok || item.EnvironmentID != environmentID {
		return nil, usecase.ErrMaintenanceNotFound
	}
	return &item, nil
}

func (r *MemoryMaintenanceRepository) Create(ctx context.Context, maintenance *model.Maintenance) error {
	if err := r.ensureEnvironment(ctx, maintenance.EnvironmentID); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.items[maintenance.ID] = *maintenance
	r.outbox = append(r.outbox, MemoryMaintenanceOutboxEvent{
		EventType: "environment.maintenance.started",
		Payload: map[string]interface{}{
			"environment_id": maintenance.EnvironmentID,
			"maintenance_id": maintenance.ID,
			"start_date":     maintenance.StartDate.Format("2006-01-02"),
			"end_date":       maintenance.EndDate.Format("2006-01-02"),
			"start_time":     maintenance.StartTime,
			"end_time":       maintenance.EndTime,
			"description":    valueOrEmpty(maintenance.Reason),
			"created_by":     maintenance.CreatedBy,
			"created_at":     maintenance.CreatedAt,
		},
	})
	return nil
}

func (r *MemoryMaintenanceRepository) OutboxEvents() []MemoryMaintenanceOutboxEvent {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return append([]MemoryMaintenanceOutboxEvent(nil), r.outbox...)
}

func (r *MemoryMaintenanceRepository) ensureEnvironment(ctx context.Context, environmentID string) error {
	item, err := r.envRepo.FindByID(ctx, environmentID)
	if err != nil {
		return err
	}
	if item == nil {
		return usecase.ErrEnvironmentNotFound
	}
	return nil
}

func maintenanceOverlaps(item model.Maintenance, from, to *time.Time) bool {
	startAt, endAt, err := model.MaintenanceBounds(item)
	if err != nil {
		return false
	}
	if from != nil && !endAt.After(maintenanceDateBoundary(*from)) {
		return false
	}
	if to != nil && !startAt.Before(maintenanceDateBoundary(*to)) {
		return false
	}
	return true
}

func maintenanceDateBoundary(value time.Time) time.Time {
	value = value.UTC()
	return time.Date(value.Year(), value.Month(), value.Day(), 0, 0, 0, 0, time.UTC)
}

func valueOrEmpty(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
