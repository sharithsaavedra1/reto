package http_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	httpAdapter "github.com/code-sena/design-software-training-environment-service/internal/adapter/in/http"
	"github.com/code-sena/design-software-training-environment-service/internal/adapter/in/http/dto"
	"github.com/code-sena/design-software-training-environment-service/internal/adapter/in/http/middleware"
	"github.com/code-sena/design-software-training-environment-service/internal/adapter/in/http/response"
	"github.com/code-sena/design-software-training-environment-service/internal/adapter/out/persistence"
	"github.com/code-sena/design-software-training-environment-service/internal/application/usecase"
	"github.com/code-sena/design-software-training-environment-service/internal/domain/model"
)

const (
	reservationEnvironmentID = "78901234-5678-9012-3456-789012345678"
	reservationRequesterID   = "d8231174-567b-40b0-8ab5-4af992406cf6"
)

func setupReservationServer() *http.ServeMux {
	envRepo := persistence.NewMemoryEnvironmentRepository()
	reservationRepo := persistence.NewMemoryReservationRepository(envRepo)
	uc := usecase.NewReservationUseCase(reservationRepo, envRepo)
	auth := middleware.NewAuthMiddleware(testJWTSecret)
	mux := http.NewServeMux()
	httpAdapter.NewReservationHandler(uc).RegisterRoutes(mux, auth)
	return mux
}

func reservationRequest(t *testing.T, mux *http.ServeMux, body string, token string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/reservations", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

func TestReservationHandler_HU007_CreateAndAntiOverlap(t *testing.T) {
	mux := setupReservationServer()
	token := createTestTokenWithSubject([]string{"ENV_RESERVATION_MANAGE"}, reservationRequesterID, testJWTSecret, false)

	first := reservationRequest(t, mux, `{"environment_id":"`+reservationEnvironmentID+`","reservation_date":"2026-09-20","start_time":"08:00","end_time":"10:00","reason":"Reunión"}`, token)
	if first.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d body=%s", first.Code, first.Body.String())
	}
	var created dto.ReservationResponse
	if err := json.Unmarshal(first.Body.Bytes(), &created); err != nil {
		t.Fatalf("invalid response JSON: %v", err)
	}
	if created.Status != model.ReservationStatusPending || created.RequesterID != reservationRequesterID || created.StartTime != "08:00:00" || created.EndTime != "10:00:00" {
		t.Fatalf("unexpected reservation response: %+v", created)
	}

	overlap := reservationRequest(t, mux, `{"environment_id":"`+reservationEnvironmentID+`","reservation_date":"2026-09-20","start_time":"09:00","end_time":"11:00"}`, token)
	if overlap.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d body=%s", overlap.Code, overlap.Body.String())
	}
	var envelope response.ErrorEnvelope
	if err := json.Unmarshal(overlap.Body.Bytes(), &envelope); err != nil || envelope.Error.Code != "RESERVATION_OVERLAP" {
		t.Fatalf("expected RESERVATION_OVERLAP envelope, got %s err=%v", overlap.Body.String(), err)
	}

	// Adjacent ranges do not overlap under PostgreSQL tsrange [start,end) semantics.
	adjacent := reservationRequest(t, mux, `{"environment_id":"`+reservationEnvironmentID+`","reservation_date":"2026-09-20","start_time":"10:00","end_time":"11:00"}`, token)
	if adjacent.Code != http.StatusCreated {
		t.Fatalf("expected adjacent reservation to be accepted with 201, got %d body=%s", adjacent.Code, adjacent.Body.String())
	}
}

func TestReservationHandler_HU007_EnvironmentValidationAndSecurity(t *testing.T) {
	token := createTestTokenWithSubject([]string{"ENV_RESERVATION_MANAGE"}, reservationRequesterID, testJWTSecret, false)
	validBody := `{"environment_id":"` + reservationEnvironmentID + `","reservation_date":"2026-09-21","start_time":"08:00","end_time":"10:00"}`

	t.Run("environment does not exist", func(t *testing.T) {
		mux := setupReservationServer()
		body := `{"environment_id":"11111111-2222-4333-8444-555555555555","reservation_date":"2026-09-21","start_time":"08:00","end_time":"10:00"}`
		rec := reservationRequest(t, mux, body, token)
		if rec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("expected 422, got %d body=%s", rec.Code, rec.Body.String())
		}
	})

	t.Run("missing authentication", func(t *testing.T) {
		rec := reservationRequest(t, setupReservationServer(), validBody, "")
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d", rec.Code)
		}
	})

	t.Run("missing feature", func(t *testing.T) {
		viewOnly := createTestTokenWithSubject([]string{"ENV_VIEW"}, reservationRequesterID, testJWTSecret, false)
		rec := reservationRequest(t, setupReservationServer(), validBody, viewOnly)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("expected 403, got %d body=%s", rec.Code, rec.Body.String())
		}
	})

	t.Run("requester comes from valid JWT subject", func(t *testing.T) {
		badSubjectToken := createTestTokenWithSubject([]string{"ENV_RESERVATION_MANAGE"}, "user-123", testJWTSecret, false)
		rec := reservationRequest(t, setupReservationServer(), validBody, badSubjectToken)
		if rec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("expected invalid requester subject to return 422, got %d body=%s", rec.Code, rec.Body.String())
		}
	})
}

func TestReservationHandler_HU007_Validation(t *testing.T) {
	token := createTestTokenWithSubject([]string{"ENV_RESERVATION_MANAGE"}, reservationRequesterID, testJWTSecret, false)
	tests := map[string]string{
		"missing date":     `{"environment_id":"` + reservationEnvironmentID + `","start_time":"08:00","end_time":"10:00"}`,
		"bad date":         `{"environment_id":"` + reservationEnvironmentID + `","reservation_date":"20-09-2026","start_time":"08:00","end_time":"10:00"}`,
		"bad start":        `{"environment_id":"` + reservationEnvironmentID + `","reservation_date":"2026-09-20","start_time":"25:00","end_time":"10:00"}`,
		"missing end":      `{"environment_id":"` + reservationEnvironmentID + `","reservation_date":"2026-09-20","start_time":"08:00"}`,
		"equal times":      `{"environment_id":"` + reservationEnvironmentID + `","reservation_date":"2026-09-20","start_time":"08:00","end_time":"08:00"}`,
		"reversed times":   `{"environment_id":"` + reservationEnvironmentID + `","reservation_date":"2026-09-20","start_time":"10:00","end_time":"08:00"}`,
		"invalid env UUID": `{"environment_id":"bad-id","reservation_date":"2026-09-20","start_time":"08:00","end_time":"10:00"}`,
		"reason too long":  `{"environment_id":"` + reservationEnvironmentID + `","reservation_date":"2026-09-20","start_time":"08:00","end_time":"10:00","reason":"` + strings.Repeat("x", 256) + `"}`,
	}
	for name, body := range tests {
		t.Run(name, func(t *testing.T) {
			rec := reservationRequest(t, setupReservationServer(), body, token)
			if rec.Code != http.StatusUnprocessableEntity {
				t.Fatalf("expected 422, got %d body=%s", rec.Code, rec.Body.String())
			}
		})
	}
}
