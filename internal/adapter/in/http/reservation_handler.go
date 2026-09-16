package http

import (
	"encoding/json"
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
	"github.com/code-sena/design-software-training-environment-service/internal/domain/model"
)

type ReservationHandler struct {
	useCase port.ReservationUseCase
}

func NewReservationHandler(useCase port.ReservationUseCase) *ReservationHandler {
	return &ReservationHandler{useCase: useCase}
}

func (h *ReservationHandler) RegisterRoutes(mux *http.ServeMux, auth *middleware.AuthMiddleware) {
	mux.HandleFunc("GET /reservations", auth.RequireFeature("ENV_VIEW", h.List))
	mux.HandleFunc("GET /reservations/{id}", auth.RequireFeature("ENV_VIEW", h.Get))
	mux.HandleFunc("POST /reservations", auth.RequireFeature("ENV_RESERVATION_MANAGE", h.Create))
}

func (h *ReservationHandler) List(w http.ResponseWriter, r *http.Request) {
	query, err := parseReservationListQuery(r)
	if err != nil {
		p, ok := err.(*queryParameterError)
		if !ok {
			p = &queryParameterError{field: "query", msg: err.Error()}
		}
		writeReservationBadRequest(w, r, "Parámetros de consulta inválidos.", p.field, "INVALID_QUERY_PARAMETER")
		return
	}

	trainingCenterID, err := reservationTrainingCenterScope(r)
	if err != nil {
		writeReservationScopeError(w, r)
		return
	}
	query.TrainingCenterID = trainingCenterID

	page, err := h.useCase.Query(r.Context(), query)
	if err != nil {
		if errors.Is(err, usecase.ErrReservationQuery) {
			writeReservationBadRequest(w, r, "Parámetros de consulta inválidos.", "query", "INVALID_QUERY_PARAMETER")
			return
		}
		response.WriteError(w, r, http.StatusInternalServerError, "INTERNAL_SERVER_ERROR", "No fue posible consultar las reservas.", nil)
		return
	}
	response.WriteJSON(w, http.StatusOK, dto.FromReservationPage(*page))
}

func (h *ReservationHandler) Get(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		if len(parts) >= 2 {
			id = strings.TrimSpace(parts[1])
		}
	}

	trainingCenterID, err := reservationTrainingCenterScope(r)
	if err != nil {
		writeReservationScopeError(w, r)
		return
	}
	item, err := h.useCase.GetByID(r.Context(), id, trainingCenterID)
	if err != nil {
		if errors.Is(err, usecase.ErrReservationNotFound) {
			response.WriteError(w, r, http.StatusNotFound, "NOT_FOUND", "Reserva no encontrada.", nil)
			return
		}
		if errors.Is(err, usecase.ErrReservationQuery) {
			writeReservationBadRequest(w, r, "Identificador de reserva inválido.", "id", "INVALID_UUID")
			return
		}
		response.WriteError(w, r, http.StatusInternalServerError, "INTERNAL_SERVER_ERROR", "No fue posible consultar la reserva.", nil)
		return
	}
	response.WriteJSON(w, http.StatusOK, dto.FromReservationModel(*item))
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

func parseReservationListQuery(r *http.Request) (port.ReservationListQuery, error) {
	values := r.URL.Query()
	query := port.ReservationListQuery{Page: 1, PageSize: 20, SortField: "created_at", SortDescending: true}

	if raw, ok := values["page"]; ok {
		value, err := strconv.Atoi(firstQueryValue(raw))
		if err != nil || value < 1 {
			return query, &queryParameterError{field: "page", msg: "page must be an integer greater than 0"}
		}
		query.Page = value
	}
	if raw, ok := values["page_size"]; ok {
		value, err := strconv.Atoi(firstQueryValue(raw))
		if err != nil || value < 1 || value > 100 {
			return query, &queryParameterError{field: "page_size", msg: "page_size must be an integer between 1 and 100"}
		}
		query.PageSize = value
	}
	if raw, ok := values["sort"]; ok {
		parts := strings.Split(firstQueryValue(raw), ":")
		if len(parts) != 2 || (parts[1] != "asc" && parts[1] != "desc") {
			return query, &queryParameterError{field: "sort", msg: "sort must use <field>:<asc|desc>"}
		}
		query.SortField = parts[0]
		query.SortDescending = parts[1] == "desc"
	}
	if raw, ok := values["environment_id"]; ok {
		query.EnvironmentID = strings.TrimSpace(firstQueryValue(raw))
		if !model.IsValidUUID(query.EnvironmentID) {
			return query, &queryParameterError{field: "environment_id", msg: "environment_id must be a valid RFC 4122 UUID"}
		}
	}
	if raw, ok := values["requester_id"]; ok {
		query.RequesterID = strings.TrimSpace(firstQueryValue(raw))
		if !model.IsValidUUID(query.RequesterID) {
			return query, &queryParameterError{field: "requester_id", msg: "requester_id must be a valid RFC 4122 UUID"}
		}
	}
	if raw, ok := values["status"]; ok {
		query.Status = strings.TrimSpace(firstQueryValue(raw))
		switch query.Status {
		case model.ReservationStatusPending, model.ReservationStatusConfirmed, model.ReservationStatusCancelled:
		default:
			return query, &queryParameterError{field: "status", msg: "status must be PENDING, CONFIRMED or CANCELLED"}
		}
	}
	if raw, ok := values["from"]; ok {
		value, err := time.Parse(time.DateOnly, firstQueryValue(raw))
		if err != nil {
			return query, &queryParameterError{field: "from", msg: "from must use YYYY-MM-DD"}
		}
		query.From = &value
	}
	if raw, ok := values["to"]; ok {
		value, err := time.Parse(time.DateOnly, firstQueryValue(raw))
		if err != nil {
			return query, &queryParameterError{field: "to", msg: "to must use YYYY-MM-DD"}
		}
		query.To = &value
	}
	if query.From != nil && query.To != nil && !query.From.Before(*query.To) {
		return query, &queryParameterError{field: "to", msg: "to must be after from"}
	}
	return query, nil
}

func reservationTrainingCenterScope(r *http.Request) (string, error) {
	claims, _ := r.Context().Value(middleware.ClaimsContextKey).(*middleware.JWTClaims)
	if claims == nil {
		return "", errors.New("missing claims")
	}
	scope := claims.FeatureScope("ENV_VIEW")
	switch scope {
	case "", "GLOBAL":
		return "", nil
	case "TRAINING_CENTER":
		centerID := strings.TrimSpace(claims.TrainingCenterID)
		if !model.IsValidUUID(centerID) {
			return "", errors.New("missing training center scope")
		}
		return centerID, nil
	default:
		return "", errors.New("unsupported ENV_VIEW scope")
	}
}

func writeReservationBadRequest(w http.ResponseWriter, r *http.Request, message, field, issue string) {
	response.WriteError(w, r, http.StatusBadRequest, "BAD_REQUEST", message, []map[string]string{{"field": field, "issue": issue}})
}

func writeReservationScopeError(w http.ResponseWriter, r *http.Request) {
	response.WriteError(w, r, http.StatusForbidden, "FORBIDDEN", "El alcance de ENV_VIEW no permite consultar estas reservas.", []map[string]string{{"field": "scope", "issue": "INVALID_SCOPE"}})
}

func writeReservationValidationError(w http.ResponseWriter, r *http.Request, message string, details interface{}) {
	response.WriteError(w, r, http.StatusUnprocessableEntity, "UNPROCESSABLE_ENTITY", message, details)
}
