package http_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	httpAdapter "github.com/code-sena/design-software-training-environment-service/internal/adapter/in/http"
	"github.com/code-sena/design-software-training-environment-service/internal/adapter/in/http/dto"
	"github.com/code-sena/design-software-training-environment-service/internal/adapter/in/http/middleware"
	"github.com/code-sena/design-software-training-environment-service/internal/adapter/out/persistence"
	"github.com/code-sena/design-software-training-environment-service/internal/application/port"
	"github.com/code-sena/design-software-training-environment-service/internal/application/usecase"
	"github.com/code-sena/design-software-training-environment-service/internal/domain/model"
)

const (
	hu008CenterA = "c1d2e3f4-a5b6-7890-1234-56789abcdef0"
	hu008CenterB = "d1d2e3f4-a5b6-7890-1234-56789abcdef0"
	hu008EnvA    = "78901234-5678-9012-3456-789012345678"
	hu008EnvB    = "89012345-6789-0123-4567-890123456789"
	hu008EnvC    = "99012345-6789-4123-8567-890123456780"
	hu008ReqA    = "d8231174-567b-40b0-8ab5-4af992406cf6"
	hu008ReqB    = "e8231174-567b-40b0-8ab5-4af992406cf6"
	hu008Res1    = "11111111-1111-4111-8111-111111111111"
	hu008Res2    = "22222222-2222-4222-8222-222222222222"
	hu008Res3    = "33333333-3333-4333-8333-333333333333"
	hu008Res4    = "44444444-4444-4444-8444-444444444444"
)

func setupReservationQueryServer(t *testing.T) *http.ServeMux {
	t.Helper()
	envRepo := persistence.NewMemoryEnvironmentRepository()
	now := time.Now().UTC()
	if err := envRepo.Save(context.Background(), &model.Environment{
		ID: hu008EnvC, EnvironmentTypeID: "347d47f9-1ba5-4b9c-89b7-892112a23d90",
		TrainingCenterID: hu008CenterB, Name: "HU008 Other Center", Capacity: 20,
		IsActive: true, CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("seed environment: %v", err)
	}

	repo := persistence.NewMemoryReservationRepository(envRepo)
	seed := []model.Reservation{
		{ID: hu008Res1, EnvironmentID: hu008EnvA, RequesterID: hu008ReqA, ReservationDate: time.Date(2026, 9, 20, 0, 0, 0, 0, time.UTC), StartTime: "08:00:00", EndTime: "09:00:00", Status: model.ReservationStatusPending, CreatedAt: now.Add(-4 * time.Hour), UpdatedAt: now.Add(-4 * time.Hour)},
		{ID: hu008Res2, EnvironmentID: hu008EnvA, RequesterID: hu008ReqB, ReservationDate: time.Date(2026, 9, 21, 0, 0, 0, 0, time.UTC), StartTime: "10:00:00", EndTime: "11:00:00", Status: model.ReservationStatusCancelled, CreatedAt: now.Add(-3 * time.Hour), UpdatedAt: now.Add(-3 * time.Hour)},
		{ID: hu008Res3, EnvironmentID: hu008EnvB, RequesterID: hu008ReqA, ReservationDate: time.Date(2026, 9, 22, 0, 0, 0, 0, time.UTC), StartTime: "12:00:00", EndTime: "13:00:00", Status: model.ReservationStatusConfirmed, CreatedAt: now.Add(-2 * time.Hour), UpdatedAt: now.Add(-2 * time.Hour)},
		{ID: hu008Res4, EnvironmentID: hu008EnvC, RequesterID: hu008ReqA, ReservationDate: time.Date(2026, 9, 23, 0, 0, 0, 0, time.UTC), StartTime: "14:00:00", EndTime: "15:00:00", Status: model.ReservationStatusPending, CreatedAt: now.Add(-1 * time.Hour), UpdatedAt: now.Add(-1 * time.Hour)},
	}
	for i := range seed {
		if err := repo.Create(context.Background(), &seed[i]); err != nil {
			t.Fatalf("seed reservation: %v", err)
		}
	}

	mux := http.NewServeMux()
	auth := middleware.NewAuthMiddleware(testJWTSecret)
	httpAdapter.NewReservationHandler(usecaseForReservationQuery(repo, envRepo)).RegisterRoutes(mux, auth)
	return mux
}

// helper keeps the setup focused while returning the real application use case.
func usecaseForReservationQuery(repo port.ReservationRepository, envRepo port.EnvironmentRepository) port.ReservationUseCase {
	return usecase.NewReservationUseCase(repo, envRepo)
}

func hu008GET(t *testing.T, mux *http.ServeMux, path, token string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

func TestReservationHandler_HU008_ListFiltersPaginationAndEmpty(t *testing.T) {
	mux := setupReservationQueryServer(t)
	token := createTestTokenWithSubject([]string{"ENV_VIEW"}, hu008ReqA, testJWTSecret, false)

	rec := hu008GET(t, mux, "/reservations?page=1&page_size=2&sort=reservation_date:asc", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var page dto.ReservationListResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(page.Data) != 2 || page.Pagination.TotalItems != 4 || page.Pagination.TotalPages != 2 {
		t.Fatalf("unexpected pagination: %+v", page)
	}

	rec = hu008GET(t, mux, "/reservations?environment_id="+hu008EnvA+"&requester_id="+hu008ReqB+"&status=CANCELLED&from=2026-09-20&to=2026-09-22", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected filtered 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil {
		t.Fatalf("decode filtered list: %v", err)
	}
	if len(page.Data) != 1 || page.Data[0].ID != hu008Res2 || page.Data[0].Status != model.ReservationStatusCancelled {
		t.Fatalf("CANCELLED filter failed: %+v", page.Data)
	}

	rec = hu008GET(t, mux, "/reservations?requester_id=aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected empty 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil {
		t.Fatalf("decode empty list: %v", err)
	}
	if len(page.Data) != 0 || page.Pagination.TotalItems != 0 || page.Pagination.TotalPages != 0 {
		t.Fatalf("expected coherent empty page, got %+v", page)
	}
}

func TestReservationHandler_HU008_DetailAndSecurity(t *testing.T) {
	mux := setupReservationQueryServer(t)
	token := createTestTokenWithSubject([]string{"ENV_VIEW"}, hu008ReqA, testJWTSecret, false)

	rec := hu008GET(t, mux, "/reservations/"+hu008Res1, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected detail 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var detail dto.ReservationResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &detail); err != nil || detail.ID != hu008Res1 {
		t.Fatalf("unexpected detail %+v err=%v", detail, err)
	}

	rec = hu008GET(t, mux, "/reservations/aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", token)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d body=%s", rec.Code, rec.Body.String())
	}
	if rec := hu008GET(t, mux, "/reservations", ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
	wrong := createTestTokenWithSubject([]string{"ENV_RESERVATION_MANAGE"}, hu008ReqA, testJWTSecret, false)
	if rec := hu008GET(t, mux, "/reservations", wrong); rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestReservationHandler_HU008_TrainingCenterScope(t *testing.T) {
	mux := setupReservationQueryServer(t)
	token := createScopedTestTokenWithSubject([]string{"ENV_VIEW:TRAINING_CENTER"}, hu008ReqA, hu008CenterA, testJWTSecret, false)

	rec := hu008GET(t, mux, "/reservations?page_size=100", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected scoped list 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var page dto.ReservationListResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil {
		t.Fatalf("decode scoped page: %v", err)
	}
	if page.Pagination.TotalItems != 3 {
		t.Fatalf("expected only center A reservations, got %+v", page)
	}
	for _, item := range page.Data {
		if item.ID == hu008Res4 {
			t.Fatalf("reservation from another training center leaked through scope")
		}
	}
	if rec := hu008GET(t, mux, "/reservations/"+hu008Res4, token); rec.Code != http.StatusNotFound {
		t.Fatalf("out-of-scope detail must be 404, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestReservationHandler_HU008_InvalidQueryIsBadRequest(t *testing.T) {
	mux := setupReservationQueryServer(t)
	token := createTestTokenWithSubject([]string{"ENV_VIEW"}, hu008ReqA, testJWTSecret, false)
	cases := []string{
		"/reservations?page=0",
		"/reservations?page_size=101",
		"/reservations?status=cancelled",
		"/reservations?environment_id=bad",
		"/reservations?from=2026-09-22&to=2026-09-22",
		"/reservations?sort=not_allowed:asc",
	}
	for _, path := range cases {
		rec := hu008GET(t, mux, path, token)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("%s expected 400, got %d body=%s", path, rec.Code, rec.Body.String())
		}
	}
}
