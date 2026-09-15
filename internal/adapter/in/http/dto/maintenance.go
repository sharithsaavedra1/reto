package dto

import (
	"time"

	"github.com/code-sena/design-software-training-environment-service/internal/application/port"
	"github.com/code-sena/design-software-training-environment-service/internal/domain/model"
)

type MaintenanceCreateRequest struct {
	StartDate string  `json:"start_date"`
	EndDate   string  `json:"end_date"`
	Reason    *string `json:"reason"`
}

type MaintenanceResponse struct {
	ID            string  `json:"id"`
	EnvironmentID string  `json:"environment_id"`
	StartDate     string  `json:"start_date"`
	EndDate       string  `json:"end_date"`
	Reason        *string `json:"reason"`
	CreatedBy     string  `json:"created_by"`
	CreatedAt     string  `json:"created_at"`
}

func (r MaintenanceCreateRequest) Command(environmentID, createdBy string) (port.MaintenanceCreateCommand, error) {
	start, err := time.Parse(time.DateOnly, r.StartDate)
	if err != nil {
		return port.MaintenanceCreateCommand{}, err
	}
	end, err := time.Parse(time.DateOnly, r.EndDate)
	if err != nil {
		return port.MaintenanceCreateCommand{}, err
	}
	return port.MaintenanceCreateCommand{
		EnvironmentID: environmentID,
		StartDate:     start,
		EndDate:       end,
		Reason:        r.Reason,
		CreatedBy:     createdBy,
	}, nil
}

func FromMaintenanceModel(m model.Maintenance) MaintenanceResponse {
	return MaintenanceResponse{
		ID:            m.ID,
		EnvironmentID: m.EnvironmentID,
		StartDate:     m.StartDate.UTC().Format(time.DateOnly),
		EndDate:       m.EndDate.UTC().Format(time.DateOnly),
		Reason:        m.Reason,
		CreatedBy:     m.CreatedBy,
		CreatedAt:     m.CreatedAt.UTC().Format(time.RFC3339),
	}
}

func FromMaintenanceModels(items []model.Maintenance) []MaintenanceResponse {
	result := make([]MaintenanceResponse, 0, len(items))
	for _, item := range items {
		result = append(result, FromMaintenanceModel(item))
	}
	return result
}
