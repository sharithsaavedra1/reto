package http

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/code-sena/design-software-training-environment-service/internal/adapter/in/http/dto"
	"github.com/code-sena/design-software-training-environment-service/internal/adapter/in/http/middleware"
	"github.com/code-sena/design-software-training-environment-service/internal/adapter/in/http/response"
	"github.com/code-sena/design-software-training-environment-service/internal/application/port"
	"github.com/code-sena/design-software-training-environment-service/internal/application/usecase"
	"github.com/code-sena/design-software-training-environment-service/internal/domain/model"
)

// EnvironmentHandler handles HTTP requests for training environments.
type EnvironmentHandler struct {
	useCase port.EnvironmentUseCase
}

// NewEnvironmentHandler creates a new EnvironmentHandler.
func NewEnvironmentHandler(useCase port.EnvironmentUseCase) *EnvironmentHandler {
	return &EnvironmentHandler{
		useCase: useCase,
	}
}

// RegisterRoutes registers the training-environments endpoints to a ServeMux with auth middlewares.
func (h *EnvironmentHandler) RegisterRoutes(mux *http.ServeMux, auth *middleware.AuthMiddleware) {
	mux.HandleFunc("POST /training-environments", auth.RequireFeature("ENV_MANAGE", h.Create))
	mux.HandleFunc("GET /training-environments/{id}", auth.RequireFeature("ENV_VIEW", h.GetByID))
	mux.HandleFunc("GET /training-environments", auth.RequireFeature("ENV_VIEW", h.List))
}

// Create handles POST /training-environments (E1, E2, E3).
func (h *EnvironmentHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req dto.CreateEnvironmentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.WriteError(w, r, http.StatusUnprocessableEntity, "UNPROCESSABLE_ENTITY", "invalid JSON payload", []response.ErrorDetail{
			{Message: err.Error()},
		})
		return
	}

	trimmedTypeID := strings.TrimSpace(req.EnvironmentTypeID)
	trimmedCenterID := strings.TrimSpace(req.TrainingCenterID)
	trimmedName := strings.TrimSpace(req.Name)

	var validationDetails []response.ErrorDetail

	if !model.IsValidUUID(trimmedTypeID) {
		validationDetails = append(validationDetails, response.ErrorDetail{
			Field:   "environment_type_id",
			Message: "environment_type_id is required and must be a valid RFC 4122 UUID",
		})
	}

	if !model.IsValidUUID(trimmedCenterID) {
		validationDetails = append(validationDetails, response.ErrorDetail{
			Field:   "training_center_id",
			Message: "training_center_id is required and must be a valid RFC 4122 UUID",
		})
	}

	if trimmedName == "" {
		validationDetails = append(validationDetails, response.ErrorDetail{
			Field:   "name",
			Message: "name is required and cannot be empty",
		})
	}
	if utf8.RuneCountInString(trimmedName) > 100 {
		validationDetails = append(validationDetails, response.ErrorDetail{
			Field:   "name",
			Message: "name length must not exceed 100 characters",
		})
	}

	if req.Capacity <= 0 {
		validationDetails = append(validationDetails, response.ErrorDetail{
			Field:   "capacity",
			Message: "capacity must be an integer greater than 0",
		})
	}

	if req.Location != nil && utf8.RuneCountInString(*req.Location) > 200 {
		validationDetails = append(validationDetails, response.ErrorDetail{
			Field:   "location",
			Message: "location length must not exceed 200 characters",
		})
	}

	if len(validationDetails) > 0 {
		response.WriteError(w, r, http.StatusUnprocessableEntity, "UNPROCESSABLE_ENTITY", "payload validation failed", validationDetails)
		return
	}

	cmd := port.CreateEnvironmentCommand{
		EnvironmentTypeID: trimmedTypeID,
		TrainingCenterID:  trimmedCenterID,
		Name:              trimmedName,
		Capacity:          req.Capacity,
		Location:          req.Location,
	}

	created, err := h.useCase.Create(r.Context(), cmd)
	if err != nil {
		// E3: Invalid / non-existent environment_type reference
		if errors.Is(err, usecase.ErrInvalidEnvironmentType) {
			response.WriteError(w, r, http.StatusUnprocessableEntity, "UNPROCESSABLE_ENTITY", "referenced environment_type_id does not exist", []response.ErrorDetail{
				{Field: "environment_type_id", Message: "environment type not found or invalid reference"},
			})
			return
		}

		if errors.Is(err, model.ErrInvalidCapacity) || errors.Is(err, model.ErrEmptyEnvironmentName) || errors.Is(err, model.ErrEnvironmentNameTooLong) || errors.Is(err, model.ErrLocationTooLong) {
			response.WriteError(w, r, http.StatusUnprocessableEntity, "UNPROCESSABLE_ENTITY", err.Error(), nil)
			return
		}

		response.WriteError(w, r, http.StatusInternalServerError, "INTERNAL_SERVER_ERROR", "failed to create training environment", nil)
		return
	}

	response.WriteJSON(w, http.StatusCreated, dto.FromEnvironmentModel(*created))
}

// GetByID handles GET /training-environments/{id} (E4).
func (h *EnvironmentHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		if len(parts) >= 2 {
			id = parts[1]
		}
	}

	if strings.TrimSpace(id) == "" {
		response.WriteError(w, r, http.StatusNotFound, "NOT_FOUND", "training environment not found", nil)
		return
	}

	item, err := h.useCase.GetByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, usecase.ErrEnvironmentNotFound) {
			response.WriteError(w, r, http.StatusNotFound, "NOT_FOUND", "training environment not found", []response.ErrorDetail{
				{Field: "id", Message: "no training environment matches the provided id"},
			})
			return
		}
		response.WriteError(w, r, http.StatusInternalServerError, "INTERNAL_SERVER_ERROR", "failed to retrieve training environment", nil)
		return
	}

	response.WriteJSON(w, http.StatusOK, dto.FromEnvironmentModel(*item))
}

// List handles GET /training-environments.
func (h *EnvironmentHandler) List(w http.ResponseWriter, r *http.Request) {
	items, err := h.useCase.List(r.Context())
	if err != nil {
		response.WriteError(w, r, http.StatusInternalServerError, "INTERNAL_SERVER_ERROR", "failed to retrieve training environments", nil)
		return
	}

	response.WriteJSON(w, http.StatusOK, dto.FromEnvironmentModelList(items))
}
