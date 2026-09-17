package dto

import (
	"time"

	"github.com/code-sena/design-software-training-environment-service/internal/application/port"
	"github.com/code-sena/design-software-training-environment-service/internal/domain/model"
)

type EnvironmentUtilizationResponse struct {
	EnvironmentID         string  `json:"environment_id"`
	EnvironmentName       string  `json:"environment_name"`
	TrainingCenterID      string  `json:"training_center_id"`
	EnvironmentTypeCode   string  `json:"environment_type_code"`
	PeriodFrom            string  `json:"period_from"`
	PeriodTo              string  `json:"period_to"`
	AvailableHours        float64 `json:"available_hours"`
	ReservedHours         float64 `json:"reserved_hours"`
	MaintenanceHours      float64 `json:"maintenance_hours"`
	UtilizationPercentage float64 `json:"utilization_percentage"`
}

type EnvironmentUtilizationPageResponse struct {
	Data       []EnvironmentUtilizationResponse `json:"data"`
	Pagination PaginationResponse               `json:"pagination"`
}

func FromEnvironmentUtilizationModel(item model.EnvironmentUtilization) EnvironmentUtilizationResponse {
	return EnvironmentUtilizationResponse{
		EnvironmentID: item.EnvironmentID, EnvironmentName: item.EnvironmentName,
		TrainingCenterID: item.TrainingCenterID, EnvironmentTypeCode: item.EnvironmentTypeCode,
		PeriodFrom: item.PeriodFrom.UTC().Format(time.DateOnly), PeriodTo: item.PeriodTo.UTC().Format(time.DateOnly),
		AvailableHours: item.AvailableHours, ReservedHours: item.ReservedHours,
		MaintenanceHours: item.MaintenanceHours, UtilizationPercentage: item.UtilizationPercentage,
	}
}

func FromEnvironmentUtilizationPage(page port.EnvironmentUtilizationPage) EnvironmentUtilizationPageResponse {
	data := make([]EnvironmentUtilizationResponse, len(page.Items))
	for i, item := range page.Items {
		data[i] = FromEnvironmentUtilizationModel(item)
	}
	return EnvironmentUtilizationPageResponse{
		Data:       data,
		Pagination: PaginationResponse{Page: page.Page, PageSize: page.PageSize, TotalItems: page.TotalItems, TotalPages: page.TotalPages},
	}
}
