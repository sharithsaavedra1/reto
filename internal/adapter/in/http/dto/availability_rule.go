package dto

import (
	"time"

	"github.com/code-sena/design-software-training-environment-service/internal/application/port"
	"github.com/code-sena/design-software-training-environment-service/internal/domain/model"
)

type AvailabilityRuleResponse struct {
	ID             string  `json:"id"`
	EnvironmentID  string  `json:"environment_id"`
	DayOfWeek      int     `json:"day_of_week"`
	StartTime      string  `json:"start_time"`
	EndTime        string  `json:"end_time"`
	EffectiveFrom  string  `json:"effective_from"`
	EffectiveUntil *string `json:"effective_until"`
	CreatedAt      string  `json:"created_at"`
}

type AvailabilityRuleRequest struct {
	DayOfWeek      int     `json:"day_of_week"`
	StartTime      string  `json:"start_time"`
	EndTime        string  `json:"end_time"`
	EffectiveFrom  string  `json:"effective_from"`
	EffectiveUntil *string `json:"effective_until"`
}

func FromAvailabilityRuleModel(rule model.AvailabilityRule) AvailabilityRuleResponse {
	var until *string
	if rule.EffectiveUntil != nil {
		value := rule.EffectiveUntil.UTC().Format(time.DateOnly)
		until = &value
	}
	return AvailabilityRuleResponse{
		ID: rule.ID, EnvironmentID: rule.EnvironmentID, DayOfWeek: rule.DayOfWeek,
		StartTime: rule.StartTime.Format("15:04:05"), EndTime: rule.EndTime.Format("15:04:05"),
		EffectiveFrom: rule.EffectiveFrom.UTC().Format(time.DateOnly), EffectiveUntil: until,
		CreatedAt: rule.CreatedAt.UTC().Format(time.RFC3339),
	}
}

func FromAvailabilityRuleModels(rules []model.AvailabilityRule) []AvailabilityRuleResponse {
	result := make([]AvailabilityRuleResponse, len(rules))
	for i, rule := range rules {
		result[i] = FromAvailabilityRuleModel(rule)
	}
	return result
}

func (r AvailabilityRuleRequest) Command(environmentID string) (port.AvailabilityRuleCreateCommand, error) {
	start, err := time.Parse("15:04:05", r.StartTime)
	if err != nil {
		return port.AvailabilityRuleCreateCommand{}, err
	}
	end, err := time.Parse("15:04:05", r.EndTime)
	if err != nil {
		return port.AvailabilityRuleCreateCommand{}, err
	}
	effectiveFrom, err := time.Parse(time.DateOnly, r.EffectiveFrom)
	if err != nil {
		return port.AvailabilityRuleCreateCommand{}, err
	}
	var effectiveUntil *time.Time
	if r.EffectiveUntil != nil {
		value, err := time.Parse(time.DateOnly, *r.EffectiveUntil)
		if err != nil {
			return port.AvailabilityRuleCreateCommand{}, err
		}
		effectiveUntil = &value
	}
	return port.AvailabilityRuleCreateCommand{
		EnvironmentID: environmentID, DayOfWeek: r.DayOfWeek, StartTime: start, EndTime: end,
		EffectiveFrom: effectiveFrom, EffectiveUntil: effectiveUntil,
	}, nil
}
