package port

import (
	"context"
	"time"

	"github.com/code-sena/design-software-training-environment-service/internal/domain/model"
)

type AvailabilityRuleCreateCommand struct {
	EnvironmentID  string
	DayOfWeek      int
	StartTime      time.Time
	EndTime        time.Time
	EffectiveFrom  time.Time
	EffectiveUntil *time.Time
}

type AvailabilityRuleUpdateCommand = AvailabilityRuleCreateCommand

type AvailabilityRuleUseCase interface {
	List(ctx context.Context, environmentID string, dayOfWeek *int) ([]model.AvailabilityRule, error)
	Get(ctx context.Context, environmentID, ruleID string) (*model.AvailabilityRule, error)
	Create(ctx context.Context, cmd AvailabilityRuleCreateCommand) (*model.AvailabilityRule, error)
	Update(ctx context.Context, environmentID, ruleID string, cmd AvailabilityRuleUpdateCommand) (*model.AvailabilityRule, error)
	Delete(ctx context.Context, environmentID, ruleID string) error
}
