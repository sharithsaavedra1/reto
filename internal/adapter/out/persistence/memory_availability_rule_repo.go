package persistence

import (
	"context"
	"sync"

	"github.com/code-sena/design-software-training-environment-service/internal/application/port"
	"github.com/code-sena/design-software-training-environment-service/internal/application/usecase"
	"github.com/code-sena/design-software-training-environment-service/internal/domain/model"
)

type MemoryAvailabilityRuleRepository struct {
	mu      sync.RWMutex
	envRepo port.EnvironmentRepository
	rules   map[string]model.AvailabilityRule
	outbox  []MemoryOutboxEvent
}

type MemoryOutboxEvent struct {
	EventType string
	Payload   map[string]interface{}
}

func NewMemoryAvailabilityRuleRepository(envRepo port.EnvironmentRepository) *MemoryAvailabilityRuleRepository {
	return &MemoryAvailabilityRuleRepository{
		envRepo: envRepo,
		rules:   make(map[string]model.AvailabilityRule),
	}
}

func (r *MemoryAvailabilityRuleRepository) List(ctx context.Context, environmentID string, dayOfWeek *int) ([]model.AvailabilityRule, error) {
	if err := r.ensureEnvironment(ctx, environmentID); err != nil {
		return nil, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]model.AvailabilityRule, 0)
	for _, rule := range r.rules {
		if rule.EnvironmentID == environmentID && (dayOfWeek == nil || rule.DayOfWeek == *dayOfWeek) {
			result = append(result, rule)
		}
	}
	return result, nil
}

func (r *MemoryAvailabilityRuleRepository) Get(ctx context.Context, environmentID, ruleID string) (*model.AvailabilityRule, error) {
	if err := r.ensureEnvironment(ctx, environmentID); err != nil {
		return nil, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	rule, ok := r.rules[ruleID]
	if !ok || rule.EnvironmentID != environmentID {
		return nil, usecase.ErrAvailabilityRuleNotFound
	}
	return &rule, nil
}

func (r *MemoryAvailabilityRuleRepository) Create(ctx context.Context, rule *model.AvailabilityRule) error {
	if err := r.ensureEnvironment(ctx, rule.EnvironmentID); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.rules[rule.ID] = *rule
	r.outbox = append(r.outbox, MemoryOutboxEvent{EventType: "environment.availability.changed", Payload: map[string]interface{}{
		"environment_id": rule.EnvironmentID, "change_type": "RULE_ADDED", "rule_id": rule.ID,
	}})
	return nil
}

func (r *MemoryAvailabilityRuleRepository) Update(ctx context.Context, rule *model.AvailabilityRule) error {
	if err := r.ensureEnvironment(ctx, rule.EnvironmentID); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	current, ok := r.rules[rule.ID]
	if !ok || current.EnvironmentID != rule.EnvironmentID {
		return usecase.ErrAvailabilityRuleNotFound
	}
	r.rules[rule.ID] = *rule
	r.outbox = append(r.outbox, MemoryOutboxEvent{EventType: "environment.availability.changed", Payload: map[string]interface{}{
		"environment_id": rule.EnvironmentID, "change_type": "RULE_MODIFIED", "rule_id": rule.ID,
	}})
	return nil
}

func (r *MemoryAvailabilityRuleRepository) Delete(ctx context.Context, environmentID, ruleID string) error {
	if err := r.ensureEnvironment(ctx, environmentID); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	rule, ok := r.rules[ruleID]
	if !ok || rule.EnvironmentID != environmentID {
		return usecase.ErrAvailabilityRuleNotFound
	}
	delete(r.rules, ruleID)
	r.outbox = append(r.outbox, MemoryOutboxEvent{EventType: "environment.availability.changed", Payload: map[string]interface{}{
		"environment_id": environmentID, "change_type": "RULE_REMOVED", "rule_id": ruleID,
	}})
	return nil
}

func (r *MemoryAvailabilityRuleRepository) OutboxEvents() []MemoryOutboxEvent {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return append([]MemoryOutboxEvent(nil), r.outbox...)
}

func (r *MemoryAvailabilityRuleRepository) ensureEnvironment(ctx context.Context, environmentID string) error {
	item, err := r.envRepo.FindByID(ctx, environmentID)
	if err != nil {
		return err
	}
	if item == nil {
		return usecase.ErrEnvironmentNotFound
	}
	return nil
}
