package persistence

import (
	"context"
	"sync"
	"time"

	"github.com/code-sena/design-software-training-environment-service/internal/application/port"
	"github.com/code-sena/design-software-training-environment-service/internal/domain/model"
)

type MemoryEnvironmentRepository struct {
	mu    sync.RWMutex
	items map[string]model.Environment
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
