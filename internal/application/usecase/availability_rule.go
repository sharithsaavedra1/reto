package usecase

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/code-sena/design-software-training-environment-service/internal/application/port"
	"github.com/code-sena/design-software-training-environment-service/internal/domain/model"
)

var (
	ErrAvailabilityRuleNotFound = errors.New("availability rule not found")
	ErrAvailabilityValidation   = errors.New("invalid availability rule")
	ErrAvailabilityRuleLimit    = errors.New("an environment cannot have more than 24 availability rules")
)

type availabilityRuleUseCase struct {
	repo port.AvailabilityRuleRepository
}

func NewAvailabilityRuleUseCase(repo port.AvailabilityRuleRepository) port.AvailabilityRuleUseCase {
	return &availabilityRuleUseCase{repo: repo}
}

func (u *availabilityRuleUseCase) List(ctx context.Context, environmentID string, dayOfWeek *int) ([]model.AvailabilityRule, error) {
	if !model.IsValidUUID(environmentID) {
		return nil, ErrAvailabilityRuleNotFound
	}
	return u.repo.List(ctx, environmentID, dayOfWeek)
}

func (u *availabilityRuleUseCase) Get(ctx context.Context, environmentID, ruleID string) (*model.AvailabilityRule, error) {
	if !model.IsValidUUID(environmentID) || !model.IsValidUUID(ruleID) {
		return nil, ErrAvailabilityRuleNotFound
	}
	return u.repo.Get(ctx, environmentID, ruleID)
}

func (u *availabilityRuleUseCase) Create(ctx context.Context, cmd port.AvailabilityRuleCreateCommand) (*model.AvailabilityRule, error) {
	rule := &model.AvailabilityRule{
		ID:             newUUID(),
		EnvironmentID:  cmd.EnvironmentID,
		DayOfWeek:      cmd.DayOfWeek,
		StartTime:      cmd.StartTime,
		EndTime:        cmd.EndTime,
		EffectiveFrom:  cmd.EffectiveFrom,
		EffectiveUntil: cmd.EffectiveUntil,
		CreatedAt:      time.Now().UTC(),
	}
	if err := rule.Validate(); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrAvailabilityValidation, err)
	}
	count, err := u.repo.Count(ctx, rule.EnvironmentID)
	if err != nil {
		return nil, err
	}
	if count >= model.MaxAvailabilityRules {
		return nil, ErrAvailabilityRuleLimit
	}
	if err := u.repo.Create(ctx, rule); err != nil {
		return nil, err
	}
	return rule, nil
}

func (u *availabilityRuleUseCase) Update(ctx context.Context, environmentID, ruleID string, cmd port.AvailabilityRuleUpdateCommand) (*model.AvailabilityRule, error) {
	rule, err := u.Get(ctx, environmentID, ruleID)
	if err != nil {
		return nil, err
	}
	rule.DayOfWeek = cmd.DayOfWeek
	rule.StartTime = cmd.StartTime
	rule.EndTime = cmd.EndTime
	rule.EffectiveFrom = cmd.EffectiveFrom
	rule.EffectiveUntil = cmd.EffectiveUntil
	if err := rule.Validate(); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrAvailabilityValidation, err)
	}
	if err := u.repo.Update(ctx, rule); err != nil {
		return nil, err
	}
	return rule, nil
}

func (u *availabilityRuleUseCase) Delete(ctx context.Context, environmentID, ruleID string) error {
	if _, err := u.Get(ctx, environmentID, ruleID); err != nil {
		return err
	}
	return u.repo.Delete(ctx, environmentID, ruleID)
}
