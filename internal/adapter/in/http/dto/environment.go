package dto

import (
	"time"

	"github.com/code-sena/design-software-training-environment-service/internal/domain/model"
)

// CreateEnvironmentRequest represents the incoming payload for POST /training-environments.
type CreateEnvironmentRequest struct {
	EnvironmentTypeID string  `json:"environment_type_id"`
	TrainingCenterID  string  `json:"training_center_id"`
	Name              string  `json:"name"`
	Capacity          int     `json:"capacity"`
	Location          *string `json:"location,omitempty"`
}

// EnvironmentResponse represents the serialized Environment in HTTP responses.
type EnvironmentResponse struct {
	ID                string  `json:"id"`
	EnvironmentTypeID string  `json:"environment_type_id"`
	TrainingCenterID  string  `json:"training_center_id"`
	Name              string  `json:"name"`
	Capacity          int     `json:"capacity"`
	Location          *string `json:"location,omitempty"`
	IsActive          bool    `json:"is_active"`
	CreatedAt         string  `json:"created_at"`
	UpdatedAt         string  `json:"updated_at"`
}

// FromEnvironmentModel converts a domain Environment entity to an EnvironmentResponse DTO.
func FromEnvironmentModel(m model.Environment) EnvironmentResponse {
	return EnvironmentResponse{
		ID:                m.ID,
		EnvironmentTypeID: m.EnvironmentTypeID,
		TrainingCenterID:  m.TrainingCenterID,
		Name:              m.Name,
		Capacity:          m.Capacity,
		Location:          m.Location,
		IsActive:          m.IsActive,
		CreatedAt:         m.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:         m.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

// FromEnvironmentModelList converts a slice of domain Environment entities to DTO responses.
func FromEnvironmentModelList(list []model.Environment) []EnvironmentResponse {
	res := make([]EnvironmentResponse, len(list))
	for i, item := range list {
		res[i] = FromEnvironmentModel(item)
	}
	return res
}
