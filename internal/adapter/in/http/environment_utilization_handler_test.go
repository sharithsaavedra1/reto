package http_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	httpAdapter "github.com/code-sena/design-software-training-environment-service/internal/adapter/in/http"
	"github.com/code-sena/design-software-training-environment-service/internal/adapter/in/http/dto"
	"github.com/code-sena/design-software-training-environment-service/internal/adapter/in/http/middleware"
	"github.com/code-sena/design-software-training-environment-service/internal/adapter/in/http/response"
	"github.com/code-sena/design-software-training-environment-service/internal/adapter/out/persistence"
	"github.com/code-sena/design-software-training-environment-service/internal/application/usecase"
)

func setupUtilizationTestServer() *http.ServeMux {
	envRepo := persistence.NewMemoryEnvironmentRepository()
	typeRepo := persistence.NewMemoryEnvironmentTypeRepository()
	ruleRepo := persistence.NewMemoryAvailabilityRuleRepository(envRepo)
	maintenanceRepo := persistence.NewMemoryMaintenanceRepository(envRepo)
	reservationRepo := persistence.NewMemoryReservationRepository(envRepo)
	reportRepo := persistence.NewMemoryEnvironmentUtilizationRepository(envRepo, typeRepo, ruleRepo, maintenanceRepo, reservationRepo)
	auth := middleware.NewAuthMiddleware(testJWTSecret)
	mux := http.NewServeMux()
	httpAdapter.NewEnvironmentUtilizationHandler(usecase.NewEnvironmentUtilizationUseCase(reportRepo)).RegisterRoutes(mux, auth)
	return mux
}

func TestEnvironmentUtilizationHandler_RequiresRangeAndFeature(t *testing.T) {
	mux := setupUtilizationTestServer()
	missingRange := httptest.NewRequest(http.MethodGet, "/reports/environment-utilization", nil)
	missingRange.Header.Set("Authorization", "Bearer "+createTestToken([]string{"ENVIRONMENT_REPORT_VIEW"}, testJWTSecret, false))
	record := httptest.NewRecorder()
	mux.ServeHTTP(record, missingRange)
	if record.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 for missing range, got %d", record.Code)
	}

	unauthorized := httptest.NewRequest(http.MethodGet, "/reports/environment-utilization?from=2026-09-14&to=2026-09-16", nil)
	record = httptest.NewRecorder()
	mux.ServeHTTP(record, unauthorized)
	if record.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without token, got %d", record.Code)
	}

	forbidden := httptest.NewRequest(http.MethodGet, "/reports/environment-utilization?from=2026-09-14&to=2026-09-16", nil)
	forbidden.Header.Set("Authorization", "Bearer "+createTestToken([]string{"ENV_VIEW"}, testJWTSecret, false))
	record = httptest.NewRecorder()
	mux.ServeHTTP(record, forbidden)
	if record.Code != http.StatusForbidden {
		t.Fatalf("expected 403 without report feature, got %d", record.Code)
	}
}

func TestEnvironmentUtilizationHandler_ReturnsPaginatedReport(t *testing.T) {
	mux := setupUtilizationTestServer()
	request := httptest.NewRequest(http.MethodGet, "/reports/environment-utilization?from=2026-09-14&to=2026-09-16&page_size=1", nil)
	request.Header.Set("Authorization", "Bearer "+createTestToken([]string{"ENVIRONMENT_REPORT_VIEW"}, testJWTSecret, false))
	record := httptest.NewRecorder()
	mux.ServeHTTP(record, request)
	if record.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (%s)", record.Code, record.Body.String())
	}
	var body dto.EnvironmentUtilizationPageResponse
	if err := json.Unmarshal(record.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Data) != 1 || body.Pagination.Page != 1 || body.Pagination.PageSize != 1 || body.Pagination.TotalItems != 2 || body.Pagination.TotalPages != 2 {
		t.Fatalf("unexpected paginated response: %+v", body)
	}
}

func TestEnvironmentUtilizationHandler_RejectsInvalidRangeAndFilter(t *testing.T) {
	mux := setupUtilizationTestServer()
	tests := []string{
		"/reports/environment-utilization?from=2026-09-16&to=2026-09-14",
		"/reports/environment-utilization?from=2026-09-14&to=2026-09-16&environment_id=bad-id",
	}
	for _, target := range tests {
		request := httptest.NewRequest(http.MethodGet, target, nil)
		request.Header.Set("Authorization", "Bearer "+createTestToken([]string{"ENVIRONMENT_REPORT_VIEW"}, testJWTSecret, false))
		record := httptest.NewRecorder()
		mux.ServeHTTP(record, request)
		if record.Code != http.StatusUnprocessableEntity {
			t.Fatalf("expected 422 for %s, got %d", target, record.Code)
		}
		var body response.ErrorEnvelope
		if err := json.Unmarshal(record.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if body.Error.Code != "UNPROCESSABLE_ENTITY" {
			t.Fatalf("unexpected error: %+v", body.Error)
		}
	}
}
