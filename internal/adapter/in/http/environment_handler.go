package http

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
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
	mux.HandleFunc("PUT /training-environments/{id}", auth.RequireFeature("ENV_MANAGE", h.Update))
	mux.HandleFunc("PATCH /training-environments/{id}", auth.RequireFeature("ENV_MANAGE", h.Update))
	mux.HandleFunc("DELETE /training-environments/{id}", auth.RequireFeature("ENV_MANAGE", h.Deactivate))
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

	if req.Location != nil && utf8.RuneCountInString(*req.Location) > 150 {
		validationDetails = append(validationDetails, response.ErrorDetail{
			Field:   "location",
			Message: "location length must not exceed 150 characters",
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

		if errors.Is(err, model.ErrInvalidCapacity) || errors.Is(err, model.ErrEmptyEnvironmentName) || errors.Is(err, model.ErrEnvironmentNameTooLong) || errors.Is(err, model.ErrLocationTooLong) || errors.Is(err, usecase.ErrValidation) {
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
	id := environmentIDFromRequest(r)

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

// Update handles PUT/PATCH /training-environments/{id}.
func (h *EnvironmentHandler) Update(w http.ResponseWriter, r *http.Request) {
	id := environmentIDFromRequest(r)
	if strings.TrimSpace(id) == "" {
		response.WriteError(w, r, http.StatusNotFound, "NOT_FOUND", "training environment not found", nil)
		return
	}
	if !model.IsValidUUID(id) {
		writeInvalidEnvironmentID(w, r)
		return
	}

	var req dto.UpdateEnvironmentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.WriteError(w, r, http.StatusUnprocessableEntity, "UNPROCESSABLE_ENTITY", "invalid JSON payload", []response.ErrorDetail{{Message: err.Error()}})
		return
	}

	if req.EnvironmentTypeID == nil && req.Name == nil && req.Capacity == nil && !req.Location.Set && req.IsActive == nil {
		response.WriteError(w, r, http.StatusUnprocessableEntity, "UNPROCESSABLE_ENTITY", "at least one editable field is required", nil)
		return
	}

	updated, err := h.useCase.Update(r.Context(), id, port.UpdateEnvironmentCommand{
		EnvironmentTypeID: req.EnvironmentTypeID,
		Name:              req.Name,
		Capacity:          req.Capacity,
		Location:          req.Location.Value,
		LocationSet:       req.Location.Set,
		IsActive:          req.IsActive,
	})
	if err != nil {
		h.writeEnvironmentMutationError(w, r, err)
		return
	}
	response.WriteJSON(w, http.StatusOK, dto.FromEnvironmentModel(*updated))
}

// Deactivate handles DELETE /training-environments/{id} as an idempotent logical deletion.
func (h *EnvironmentHandler) Deactivate(w http.ResponseWriter, r *http.Request) {
	id := environmentIDFromRequest(r)
	if strings.TrimSpace(id) == "" {
		response.WriteError(w, r, http.StatusNotFound, "NOT_FOUND", "training environment not found", nil)
		return
	}
	if !model.IsValidUUID(id) {
		writeInvalidEnvironmentID(w, r)
		return
	}

	if err := h.useCase.Deactivate(r.Context(), id); err != nil {
		if errors.Is(err, usecase.ErrEnvironmentNotFound) {
			response.WriteError(w, r, http.StatusNotFound, "NOT_FOUND", "training environment not found", nil)
			return
		}
		response.WriteError(w, r, http.StatusInternalServerError, "INTERNAL_SERVER_ERROR", "failed to deactivate training environment", nil)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func writeInvalidEnvironmentID(w http.ResponseWriter, r *http.Request) {
	response.WriteError(w, r, http.StatusUnprocessableEntity, "UNPROCESSABLE_ENTITY", "invalid training environment id", []response.ErrorDetail{
		{Field: "id", Message: "id must be a valid RFC 4122 UUID"},
	})
}

func (h *EnvironmentHandler) writeEnvironmentMutationError(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, usecase.ErrEnvironmentNotFound) {
		response.WriteError(w, r, http.StatusNotFound, "NOT_FOUND", "training environment not found", nil)
		return
	}
	if errors.Is(err, usecase.ErrInvalidEnvironmentType) {
		response.WriteError(w, r, http.StatusUnprocessableEntity, "UNPROCESSABLE_ENTITY", "referenced environment_type_id does not exist", []response.ErrorDetail{{Field: "environment_type_id", Message: "environment type not found or invalid reference"}})
		return
	}
	if errors.Is(err, usecase.ErrInvalidEnvironmentUpdate) || errors.Is(err, usecase.ErrValidation) || errors.Is(err, model.ErrInvalidEnvironmentTypeID) || errors.Is(err, model.ErrEmptyEnvironmentName) || errors.Is(err, model.ErrEnvironmentNameTooLong) || errors.Is(err, model.ErrInvalidCapacity) || errors.Is(err, model.ErrLocationTooLong) {
		response.WriteError(w, r, http.StatusUnprocessableEntity, "UNPROCESSABLE_ENTITY", err.Error(), nil)
		return
	}
	response.WriteError(w, r, http.StatusInternalServerError, "INTERNAL_SERVER_ERROR", "failed to update training environment", nil)
}

func environmentIDFromRequest(r *http.Request) string {
	id := r.PathValue("id")
	if id == "" {
		parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		if len(parts) >= 2 {
			id = parts[1]
		}
	}
	return id
}

// List handles GET /training-environments.
func (h *EnvironmentHandler) List(w http.ResponseWriter, r *http.Request) {
	query, err := parseEnvironmentListQuery(r)
	if err != nil {
		parameterErr, ok := err.(*queryParameterError)
		if !ok {
			parameterErr = &queryParameterError{field: "query", msg: err.Error()}
		}
		response.WriteError(w, r, http.StatusUnprocessableEntity, "UNPROCESSABLE_ENTITY", "invalid training environment query", []response.ErrorDetail{
			{Field: parameterErr.field, Message: parameterErr.Error()},
		})
		return
	}

	page, err := h.useCase.Query(r.Context(), query)
	if err != nil {
		if errors.Is(err, usecase.ErrInvalidEnvironmentQuery) {
			response.WriteError(w, r, http.StatusUnprocessableEntity, "UNPROCESSABLE_ENTITY", "invalid training environment query", nil)
			return
		}
		response.WriteError(w, r, http.StatusInternalServerError, "INTERNAL_SERVER_ERROR", "failed to retrieve training environments", nil)
		return
	}

	response.WriteJSON(w, http.StatusOK, dto.FromEnvironmentPage(*page))
}

type queryParameterError struct {
	field string
	msg   string
}

func (e *queryParameterError) Error() string { return e.msg }

func parseEnvironmentListQuery(r *http.Request) (port.EnvironmentListQuery, error) {
	values := r.URL.Query()
	query := port.EnvironmentListQuery{Page: 1, PageSize: 20, SortField: "created_at"}

	if raw, ok := values["page"]; ok {
		page, err := strconv.Atoi(firstQueryValue(raw))
		if err != nil || page < 1 {
			return query, &queryParameterError{field: "page", msg: "page must be an integer greater than 0"}
		}
		query.Page = page
	}
	if raw, ok := values["page_size"]; ok {
		pageSize, err := strconv.Atoi(firstQueryValue(raw))
		if err != nil || pageSize < 1 || pageSize > 100 {
			return query, &queryParameterError{field: "page_size", msg: "page_size must be an integer between 1 and 100"}
		}
		query.PageSize = pageSize
	}

	if raw, ok := values["sort"]; ok {
		parts := strings.Split(firstQueryValue(raw), ":")
		if len(parts) != 2 || (parts[1] != "asc" && parts[1] != "desc") {
			return query, &queryParameterError{field: "sort", msg: "sort must use the format <field>:<asc|desc>"}
		}
		query.SortField = parts[0]
		query.SortDescending = parts[1] == "desc"
	}

	if raw, ok := values["environment_type_id"]; ok {
		query.EnvironmentTypeID = strings.TrimSpace(firstQueryValue(raw))
		if !model.IsValidUUID(query.EnvironmentTypeID) {
			return query, &queryParameterError{field: "environment_type_id", msg: "environment_type_id must be a valid RFC 4122 UUID"}
		}
	}
	if raw, ok := values["training_center_id"]; ok {
		query.TrainingCenterID = strings.TrimSpace(firstQueryValue(raw))
		if !model.IsValidUUID(query.TrainingCenterID) {
			return query, &queryParameterError{field: "training_center_id", msg: "training_center_id must be a valid RFC 4122 UUID"}
		}
	}
	if raw, ok := values["is_active"]; ok {
		active, err := strconv.ParseBool(firstQueryValue(raw))
		if err != nil {
			return query, &queryParameterError{field: "is_active", msg: "is_active must be a boolean"}
		}
		query.IsActive = &active
	}

	dateRaw, dateOK := values["available_date"]
	startRaw, startOK := values["available_start_time"]
	endRaw, endOK := values["available_end_time"]
	present := 0
	if dateOK {
		present++
	}
	if startOK {
		present++
	}
	if endOK {
		present++
	}
	if present != 0 && present != 3 {
		return query, &queryParameterError{field: "available_date", msg: "available_date, available_start_time and available_end_time must be provided together"}
	}
	if present == 3 {
		date, err := time.Parse(time.DateOnly, firstQueryValue(dateRaw))
		if err != nil {
			return query, &queryParameterError{field: "available_date", msg: "available_date must use YYYY-MM-DD"}
		}
		start, err := parseAvailabilityTime(firstQueryValue(startRaw))
		if err != nil {
			return query, &queryParameterError{field: "available_start_time", msg: "available_start_time must use HH:MM or HH:MM:SS"}
		}
		end, err := parseAvailabilityTime(firstQueryValue(endRaw))
		if err != nil {
			return query, &queryParameterError{field: "available_end_time", msg: "available_end_time must use HH:MM or HH:MM:SS"}
		}
		if !start.Before(end) {
			return query, &queryParameterError{field: "available_end_time", msg: "available_end_time must be after available_start_time"}
		}
		query.AvailableDate = &date
		query.AvailableStart = &start
		query.AvailableEnd = &end
	}

	return query, nil
}

func firstQueryValue(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

func parseAvailabilityTime(value string) (time.Time, error) {
	for _, layout := range []string{"15:04", "15:04:05"} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed, nil
		}
	}
	return time.Time{}, fmt.Errorf("invalid time")
}
