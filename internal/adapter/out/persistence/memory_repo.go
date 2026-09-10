package persistence

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/code-sena/design-software-training-environment-service/internal/application/port"
	"github.com/code-sena/design-software-training-environment-service/internal/domain/model"
)

type MemoryEnvironmentTypeRepository struct {
	mu    sync.RWMutex
	items map[string]model.EnvironmentType
}

// NewMemoryEnvironmentTypeRepository initializes an in-memory repository pre-populated with seed data.
func NewMemoryEnvironmentTypeRepository() port.EnvironmentTypeRepository {
	aulaDesc := "Espacio de formacion teorica con capacidad para grupos completos."
	labDesc := "Espacio dotado con equipos especializados para practicas tecnicas."
	tallerDesc := "Espacio para practicas manuales u operativas con herramientas."

	now := time.Now().UTC()
	repo := &MemoryEnvironmentTypeRepository{
		items: make(map[string]model.EnvironmentType),
	}

	seeds := []model.EnvironmentType{
		{
			ID:          "347d47f9-1ba5-4b9c-89b7-892112a23d90",
			Code:        "CLASSROOM",
			Name:        "Aula",
			Description: &aulaDesc,
			CreatedAt:   now,
		},
		{
			ID:          "9518271b-529f-4942-b7d3-1271827cc812",
			Code:        "LAB",
			Name:        "Laboratorio",
			Description: &labDesc,
			CreatedAt:   now,
		},
		{
			ID:          "4d3be94b-349d-4209-a052-8f3d7e7ce814",
			Code:        "WORKSHOP",
			Name:        "Taller",
			Description: &tallerDesc,
			CreatedAt:   now,
		},
	}

	for _, s := range seeds {
		repo.items[s.ID] = s
	}

	return repo
}

func (r *MemoryEnvironmentTypeRepository) FindAll(_ context.Context) ([]model.EnvironmentType, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]model.EnvironmentType, 0, len(r.items))
	for _, item := range r.items {
		result = append(result, item)
	}
	return result, nil
}

func (r *MemoryEnvironmentTypeRepository) FindByID(_ context.Context, id string) (*model.EnvironmentType, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	item, exists := r.items[id]
	if !exists {
		return nil, nil
	}
	return &item, nil
}

func (r *MemoryEnvironmentTypeRepository) FindByName(_ context.Context, name string) (*model.EnvironmentType, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, item := range r.items {
		if strings.EqualFold(item.Name, strings.TrimSpace(name)) {
			return &item, nil
		}
	}
	return nil, nil
}

func (r *MemoryEnvironmentTypeRepository) FindByCode(_ context.Context, code string) (*model.EnvironmentType, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	trimmed := strings.TrimSpace(code)
	if trimmed == "" {
		return nil, nil
	}

	for _, item := range r.items {
		if strings.EqualFold(item.Code, trimmed) {
			return &item, nil
		}
	}
	return nil, nil
}

func (r *MemoryEnvironmentTypeRepository) Save(_ context.Context, entity *model.EnvironmentType) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.items[entity.ID] = *entity
	return nil
}
