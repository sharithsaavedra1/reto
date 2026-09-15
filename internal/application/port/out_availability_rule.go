package port

import (
	"context"

	"github.com/code-sena/design-software-training-environment-service/internal/domain/model"
)

type AvailabilityRuleRepository interface {
	List(ctx context.Context, environmentID string, dayOfWeek *int) ([]model.AvailabilityRule, error)
	Get(ctx context.Context, environmentID, ruleID string) (*model.AvailabilityRule, error)
	Create(ctx context.Context, rule *model.AvailabilityRule) error
	Update(ctx context.Context, rule *model.AvailabilityRule) error
	Delete(ctx context.Context, environmentID, ruleID string) error
}
