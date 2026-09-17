package http

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/code-sena/design-software-training-environment-service/internal/adapter/in/http/dto"
	"github.com/code-sena/design-software-training-environment-service/internal/adapter/in/http/middleware"
	"github.com/code-sena/design-software-training-environment-service/internal/adapter/in/http/response"
	"github.com/code-sena/design-software-training-environment-service/internal/application/port"
	"github.com/code-sena/design-software-training-environment-service/internal/application/usecase"
)

type EnvironmentUtilizationHandler struct {
	useCase port.EnvironmentUtilizationUseCase
}

func NewEnvironmentUtilizationHandler(useCase port.EnvironmentUtilizationUseCase) *EnvironmentUtilizationHandler {
	return &EnvironmentUtilizationHandler{useCase: useCase}
}

func (h *EnvironmentUtilizationHandler) RegisterRoutes(mux *http.ServeMux, auth *middleware.AuthMiddleware) {
	mux.HandleFunc("GET /reports/environment-utilization", auth.RequireFeature("ENVIRONMENT_REPORT_VIEW", h.Report))
}

func (h *EnvironmentUtilizationHandler) Report(w http.ResponseWriter, r *http.Request) {
	query, err := parseEnvironmentUtilizationQuery(r)
	if err != nil {
		field, message := "query", err.Error()
		var parameterErr *utilizationQueryError
		if errors.As(err, &parameterErr) {
			field, message = parameterErr.field, parameterErr.message
		}
		response.WriteError(w, r, http.StatusUnprocessableEntity, "UNPROCESSABLE_ENTITY", "invalid report query", []response.ErrorDetail{{Field: field, Message: message}})
		return
	}

	page, err := h.useCase.Report(r.Context(), query)
	if err != nil {
		if errors.Is(err, usecase.ErrUtilizationRangeRequired) || errors.Is(err, usecase.ErrUtilizationRangeInvalid) || errors.Is(err, usecase.ErrUtilizationPagination) || errors.Is(err, usecase.ErrUtilizationFilter) {
			response.WriteError(w, r, http.StatusUnprocessableEntity, "UNPROCESSABLE_ENTITY", err.Error(), nil)
			return
		}
		response.WriteError(w, r, http.StatusInternalServerError, "INTERNAL_SERVER_ERROR", "failed to retrieve environment utilization", nil)
		return
	}
	response.WriteJSON(w, http.StatusOK, dto.FromEnvironmentUtilizationPage(*page))
}

type utilizationQueryError struct {
	field   string
	message string
}

func (e *utilizationQueryError) Error() string { return e.message }

func parseEnvironmentUtilizationQuery(r *http.Request) (port.EnvironmentUtilizationQuery, error) {
	values := r.URL.Query()
	query := port.EnvironmentUtilizationQuery{Page: 1, PageSize: 20}

	fromValue := strings.TrimSpace(values.Get("from"))
	toValue := strings.TrimSpace(values.Get("to"))
	if fromValue == "" {
		return query, &utilizationQueryError{field: "from", message: "from is required"}
	}
	if toValue == "" {
		return query, &utilizationQueryError{field: "to", message: "to is required"}
	}
	var err error
	query.From, err = time.Parse(time.DateOnly, fromValue)
	if err != nil {
		return query, &utilizationQueryError{field: "from", message: "from must be a valid date (YYYY-MM-DD)"}
	}
	query.To, err = time.Parse(time.DateOnly, toValue)
	if err != nil {
		return query, &utilizationQueryError{field: "to", message: "to must be a valid date (YYYY-MM-DD)"}
	}
	if !query.From.Before(query.To) {
		return query, &utilizationQueryError{field: "to", message: "from must be before to"}
	}

	if raw := strings.TrimSpace(values.Get("page")); raw != "" {
		query.Page, err = strconv.Atoi(raw)
		if err != nil || query.Page < 1 {
			return query, &utilizationQueryError{field: "page", message: "page must be an integer greater than 0"}
		}
	}
	if raw := strings.TrimSpace(values.Get("page_size")); raw != "" {
		query.PageSize, err = strconv.Atoi(raw)
		if err != nil || query.PageSize < 1 || query.PageSize > 100 {
			return query, &utilizationQueryError{field: "page_size", message: "page_size must be an integer between 1 and 100"}
		}
	}
	query.TrainingCenterID = strings.TrimSpace(values.Get("training_center_id"))
	query.EnvironmentID = strings.TrimSpace(values.Get("environment_id"))
	query.EnvironmentTypeID = strings.TrimSpace(values.Get("environment_type_id"))
	return query, nil
}
