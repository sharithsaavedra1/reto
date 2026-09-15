package http

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/code-sena/design-software-training-environment-service/internal/adapter/in/http/dto"
	"github.com/code-sena/design-software-training-environment-service/internal/adapter/in/http/middleware"
	"github.com/code-sena/design-software-training-environment-service/internal/adapter/in/http/response"
	"github.com/code-sena/design-software-training-environment-service/internal/application/port"
	"github.com/code-sena/design-software-training-environment-service/internal/application/usecase"
	"github.com/code-sena/design-software-training-environment-service/internal/domain/model"
)

type MaintenanceHandler struct {
	useCase port.MaintenanceUseCase
}

func NewMaintenanceHandler(useCase port.MaintenanceUseCase) *MaintenanceHandler {
	return &MaintenanceHandler{useCase: useCase}
}

func (h *MaintenanceHandler) RegisterRoutes(mux *http.ServeMux, auth *middleware.AuthMiddleware) {
	mux.HandleFunc("GET /training-environments/{id}/maintenance", auth.RequireFeature("ENV_VIEW", h.List))
	mux.HandleFunc("POST /training-environments/{id}/maintenance", auth.RequireFeature("ENV_MAINTENANCE_MANAGE", h.Create))
	mux.HandleFunc("GET /training-environments/{id}/maintenance/{maintenance_id}", auth.RequireFeature("ENV_VIEW", h.Get))
}

func (h *MaintenanceHandler) List(w http.ResponseWriter, r *http.Request) {
	environmentID := maintenancePathValue(r, "id")
	if !model.IsValidUUID(environmentID) {
		writeMaintenancePathError(w, r, "id")
		return
	}
	from, err := maintenanceQueryDate(r, "from")
	if err != nil {
		writeMaintenanceQueryError(w, r, "from must be a valid ISO date (YYYY-MM-DD)")
		return
	}
	to, err := maintenanceQueryDate(r, "to")
	if err != nil {
		writeMaintenanceQueryError(w, r, "to must be a valid ISO date (YYYY-MM-DD)")
		return
	}
	items, err := h.useCase.List(r.Context(), port.MaintenanceListQuery{EnvironmentID: environmentID, From: from, To: to})
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	response.WriteJSON(w, http.StatusOK, dto.FromMaintenanceModels(items))
}

func (h *MaintenanceHandler) Create(w http.ResponseWriter, r *http.Request) {
	environmentID := maintenancePathValue(r, "id")
	if !model.IsValidUUID(environmentID) {
		writeMaintenancePathError(w, r, "id")
		return
	}
	var request dto.MaintenanceCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeMaintenanceValidationError(w, r, "invalid JSON payload")
		return
	}
	claims, _ := r.Context().Value(middleware.ClaimsContextKey).(*middleware.JWTClaims)
	createdBy := ""
	if claims != nil {
		createdBy = strings.TrimSpace(claims.Subject)
	}
	command, err := request.Command(environmentID, createdBy)
	if err != nil {
		writeMaintenanceValidationError(w, r, "start_date and end_date must use ISO date format (YYYY-MM-DD)")
		return
	}
	created, err := h.useCase.Create(r.Context(), command)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	response.WriteJSON(w, http.StatusCreated, dto.FromMaintenanceModel(*created))
}

func (h *MaintenanceHandler) Get(w http.ResponseWriter, r *http.Request) {
	environmentID := maintenancePathValue(r, "id")
	maintenanceID := maintenancePathValue(r, "maintenance_id")
	if !model.IsValidUUID(environmentID) || !model.IsValidUUID(maintenanceID) {
		writeMaintenancePathError(w, r, "id")
		return
	}
	item, err := h.useCase.Get(r.Context(), environmentID, maintenanceID)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	response.WriteJSON(w, http.StatusOK, dto.FromMaintenanceModel(*item))
}

func (h *MaintenanceHandler) writeError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, usecase.ErrEnvironmentNotFound), errors.Is(err, usecase.ErrMaintenanceNotFound):
		response.WriteError(w, r, http.StatusNotFound, "NOT_FOUND", "maintenance period or training environment not found", nil)
	case errors.Is(err, usecase.ErrMaintenanceValidation), errors.Is(err, model.ErrInvalidMaintenanceDateRange), errors.Is(err, model.ErrMaintenanceReasonTooLong):
		response.WriteError(w, r, http.StatusUnprocessableEntity, "UNPROCESSABLE_ENTITY", err.Error(), nil)
	case errors.Is(err, usecase.ErrMaintenanceQuery):
		writeMaintenanceQueryError(w, r, err.Error())
	default:
		response.WriteError(w, r, http.StatusInternalServerError, "INTERNAL_SERVER_ERROR", "failed to process maintenance period", nil)
	}
}

func maintenanceQueryDate(r *http.Request, name string) (*time.Time, error) {
	values, exists := r.URL.Query()[name]
	if !exists {
		return nil, nil
	}
	if len(values) != 1 || values[0] == "" {
		return nil, errors.New("missing query date")
	}
	value, err := time.Parse(time.DateOnly, values[0])
	if err != nil {
		return nil, err
	}
	return &value, nil
}

func maintenancePathValue(r *http.Request, name string) string {
	if value := r.PathValue(name); value != "" {
		return strings.TrimSpace(value)
	}
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	for index := range parts {
		if parts[index] == "training-environments" && index+1 < len(parts) && name == "id" {
			return parts[index+1]
		}
		if parts[index] == "maintenance" && index+1 < len(parts) && name == "maintenance_id" {
			return parts[index+1]
		}
	}
	return ""
}

func writeMaintenancePathError(w http.ResponseWriter, r *http.Request, field string) {
	response.WriteError(w, r, http.StatusUnprocessableEntity, "UNPROCESSABLE_ENTITY", "invalid maintenance path", []response.ErrorDetail{{Field: field, Message: "must be a valid RFC 4122 UUID"}})
}

func writeMaintenanceQueryError(w http.ResponseWriter, r *http.Request, message string) {
	response.WriteError(w, r, http.StatusUnprocessableEntity, "UNPROCESSABLE_ENTITY", message, nil)
}

func writeMaintenanceValidationError(w http.ResponseWriter, r *http.Request, message string) {
	response.WriteError(w, r, http.StatusUnprocessableEntity, "UNPROCESSABLE_ENTITY", message, nil)
}
