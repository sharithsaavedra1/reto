package http_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/code-sena/design-software-training-environment-service/internal/adapter/in/http/dto"
	"github.com/code-sena/design-software-training-environment-service/internal/domain/model"
)

func TestReservationHandler_HU019_FiltersPaginationAndOrder(t *testing.T) {
	mux := setupReservationQueryServer(t)
	token := createTestTokenWithSubject([]string{"ENV_VIEW"}, hu008ReqA, testJWTSecret, false)

	t.Run("environment and reservation date use existing behavior", func(t *testing.T) {
		rec := hu008GET(t, mux, "/reservations?environment_id="+hu008EnvA+"&from=2026-09-20&to=2026-09-23&page=1&page_size=2&sort=reservation_date:asc", token)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
		}
		var page dto.ReservationListResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil {
			t.Fatalf("decode environment/date page: %v", err)
		}
		if page.Pagination.TotalItems != 2 || page.Pagination.TotalPages != 1 || len(page.Data) != 2 {
			t.Fatalf("unexpected environment/date pagination: %+v", page)
		}
		if page.Data[0].ID != hu008Res1 || page.Data[1].ID != hu008Res2 {
			t.Fatalf("reservation_date asc order changed: %+v", page.Data)
		}
	})

	t.Run("requester and reservation date keep pagination and order", func(t *testing.T) {
		rec := hu008GET(t, mux, "/reservations?requester_id="+hu008ReqA+"&from=2026-09-20&to=2026-09-23&page=2&page_size=1&sort=reservation_date:desc", token)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
		}
		var page dto.ReservationListResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil {
			t.Fatalf("decode requester/date page: %v", err)
		}
		if page.Pagination.TotalItems != 2 || page.Pagination.TotalPages != 2 || page.Pagination.Page != 2 || len(page.Data) != 1 {
			t.Fatalf("unexpected requester/date pagination: %+v", page)
		}
		if page.Data[0].ID != hu008Res1 {
			t.Fatalf("expected second descending requester/date item %s, got %+v", hu008Res1, page.Data)
		}
	})

	t.Run("status and reservation date include cancelled reservations", func(t *testing.T) {
		rec := hu008GET(t, mux, "/reservations?status=CANCELLED&from=2026-09-20&to=2026-09-24&sort=reservation_date:asc", token)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
		}
		var page dto.ReservationListResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil {
			t.Fatalf("decode status/date page: %v", err)
		}
		if page.Pagination.TotalItems != 1 || len(page.Data) != 1 || page.Data[0].ID != hu008Res2 || page.Data[0].Status != model.ReservationStatusCancelled {
			t.Fatalf("CANCELLED status/date filter changed behavior: %+v", page)
		}
	})
}
