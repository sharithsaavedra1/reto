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

type ReservationHandler struct {
	useCase port.ReservationUseCase
}

func NewReservationHandler(useCase port.ReservationUseCase) *ReservationHandler {
	return &ReservationHandler{useCase: useCase}
}

func (h *ReservationHandler) RegisterRoutes(mux *http.ServeMux, auth *middleware.AuthMiddleware) {
	mux.HandleFunc("POST /reservations", auth.RequireFeature("ENV_RESERVATION_MANAGE", h.Create))
}

func (h *ReservationHandler) Create(w http.ResponseWriter, r *http.Request) {
	var request dto.CreateReservationRequest
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(&request); err != nil {
		writeReservationValidationError(w, r, "Payload JSON inválido.", nil)
		return
	}

	claims, _ := r.Context().Value(middleware.ClaimsContextKey).(*middleware.JWTClaims)
	requesterID := ""
	if claims != nil {
		requesterID = strings.TrimSpace(claims.Subject)
	}

	command, err := request.Command(requesterID)
	if err != nil {
		writeReservationValidationError(w, r, "reservation_date debe usar YYYY-MM-DD y start_time/end_time deben usar HH:MM o HH:MM:SS.", nil)
		return
	}

	created, err := h.useCase.Create(r.Context(), command)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	response.WriteJSON(w, http.StatusCreated, dto.FromReservationModel(*created))
}

func (h *ReservationHandler) writeError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, usecase.ErrReservationOverlap):
		response.WriteError(w, r, http.StatusConflict, "RESERVATION_OVERLAP", "El ambiente ya está reservado en ese rango.", []map[string]string{
			{"field": "reservation_date", "issue": "OVERLAPS_EXISTING_RESERVATION"},
		})
	case errors.Is(err, usecase.ErrReservationInvalidEnvironment):
		response.WriteError(w, r, http.StatusUnprocessableEntity, "UNPROCESSABLE_ENTITY", "El ambiente de formación referenciado no existe.", []map[string]string{
			{"field": "environment_id", "issue": "INVALID_REFERENCE"},
		})
	case errors.Is(err, usecase.ErrReservationValidation),
		errors.Is(err, model.ErrInvalidReservationEnvironmentID),
		errors.Is(err, model.ErrInvalidReservationRequesterID),
		errors.Is(err, model.ErrInvalidReservationDate),
		errors.Is(err, model.ErrInvalidReservationStartTime),
		errors.Is(err, model.ErrInvalidReservationEndTime),
		errors.Is(err, model.ErrInvalidReservationTimeRange),
		errors.Is(err, model.ErrReservationReasonTooLong):
		writeReservationValidationError(w, r, err.Error(), nil)
	default:
		response.WriteError(w, r, http.StatusInternalServerError, "INTERNAL_SERVER_ERROR", "No fue posible crear la reserva.", nil)
	}
}

func writeReservationValidationError(w http.ResponseWriter, r *http.Request, message string, details interface{}) {
	response.WriteError(w, r, http.StatusUnprocessableEntity, "UNPROCESSABLE_ENTITY", message, details)
}
