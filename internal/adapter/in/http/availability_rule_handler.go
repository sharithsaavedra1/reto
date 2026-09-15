package http

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/code-sena/design-software-training-environment-service/internal/adapter/in/http/dto"
	"github.com/code-sena/design-software-training-environment-service/internal/adapter/in/http/middleware"
	"github.com/code-sena/design-software-training-environment-service/internal/adapter/in/http/response"
	"github.com/code-sena/design-software-training-environment-service/internal/application/port"
	"github.com/code-sena/design-software-training-environment-service/internal/application/usecase"
	"github.com/code-sena/design-software-training-environment-service/internal/domain/model"
)

type AvailabilityRuleHandler struct {
	useCase port.AvailabilityRuleUseCase
}

func NewAvailabilityRuleHandler(useCase port.AvailabilityRuleUseCase) *AvailabilityRuleHandler {
	return &AvailabilityRuleHandler{useCase: useCase}
}

func (h *AvailabilityRuleHandler) RegisterRoutes(mux *http.ServeMux, auth *middleware.AuthMiddleware) {
	mux.HandleFunc("GET /training-environments/{id}/availability-rules", auth.RequireFeature("ENV_VIEW", h.List))
	mux.HandleFunc("POST /training-environments/{id}/availability-rules", auth.RequireFeature("ENV_AVAILABILITY_QUERY", h.Create))
	mux.HandleFunc("GET /training-environments/{id}/availability-rules/{rule_id}", auth.RequireFeature("ENV_VIEW", h.Get))
	mux.HandleFunc("PUT /training-environments/{id}/availability-rules/{rule_id}", auth.RequireFeature("ENV_AVAILABILITY_QUERY", h.Update))
	mux.HandleFunc("DELETE /training-environments/{id}/availability-rules/{rule_id}", auth.RequireFeature("ENV_AVAILABILITY_QUERY", h.Delete))
}

func (h *AvailabilityRuleHandler) List(w http.ResponseWriter, r *http.Request) {
	environmentID := pathValue(r, "id")
	if !model.IsValidUUID(environmentID) {
		writeInvalidRulePath(w, r, "id")
		return
	}
	var dayOfWeek *int
	if raw := r.URL.Query().Get("day_of_week"); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 1 || value > 7 {
			writeRuleValidationError(w, r, "day_of_week must be between 1 and 7")
			return
		}
		dayOfWeek = &value
	}
	rules, err := h.useCase.List(r.Context(), environmentID, dayOfWeek)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	response.WriteJSON(w, http.StatusOK, dto.FromAvailabilityRuleModels(rules))
}

func (h *AvailabilityRuleHandler) Create(w http.ResponseWriter, r *http.Request) {
	environmentID := pathValue(r, "id")
	if !model.IsValidUUID(environmentID) {
		writeInvalidRulePath(w, r, "id")
		return
	}
	var req dto.AvailabilityRuleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeRuleValidationError(w, r, "invalid JSON payload")
		return
	}
	command, err := req.Command(environmentID)
	if err != nil {
		writeRuleValidationError(w, r, "invalid availability rule date or time format")
		return
	}
	created, err := h.useCase.Create(r.Context(), command)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	response.WriteJSON(w, http.StatusCreated, dto.FromAvailabilityRuleModel(*created))
}

func (h *AvailabilityRuleHandler) Get(w http.ResponseWriter, r *http.Request) {
	environmentID, ruleID, valid := h.rulePath(r)
	if !valid {
		writeInvalidRulePath(w, r, "id")
		return
	}
	rule, err := h.useCase.Get(r.Context(), environmentID, ruleID)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	response.WriteJSON(w, http.StatusOK, dto.FromAvailabilityRuleModel(*rule))
}

func (h *AvailabilityRuleHandler) Update(w http.ResponseWriter, r *http.Request) {
	environmentID, ruleID, valid := h.rulePath(r)
	if !valid {
		writeInvalidRulePath(w, r, "id")
		return
	}
	var req dto.AvailabilityRuleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeRuleValidationError(w, r, "invalid JSON payload")
		return
	}
	command, err := req.Command(environmentID)
	if err != nil {
		writeRuleValidationError(w, r, "invalid availability rule date or time format")
		return
	}
	updated, err := h.useCase.Update(r.Context(), environmentID, ruleID, command)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	response.WriteJSON(w, http.StatusOK, dto.FromAvailabilityRuleModel(*updated))
}

func (h *AvailabilityRuleHandler) Delete(w http.ResponseWriter, r *http.Request) {
	environmentID, ruleID, valid := h.rulePath(r)
	if !valid {
		writeInvalidRulePath(w, r, "id")
		return
	}
	if err := h.useCase.Delete(r.Context(), environmentID, ruleID); err != nil {
		h.writeError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *AvailabilityRuleHandler) rulePath(r *http.Request) (string, string, bool) {
	environmentID := pathValue(r, "id")
	ruleID := pathValue(r, "rule_id")
	return environmentID, ruleID, model.IsValidUUID(environmentID) && model.IsValidUUID(ruleID)
}

func (h *AvailabilityRuleHandler) writeError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, usecase.ErrEnvironmentNotFound), errors.Is(err, usecase.ErrAvailabilityRuleNotFound):
		response.WriteError(w, r, http.StatusNotFound, "NOT_FOUND", "availability rule or training environment not found", nil)
	case errors.Is(err, usecase.ErrAvailabilityRuleLimit):
		response.WriteError(w, r, http.StatusUnprocessableEntity, "UNPROCESSABLE_ENTITY", err.Error(), nil)
	case errors.Is(err, usecase.ErrAvailabilityValidation), errors.Is(err, model.ErrInvalidAvailabilityDay), errors.Is(err, model.ErrInvalidAvailabilityTimeRange), errors.Is(err, model.ErrInvalidAvailabilityDateRange):
		response.WriteError(w, r, http.StatusUnprocessableEntity, "UNPROCESSABLE_ENTITY", err.Error(), nil)
	default:
		response.WriteError(w, r, http.StatusInternalServerError, "INTERNAL_SERVER_ERROR", "failed to process availability rule", nil)
	}
}

func writeInvalidRulePath(w http.ResponseWriter, r *http.Request, field string) {
	response.WriteError(w, r, http.StatusUnprocessableEntity, "UNPROCESSABLE_ENTITY", "invalid availability rule path", []response.ErrorDetail{{Field: field, Message: "must be a valid RFC 4122 UUID"}})
}

func writeRuleValidationError(w http.ResponseWriter, r *http.Request, message string) {
	response.WriteError(w, r, http.StatusUnprocessableEntity, "UNPROCESSABLE_ENTITY", message, nil)
}

func pathValue(r *http.Request, name string) string {
	value := r.PathValue(name)
	if value != "" {
		return strings.TrimSpace(value)
	}
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	for i := range parts {
		if parts[i] == "training-environments" && i+1 < len(parts) && name == "id" {
			return parts[i+1]
		}
		if parts[i] == "availability-rules" && i+1 < len(parts) && name == "rule_id" {
			return parts[i+1]
		}
	}
	return ""
}
