package http

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/code-sena/design-software-training-environment-service/internal/adapter/in/http/dto"
	"github.com/code-sena/design-software-training-environment-service/internal/adapter/in/http/middleware"
	"github.com/code-sena/design-software-training-environment-service/internal/adapter/in/http/response"
	"github.com/code-sena/design-software-training-environment-service/internal/application/port"
	"github.com/code-sena/design-software-training-environment-service/internal/application/usecase"
	"github.com/code-sena/design-software-training-environment-service/internal/domain/model"
)

// EnvironmentTypeHandler handles HTTP requests for environment types.
type EnvironmentTypeHandler struct {
	useCase port.EnvironmentTypeUseCase
}

// NewEnvironmentTypeHandler creates a new EnvironmentTypeHandler.
func NewEnvironmentTypeHandler(useCase port.EnvironmentTypeUseCase) *EnvironmentTypeHandler {
	return &EnvironmentTypeHandler{
		useCase: useCase,
	}
}

// RegisterRoutes registers the environment-types endpoints to a standard http.ServeMux with auth middlewares.
func (h *EnvironmentTypeHandler) RegisterRoutes(mux *http.ServeMux, auth *middleware.AuthMiddleware) {
	mux.HandleFunc("GET /environment-types", auth.RequireFeature("ENV_VIEW", h.List))
	mux.HandleFunc("POST /environment-types", auth.RequireFeature("ENV_MANAGE", h.Create))
	mux.HandleFunc("GET /environment-types/{id}", auth.RequireFeature("ENV_VIEW", h.GetByID))
}

// List handles GET /environment-types (E1).
func (h *EnvironmentTypeHandler) List(w http.ResponseWriter, r *http.Request) {
	items, err := h.useCase.List(r.Context())
	if err != nil {
		response.WriteError(w, r, http.StatusInternalServerError, "INTERNAL_SERVER_ERROR", "failed to retrieve environment types", nil)
		return
	}

	response.WriteJSON(w, http.StatusOK, dto.FromModelList(items))
}

// Create handles POST /environment-types (E2).
func (h *EnvironmentTypeHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req dto.CreateEnvironmentTypeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.WriteError(w, r, http.StatusUnprocessableEntity, "UNPROCESSABLE_ENTITY", "invalid JSON payload", []response.ErrorDetail{
			{Message: err.Error()},
		})
		return
	}

	// Payload validations
	trimmedName := strings.TrimSpace(req.Name)
	trimmedCode := strings.TrimSpace(req.Code)

	var validationDetails []response.ErrorDetail
	if trimmedName == "" {
		validationDetails = append(validationDetails, response.ErrorDetail{Field: "name", Message: "name is required and cannot be empty"})
	}
	if len(trimmedName) > 100 {
		validationDetails = append(validationDetails, response.ErrorDetail{Field: "name", Message: "name length must not exceed 100 characters"})
	}
	if len(trimmedCode) > 20 {
		validationDetails = append(validationDetails, response.ErrorDetail{Field: "code", Message: "code length must not exceed 20 characters"})
	}
	if req.Description != nil && len(*req.Description) > 255 {
		validationDetails = append(validationDetails, response.ErrorDetail{Field: "description", Message: "description length must not exceed 255 characters"})
	}

	if len(validationDetails) > 0 {
		response.WriteError(w, r, http.StatusUnprocessableEntity, "UNPROCESSABLE_ENTITY", "payload validation failed", validationDetails)
		return
	}

	cmd := port.CreateEnvironmentTypeCommand{
		Code:        trimmedCode,
		Name:        trimmedName,
		Description: req.Description,
	}

	created, err := h.useCase.Create(r.Context(), cmd)
	if err != nil {
		if errors.Is(err, usecase.ErrConflictName) {
			response.WriteError(w, r, http.StatusConflict, "CONFLICT", "an environment type with this name already exists", []response.ErrorDetail{
				{Field: "name", Message: "name must be unique"},
			})
			return
		}
		if errors.Is(err, usecase.ErrConflictCode) {
			response.WriteError(w, r, http.StatusConflict, "CONFLICT", "an environment type with this code already exists", []response.ErrorDetail{
				{Field: "code", Message: "code must be unique"},
			})
			return
		}
		if errors.Is(err, usecase.ErrValidation) || errors.Is(err, model.ErrEmptyName) || errors.Is(err, model.ErrNameTooLong) || errors.Is(err, model.ErrCodeTooLong) {
			response.WriteError(w, r, http.StatusUnprocessableEntity, "UNPROCESSABLE_ENTITY", err.Error(), nil)
			return
		}

		response.WriteError(w, r, http.StatusInternalServerError, "INTERNAL_SERVER_ERROR", "failed to create environment type", nil)
		return
	}

	response.WriteJSON(w, http.StatusCreated, dto.FromModel(*created))
}

// GetByID handles GET /environment-types/{id} (E3).
func (h *EnvironmentTypeHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		// Fallback extraction for older URL routing or custom routers
		parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		if len(parts) >= 2 {
			id = parts[1]
		}
	}

	if strings.TrimSpace(id) == "" {
		response.WriteError(w, r, http.StatusNotFound, "NOT_FOUND", "environment type not found", nil)
		return
	}

	item, err := h.useCase.GetByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, usecase.ErrNotFound) {
			response.WriteError(w, r, http.StatusNotFound, "NOT_FOUND", "environment type not found", []response.ErrorDetail{
				{Field: "id", Message: "no environment type matches the provided id"},
			})
			return
		}
		response.WriteError(w, r, http.StatusInternalServerError, "INTERNAL_SERVER_ERROR", "failed to retrieve environment type", nil)
		return
	}

	response.WriteJSON(w, http.StatusOK, dto.FromModel(*item))
}
