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

func reservationPatchRequest(t *testing.T, mux *http.ServeMux, id string, body string, token string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPatch, "/reservations/"+id, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

func reservationDeleteRequest(t *testing.T, mux *http.ServeMux, id string, token string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodDelete, "/reservations/"+id, nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

func TestReservationHandler_HU009_UpdateReservation(t *testing.T) {
	mux := setupReservationServer()
	token := createTestTokenWithSubject([]string{"ENV_RESERVATION_MANAGE"}, reservationRequesterID, testJWTSecret, false)

	// Create initial reservation 1
	createRes1 := reservationRequest(t, mux, `{"environment_id":"`+reservationEnvironmentID+`","reservation_date":"2026-09-20","start_time":"08:00","end_time":"10:00","reason":"Inicial"}`, token)
	if createRes1.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", createRes1.Code)
	}
	var res1 dto.ReservationResponse
	_ = json.Unmarshal(createRes1.Body.Bytes(), &res1)

	// Create initial reservation 2 (different slot)
	createRes2 := reservationRequest(t, mux, `{"environment_id":"`+reservationEnvironmentID+`","reservation_date":"2026-09-20","start_time":"14:00","end_time":"16:00"}`, token)
	if createRes2.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", createRes2.Code)
	}
	var res2 dto.ReservationResponse
	_ = json.Unmarshal(createRes2.Body.Bytes(), &res2)

	t.Run("successful patch update", func(t *testing.T) {
		patchBody := `{"reservation_date":"2026-09-21","start_time":"09:00","end_time":"11:00","reason":"Reprogramada","status":"CONFIRMED"}`
		rec := reservationPatchRequest(t, mux, res1.ID, patchBody, token)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
		}
		var updated dto.ReservationResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &updated); err != nil {
			t.Fatalf("invalid json: %v", err)
		}
		if updated.ReservationDate != "2026-09-21" || updated.StartTime != "09:00:00" || updated.EndTime != "11:00:00" || updated.Status != model.ReservationStatusConfirmed {
			t.Fatalf("unexpected updated fields: %+v", updated)
		}
		if updated.Reason == nil || *updated.Reason != "Reprogramada" {
			t.Fatalf("unexpected reason: %v", updated.Reason)
		}
	})

	t.Run("conflict on overlap with existing active reservation", func(t *testing.T) {
		// Try to move res2 into 2026-09-21 09:30-10:30 which overlaps with res1 (09:00-11:00)
		patchBody := `{"reservation_date":"2026-09-21","start_time":"09:30","end_time":"10:30"}`
		rec := reservationPatchRequest(t, mux, res2.ID, patchBody, token)
		if rec.Code != http.StatusConflict {
			t.Fatalf("expected 409 Conflict, got %d body=%s", rec.Code, rec.Body.String())
		}
		var envelope response.ErrorEnvelope
		if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil || envelope.Error.Code != "RESERVATION_OVERLAP" {
			t.Fatalf("expected RESERVATION_OVERLAP, got %s", rec.Body.String())
		}
	})

	t.Run("validation invalid status", func(t *testing.T) {
		patchBody := `{"status":"UNKNOWN"}`
		rec := reservationPatchRequest(t, mux, res1.ID, patchBody, token)
		if rec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("expected 422, got %d body=%s", rec.Code, rec.Body.String())
		}
	})

	t.Run("validation invalid time range", func(t *testing.T) {
		patchBody := `{"start_time":"12:00","end_time":"10:00"}`
		rec := reservationPatchRequest(t, mux, res1.ID, patchBody, token)
		if rec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("expected 422, got %d body=%s", rec.Code, rec.Body.String())
		}
	})

	t.Run("validation reason too long", func(t *testing.T) {
		patchBody := `{"reason":"` + strings.Repeat("a", 256) + `"}`
		rec := reservationPatchRequest(t, mux, res1.ID, patchBody, token)
		if rec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("expected 422, got %d body=%s", rec.Code, rec.Body.String())
		}
	})

	t.Run("not found for non-existent UUID", func(t *testing.T) {
		patchBody := `{"status":"CONFIRMED"}`
		rec := reservationPatchRequest(t, mux, "99999999-9999-4999-8999-999999999999", patchBody, token)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d body=%s", rec.Code, rec.Body.String())
		}
	})

	t.Run("not found for malformed UUID", func(t *testing.T) {
		patchBody := `{"status":"CONFIRMED"}`
		rec := reservationPatchRequest(t, mux, "invalid-uuid-format", patchBody, token)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d body=%s", rec.Code, rec.Body.String())
		}
	})

	t.Run("security unauthenticated", func(t *testing.T) {
		patchBody := `{"status":"CONFIRMED"}`
		rec := reservationPatchRequest(t, mux, res1.ID, patchBody, "")
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d", rec.Code)
		}
	})

	t.Run("security forbidden missing feature", func(t *testing.T) {
		viewOnly := createTestTokenWithSubject([]string{"ENV_VIEW"}, reservationRequesterID, testJWTSecret, false)
		patchBody := `{"status":"CONFIRMED"}`
		rec := reservationPatchRequest(t, mux, res1.ID, patchBody, viewOnly)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("expected 403, got %d", rec.Code)
		}
	})
}

func TestReservationHandler_HU009_CancelReservation(t *testing.T) {
	mux := setupReservationServer()
	token := createTestTokenWithSubject([]string{"ENV_RESERVATION_MANAGE"}, reservationRequesterID, testJWTSecret, false)

	createRes := reservationRequest(t, mux, `{"environment_id":"`+reservationEnvironmentID+`","reservation_date":"2026-09-25","start_time":"08:00","end_time":"10:00"}`, token)
	if createRes.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", createRes.Code)
	}
	var res dto.ReservationResponse
	_ = json.Unmarshal(createRes.Body.Bytes(), &res)

	t.Run("cancel active reservation returns 204", func(t *testing.T) {
		rec := reservationDeleteRequest(t, mux, res.ID, token)
		if rec.Code != http.StatusNoContent {
			t.Fatalf("expected 204 No Content, got %d body=%s", rec.Code, rec.Body.String())
		}
	})

	t.Run("cancel already cancelled reservation is idempotent and returns 204", func(t *testing.T) {
		rec := reservationDeleteRequest(t, mux, res.ID, token)
		if rec.Code != http.StatusNoContent {
			t.Fatalf("expected 204 No Content on idempotent delete, got %d body=%s", rec.Code, rec.Body.String())
		}
	})

	t.Run("cancel non-existent reservation returns 404", func(t *testing.T) {
		rec := reservationDeleteRequest(t, mux, "99999999-9999-4999-8999-999999999999", token)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d body=%s", rec.Code, rec.Body.String())
		}
	})

	t.Run("cancel malformed UUID returns 404", func(t *testing.T) {
		rec := reservationDeleteRequest(t, mux, "not-a-valid-uuid", token)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d body=%s", rec.Code, rec.Body.String())
		}
	})

	t.Run("security unauthenticated", func(t *testing.T) {
		rec := reservationDeleteRequest(t, mux, res.ID, "")
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d", rec.Code)
		}
	})

	t.Run("security forbidden missing feature", func(t *testing.T) {
		viewOnly := createTestTokenWithSubject([]string{"ENV_VIEW"}, reservationRequesterID, testJWTSecret, false)
		rec := reservationDeleteRequest(t, mux, res.ID, viewOnly)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("expected 403, got %d", rec.Code)
		}
	})
}
