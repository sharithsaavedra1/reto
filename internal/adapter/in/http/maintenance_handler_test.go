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
	"github.com/code-sena/design-software-training-environment-service/internal/adapter/out/persistence"
	"github.com/code-sena/design-software-training-environment-service/internal/application/usecase"
)

func setupMaintenanceTestServer() (*http.ServeMux, *persistence.MemoryMaintenanceRepository) {
	envRepo := persistence.NewMemoryEnvironmentRepository()
	maintenanceRepo := persistence.NewMemoryMaintenanceRepository(envRepo)
	maintenanceUseCase := usecase.NewMaintenanceUseCase(maintenanceRepo)
	auth := middleware.NewAuthMiddleware(testJWTSecret)
	mux := http.NewServeMux()
	httpAdapter.NewMaintenanceHandler(maintenanceUseCase).RegisterRoutes(mux, auth)
	return mux, maintenanceRepo
}

func TestMaintenanceHandler_HU006_ListCreateDetailAndFilters(t *testing.T) {
	mux, maintenanceRepo := setupMaintenanceTestServer()
	environmentID := "78901234-5678-9012-3456-789012345678"
	createdBy := "42c78701-2b34-4a6f-b034-3f7a3eca57d8"
	manageToken := createTestTokenWithSubject([]string{"ENV_MAINTENANCE_MANAGE"}, createdBy, testJWTSecret, false)
	viewToken := createTestTokenWithSubject([]string{"ENV_VIEW"}, createdBy, testJWTSecret, false)

	request := func(method, path, token string, payload interface{}) *httptest.ResponseRecorder {
		var body []byte
		if payload != nil {
			body, _ = json.Marshal(payload)
		}
		req := httptest.NewRequest(method, path, bytes.NewReader(body))
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		return rec
	}

	initial := request(http.MethodGet, "/training-environments/"+environmentID+"/maintenance", viewToken, nil)
	if initial.Code != http.StatusOK {
		t.Fatalf("expected 200 for an empty maintenance list, got %d (%s)", initial.Code, initial.Body.String())
	}
	first := request(http.MethodPost, "/training-environments/"+environmentID+"/maintenance", manageToken, dto.MaintenanceCreateRequest{
		StartDate: "2026-06-01", EndDate: "2026-06-10", StartTime: "00:00", EndTime: "00:00", Reason: stringPointer("Electrical inspection"),
	})
	if first.Code != http.StatusCreated {
		t.Fatalf("expected 201 for valid maintenance, got %d (%s)", first.Code, first.Body.String())
	}
	var created dto.MaintenanceResponse
	if err := json.Unmarshal(first.Body.Bytes(), &created); err != nil {
		t.Fatalf("failed to decode created maintenance: %v", err)
	}
	if created.EnvironmentID != environmentID || created.StartDate != "2026-06-01" || created.EndDate != "2026-06-10" || created.StartTime != "00:00:00" || created.EndTime != "00:00:00" || created.CreatedBy != createdBy || created.Reason == nil || *created.Reason != "Electrical inspection" {
		t.Fatalf("unexpected created maintenance response: %+v", created)
	}

	second := request(http.MethodPost, "/training-environments/"+environmentID+"/maintenance", manageToken, dto.MaintenanceCreateRequest{
		StartDate: "2026-06-20", EndDate: "2026-06-30", StartTime: "22:00", EndTime: "06:00",
	})
	if second.Code != http.StatusCreated {
		t.Fatalf("expected 201 when reason is omitted, got %d (%s)", second.Code, second.Body.String())
	}

	list := func(query string) []dto.MaintenanceResponse {
		rec := request(http.MethodGet, "/training-environments/"+environmentID+"/maintenance"+query, viewToken, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 for maintenance query %q, got %d (%s)", query, rec.Code, rec.Body.String())
		}
		var items []dto.MaintenanceResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &items); err != nil {
			t.Fatalf("failed to decode maintenance list for %q: %v", query, err)
		}
		return items
	}
	if items := list(""); len(items) != 2 {
		t.Fatalf("expected two maintenance periods without filters, got %d", len(items))
	}
	if items := list("?from=2026-06-05"); len(items) != 2 {
		t.Fatalf("expected two periods overlapping from=2026-06-05, got %d", len(items))
	}
	if items := list("?to=2026-06-15"); len(items) != 1 || items[0].ID != created.ID {
		t.Fatalf("expected only first period before to=2026-06-15, got %+v", items)
	}
	if items := list("?from=2026-06-05&to=2026-06-15"); len(items) != 1 || items[0].ID != created.ID {
		t.Fatalf("expected first period for overlapping range, got %+v", items)
	}
	if items := list("?from=2026-06-10"); len(items) != 1 {
		t.Fatalf("expected boundary from=2026-06-10 to exclude the first period, got %d", len(items))
	}
	if items := list("?to=2026-06-01"); len(items) != 0 {
		t.Fatalf("expected boundary to=2026-06-01 to exclude the first period, got %d", len(items))
	}
	if items := list("?from=2026-07-01&to=2026-07-10"); len(items) != 0 {
		t.Fatalf("expected no periods for a non-overlapping range, got %d", len(items))
	}

	detail := request(http.MethodGet, "/training-environments/"+environmentID+"/maintenance/"+created.ID, viewToken, nil)
	if detail.Code != http.StatusOK {
		t.Fatalf("expected 200 for maintenance detail, got %d (%s)", detail.Code, detail.Body.String())
	}
	missing := request(http.MethodGet, "/training-environments/"+environmentID+"/maintenance/00000000-0000-0000-0000-000000000001", viewToken, nil)
	if missing.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for unknown maintenance, got %d", missing.Code)
	}
	wrongEnvironment := request(http.MethodGet, "/training-environments/89012345-6789-0123-4567-890123456789/maintenance/"+created.ID, viewToken, nil)
	if wrongEnvironment.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for maintenance owned by another environment, got %d", wrongEnvironment.Code)
	}
	unknownEnvironment := request(http.MethodGet, "/training-environments/00000000-0000-0000-0000-000000000000/maintenance", viewToken, nil)
	if unknownEnvironment.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for unknown environment, got %d", unknownEnvironment.Code)
	}
	if events := maintenanceRepo.OutboxEvents(); len(events) != 2 || events[0].EventType != "environment.maintenance.started" {
		t.Fatalf("expected one event per successful creation, got %+v", events)
	}
}

func TestMaintenanceHandler_HU006_ValidationSecurityAndAppendOnly(t *testing.T) {
	mux, maintenanceRepo := setupMaintenanceTestServer()
	environmentID := "78901234-5678-9012-3456-789012345678"
	createdBy := "42c78701-2b34-4a6f-b034-3f7a3eca57d8"
	manageToken := createTestTokenWithSubject([]string{"ENV_MAINTENANCE_MANAGE"}, createdBy, testJWTSecret, false)
	viewToken := createTestTokenWithSubject([]string{"ENV_VIEW"}, createdBy, testJWTSecret, false)
	request := func(method, path, token string, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		return rec
	}

	invalidPayloads := []string{
		`{"start_date":"2026-06-10","end_date":"2026-06-01","start_time":"08:00","end_time":"09:00"}`,
		`{"start_date":"2026-06-10","end_date":"2026-06-10","start_time":"10:00","end_time":"10:00"}`,
		`{"start_date":"not-a-date","end_date":"2026-06-10","start_time":"08:00","end_time":"09:00"}`,
		`{"start_date":"2026-06-01","end_date":"2026-06-10","start_time":"25:00","end_time":"09:00"}`,
		`{"start_date":"2026-06-01","end_date":"2026-06-10","start_time":"08:00","end_time":"09:00","reason":"` + strings.Repeat("r", 256) + `"}`,
	}
	for index, payload := range invalidPayloads[:4] {
		if rec := request(http.MethodPost, "/training-environments/"+environmentID+"/maintenance", manageToken, payload); rec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("invalid payload %d: expected 422, got %d (%s)", index, rec.Code, rec.Body.String())
		}
	}
	valid := `{"start_date":"2026-06-01","end_date":"2026-06-10","start_time":"22:00","end_time":"06:00"}`
	if rec := request(http.MethodPost, "/training-environments/"+environmentID+"/maintenance", manageToken, valid); rec.Code != http.StatusCreated {
		t.Fatalf("expected valid payload to return 201, got %d (%s)", rec.Code, rec.Body.String())
	}
	beforeInvalidReason := len(maintenanceRepo.OutboxEvents())
	if rec := request(http.MethodPost, "/training-environments/"+environmentID+"/maintenance", manageToken, invalidPayloads[4]); rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("reason over 255 characters: expected 422, got %d (%s)", rec.Code, rec.Body.String())
	}
	if after := len(maintenanceRepo.OutboxEvents()); after != beforeInvalidReason {
		t.Fatalf("rejected maintenance must not create an event: before=%d after=%d", beforeInvalidReason, after)
	}

	if rec := request(http.MethodGet, "/training-environments/"+environmentID+"/maintenance?from=bad-date", viewToken, ""); rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("invalid from filter: expected 422, got %d", rec.Code)
	}
	if rec := request(http.MethodGet, "/training-environments/"+environmentID+"/maintenance?from=2026-07-01&to=2026-06-01", viewToken, ""); rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("from after to: expected 422, got %d", rec.Code)
	}
	if rec := request(http.MethodGet, "/training-environments/"+environmentID+"/maintenance", "", ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated GET: expected 401, got %d", rec.Code)
	}
	if rec := request(http.MethodGet, "/training-environments/"+environmentID+"/maintenance", manageToken, ""); rec.Code != http.StatusForbidden {
		t.Fatalf("GET without ENV_VIEW: expected 403, got %d", rec.Code)
	}
	if rec := request(http.MethodPost, "/training-environments/"+environmentID+"/maintenance", viewToken, valid); rec.Code != http.StatusForbidden {
		t.Fatalf("POST without ENV_MAINTENANCE_MANAGE: expected 403, got %d", rec.Code)
	}
	if rec := request(http.MethodPost, "/training-environments/"+environmentID+"/maintenance", "", valid); rec.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated POST: expected 401, got %d", rec.Code)
	}
	if rec := request(http.MethodPut, "/training-environments/"+environmentID+"/maintenance/00000000-0000-0000-0000-000000000001", manageToken, valid); rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("append-only maintenance must not support PUT, got %d", rec.Code)
	}
}

func TestMaintenanceHandler_HU018_TemporalValidationReturns422(t *testing.T) {
	mux, maintenanceRepo := setupMaintenanceTestServer()
	environmentID := "78901234-5678-9012-3456-789012345678"
	createdBy := "42c78701-2b34-4a6f-b034-3f7a3eca57d8"
	manageToken := createTestTokenWithSubject([]string{"ENV_MAINTENANCE_MANAGE"}, createdBy, testJWTSecret, false)

	post := func(payload string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/training-environments/"+environmentID+"/maintenance", bytes.NewBufferString(payload))
		req.Header.Set("Authorization", "Bearer "+manageToken)
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		return rec
	}

	invalid := []string{
		`{"start_date":"2026-10-06","end_date":"2026-10-05","start_time":"08:00","end_time":"09:00"}`,
		`{"start_date":"2026-10-05","end_date":"2026-10-05","start_time":"10:00","end_time":"10:00"}`,
		`{"start_date":"2026-10-05","end_date":"2026-10-05","start_time":"10:00","end_time":"09:00"}`,
	}
	for index, payload := range invalid {
		rec := post(payload)
		if rec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("invalid temporal payload %d: expected 422, got %d (%s)", index, rec.Code, rec.Body.String())
		}
	}
	if events := maintenanceRepo.OutboxEvents(); len(events) != 0 {
		t.Fatalf("invalid temporal data must not create events: %+v", events)
	}

	sameDay := post(`{"start_date":"2026-10-05","end_date":"2026-10-05","start_time":"08:00","end_time":"10:30"}`)
	if sameDay.Code != http.StatusCreated {
		t.Fatalf("valid same-day maintenance: expected 201, got %d (%s)", sameDay.Code, sameDay.Body.String())
	}
	var sameDayResponse dto.MaintenanceResponse
	if err := json.Unmarshal(sameDay.Body.Bytes(), &sameDayResponse); err != nil {
		t.Fatalf("decode same-day response: %v", err)
	}
	if sameDayResponse.StartTime != "08:00:00" || sameDayResponse.EndTime != "10:30:00" {
		t.Fatalf("same-day response did not preserve normalized times: %+v", sameDayResponse)
	}

	multiDay := post(`{"start_date":"2026-10-05","end_date":"2026-10-06","start_time":"22:00","end_time":"06:00"}`)
	if multiDay.Code != http.StatusCreated {
		t.Fatalf("multi-day maintenance with earlier end clock: expected 201, got %d (%s)", multiDay.Code, multiDay.Body.String())
	}
	if events := maintenanceRepo.OutboxEvents(); len(events) != 2 {
		t.Fatalf("expected two events for the valid maintenances, got %+v", events)
	}
}
