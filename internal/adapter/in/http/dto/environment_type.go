package dto

import (
	"time"

	"github.com/code-sena/design-software-training-environment-service/internal/domain/model"
)

// CreateEnvironmentTypeRequest represents the incoming JSON payload for POST /environment-types.
type CreateEnvironmentTypeRequest struct {
	Code        string  `json:"code"`
	Name        string  `json:"name"`
	Description *string `json:"description,omitempty"`
}

// EnvironmentTypeResponse represents the serialized EnvironmentType in responses.
type EnvironmentTypeResponse struct {
	ID          string  `json:"id"`
	Code        string  `json:"code"`
	Name        string  `json:"name"`
	Description *string `json:"description,omitempty"`
	CreatedAt   string  `json:"created_at"`
}

// FromModel converts a domain EnvironmentType into an HTTP response DTO.
func FromModel(m model.EnvironmentType) EnvironmentTypeResponse {
	return EnvironmentTypeResponse{
		ID:          m.ID,
		Code:        m.Code,
		Name:        m.Name,
		Description: m.Description,
		CreatedAt:   m.CreatedAt.UTC().Format(time.RFC3339),
	}
}

// FromModelList converts a slice of domain EnvironmentType into a slice of response DTOs.
func FromModelList(list []model.EnvironmentType) []EnvironmentTypeResponse {
	res := make([]EnvironmentTypeResponse, len(list))
	for i, item := range list {
		res[i] = FromModel(item)
	}
	return res
}
