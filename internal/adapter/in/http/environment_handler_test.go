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
)

func setupEnvironmentTestServer() *http.ServeMux {
	envRepo := persistence.NewMemoryEnvironmentRepository()
	envTypeRepo := persistence.NewMemoryEnvironmentTypeRepository()
	envUseCase := usecase.NewEnvironmentUseCase(envRepo, envTypeRepo)
	auth := middleware.NewAuthMiddleware(testJWTSecret)

	mux := http.NewServeMux()
	handler := httpAdapter.NewEnvironmentHandler(envUseCase)
	handler.RegisterRoutes(mux, auth)
	return mux
}

// E1: POST /training-environments with ENV_MANAGE returns 201 with full Environment (is_active=true, timestamps)
func TestEnvironmentHandler_E1_CreateEnvironmentValid(t *testing.T) {
	mux := setupEnvironmentTestServer()
	token := createTestToken([]string{"ENV_MANAGE"}, testJWTSecret, false)

	loc := "Piso 2, Bloque C"
	reqBody, _ := json.Marshal(dto.CreateEnvironmentRequest{
		EnvironmentTypeID: "347d47f9-1ba5-4b9c-89b7-892112a23d90", // Aula seed
		TrainingCenterID:  "c1d2e3f4-a5b6-7890-1234-56789abcdef0",
		Name:              "Aula 204 - Arquitectura de Software",
		Capacity:          40,
		Location:          &loc,
	})

	req := httptest.NewRequest(http.MethodPost, "/training-environments", bytes.NewReader(reqBody))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created, got: %d (%s)", rec.Code, rec.Body.String())
	}

	var created dto.EnvironmentResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if created.ID == "" || !created.IsActive || created.CreatedAt == "" || created.UpdatedAt == "" {
		t.Fatalf("missing required full environment fields: %+v", created)
	}
	if created.Name != "Aula 204 - Arquitectura de Software" || created.Capacity != 40 {
		t.Fatalf("mismatched created values: %+v", created)
	}
}

// E2: Validation of payload - capacity <= 0, name empty or > 100 runes, location > 200 runes -> 422
func TestEnvironmentHandler_E2_PayloadValidation(t *testing.T) {
	mux := setupEnvironmentTestServer()
	token := createTestToken([]string{"ENV_MANAGE"}, testJWTSecret, false)

	// 1. Capacity <= 0
	zeroCapBody, _ := json.Marshal(dto.CreateEnvironmentRequest{
		EnvironmentTypeID: "347d47f9-1ba5-4b9c-89b7-892112a23d90",
		TrainingCenterID:  "c1d2e3f4-a5b6-7890-1234-56789abcdef0",
		Name:              "Aula Capacidad Cero",
		Capacity:          0,
	})
	reqZero := httptest.NewRequest(http.MethodPost, "/training-environments", bytes.NewReader(zeroCapBody))
	reqZero.Header.Set("Authorization", "Bearer "+token)
	reqZero.Header.Set("Content-Type", "application/json")
	recZero := httptest.NewRecorder()
	mux.ServeHTTP(recZero, reqZero)

	if recZero.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 for capacity=0, got: %d (%s)", recZero.Code, recZero.Body.String())
	}

	// 2. Empty name
	emptyNameBody, _ := json.Marshal(dto.CreateEnvironmentRequest{
		EnvironmentTypeID: "347d47f9-1ba5-4b9c-89b7-892112a23d90",
		TrainingCenterID:  "c1d2e3f4-a5b6-7890-1234-56789abcdef0",
		Name:              "   ",
		Capacity:          20,
	})
	reqEmptyName := httptest.NewRequest(http.MethodPost, "/training-environments", bytes.NewReader(emptyNameBody))
	reqEmptyName.Header.Set("Authorization", "Bearer "+token)
	reqEmptyName.Header.Set("Content-Type", "application/json")
	recEmptyName := httptest.NewRecorder()
	mux.ServeHTTP(recEmptyName, reqEmptyName)

	if recEmptyName.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 for empty name, got: %d (%s)", recEmptyName.Code, recEmptyName.Body.String())
	}

	// 3. Name > 100 runes
	longNameBody, _ := json.Marshal(dto.CreateEnvironmentRequest{
		EnvironmentTypeID: "347d47f9-1ba5-4b9c-89b7-892112a23d90",
		TrainingCenterID:  "c1d2e3f4-a5b6-7890-1234-56789abcdef0",
		Name:              strings.Repeat("ñ", 101),
		Capacity:          20,
	})
	reqLongName := httptest.NewRequest(http.MethodPost, "/training-environments", bytes.NewReader(longNameBody))
	reqLongName.Header.Set("Authorization", "Bearer "+token)
	reqLongName.Header.Set("Content-Type", "application/json")
	recLongName := httptest.NewRecorder()
	mux.ServeHTTP(recLongName, reqLongName)

	if recLongName.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 for name > 100 runes, got: %d (%s)", recLongName.Code, recLongName.Body.String())
	}

	// 4. Location > 200 runes -> 422
	tooLongLoc := strings.Repeat("L", 201)
	longLocBody, _ := json.Marshal(dto.CreateEnvironmentRequest{
		EnvironmentTypeID: "347d47f9-1ba5-4b9c-89b7-892112a23d90",
		TrainingCenterID:  "c1d2e3f4-a5b6-7890-1234-56789abcdef0",
		Name:              "Ambiente con ubicación larga",
		Capacity:          20,
		Location:          &tooLongLoc,
	})
	reqLongLoc := httptest.NewRequest(http.MethodPost, "/training-environments", bytes.NewReader(longLocBody))
	reqLongLoc.Header.Set("Authorization", "Bearer "+token)
	reqLongLoc.Header.Set("Content-Type", "application/json")
	recLongLoc := httptest.NewRecorder()
	mux.ServeHTTP(recLongLoc, reqLongLoc)

	if recLongLoc.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 for location > 200 runes, got: %d (%s)", recLongLoc.Code, recLongLoc.Body.String())
	}

	// 4b. Contract boundary values 150, 151, 199 and 200 runes -> 201 Created.
	for _, length := range []int{150, 151, 199, 200} {
		location := strings.Repeat("L", length)
		boundaryLocBody, _ := json.Marshal(dto.CreateEnvironmentRequest{
			EnvironmentTypeID: "347d47f9-1ba5-4b9c-89b7-892112a23d90",
			TrainingCenterID:  "c1d2e3f4-a5b6-7890-1234-56789abcdef0",
			Name:              "Ambiente con ubicación límite",
			Capacity:          20,
			Location:          &location,
		})
		reqBoundaryLoc := httptest.NewRequest(http.MethodPost, "/training-environments", bytes.NewReader(boundaryLocBody))
		reqBoundaryLoc.Header.Set("Authorization", "Bearer "+token)
		reqBoundaryLoc.Header.Set("Content-Type", "application/json")
		recBoundaryLoc := httptest.NewRecorder()
		mux.ServeHTTP(recBoundaryLoc, reqBoundaryLoc)

		if recBoundaryLoc.Code != http.StatusCreated {
			t.Fatalf("expected 201 Created for location == %d runes, got: %d (%s)", length, recBoundaryLoc.Code, recBoundaryLoc.Body.String())
		}
	}

	// 5. Valid 100 UTF-8 rune name with accents (201 Created)
	name100 := strings.Repeat("á", 100)
	multibyteBody, _ := json.Marshal(dto.CreateEnvironmentRequest{
		EnvironmentTypeID: "347d47f9-1ba5-4b9c-89b7-892112a23d90",
		TrainingCenterID:  "c1d2e3f4-a5b6-7890-1234-56789abcdef0",
		Name:              name100,
		Capacity:          20,
	})
	reqMulti := httptest.NewRequest(http.MethodPost, "/training-environments", bytes.NewReader(multibyteBody))
	reqMulti.Header.Set("Authorization", "Bearer "+token)
	reqMulti.Header.Set("Content-Type", "application/json")
	recMulti := httptest.NewRecorder()
	mux.ServeHTTP(recMulti, reqMulti)

	if recMulti.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created for 100 rune name, got: %d (%s)", recMulti.Code, recMulti.Body.String())
	}

	// 6. Malformed UUID (36 non-hex chars like 'x's) -> 422 Unprocessable Entity
	malformedUUIDBody, _ := json.Marshal(dto.CreateEnvironmentRequest{
		EnvironmentTypeID: "xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx",
		TrainingCenterID:  "yyyyyyyy-yyyy-yyyy-yyyy-yyyyyyyyyyyy",
		Name:              "Aula UUID Malformado",
		Capacity:          20,
	})
	reqMalformedUUID := httptest.NewRequest(http.MethodPost, "/training-environments", bytes.NewReader(malformedUUIDBody))
	reqMalformedUUID.Header.Set("Authorization", "Bearer "+token)
	reqMalformedUUID.Header.Set("Content-Type", "application/json")
	recMalformedUUID := httptest.NewRecorder()
	mux.ServeHTTP(recMalformedUUID, reqMalformedUUID)

	if recMalformedUUID.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 for malformed UUID (36 'x's), got: %d (%s)", recMalformedUUID.Code, recMalformedUUID.Body.String())
	}
}

// E3: Invalid / non-existing environment_type_id returns 422 Unprocessable Entity
func TestEnvironmentHandler_E3_InvalidEnvironmentTypeReference(t *testing.T) {
	mux := setupEnvironmentTestServer()
	token := createTestToken([]string{"ENV_MANAGE"}, testJWTSecret, false)

	reqBody, _ := json.Marshal(dto.CreateEnvironmentRequest{
		EnvironmentTypeID: "ffffffff-ffff-ffff-ffff-ffffffffffff", // Non-existent FK
		TrainingCenterID:  "c1d2e3f4-a5b6-7890-1234-56789abcdef0",
		Name:              "Aula con Tipo Inexistente",
		Capacity:          25,
	})

	req := httptest.NewRequest(http.MethodPost, "/training-environments", bytes.NewReader(reqBody))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 Unprocessable Entity for invalid FK, got: %d (%s)", rec.Code, rec.Body.String())
	}

	var errResp response.ErrorEnvelope
	if err := json.Unmarshal(rec.Body.Bytes(), &errResp); err != nil {
		t.Fatalf("failed to decode error envelope: %v", err)
	}
	if errResp.Error.Code != "UNPROCESSABLE_ENTITY" || errResp.Error.TraceID == "" {
		t.Fatalf("unexpected error envelope structure: %+v", errResp)
	}
}

// E4: GET /training-environments/{id} with ENV_VIEW returns 200 or 404
func TestEnvironmentHandler_E4_GetByID(t *testing.T) {
	mux := setupEnvironmentTestServer()
	token := createTestToken([]string{"ENV_VIEW"}, testJWTSecret, false)

	// Existing seed environment (200 OK)
	req := httptest.NewRequest(http.MethodGet, "/training-environments/78901234-5678-9012-3456-789012345678", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got: %d (%s)", rec.Code, rec.Body.String())
	}

	var item dto.EnvironmentResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &item); err != nil {
		t.Fatalf("failed to parse environment: %v", err)
	}
	if item.ID != "78901234-5678-9012-3456-789012345678" || item.Name != "Aula 101 - Informática" {
		t.Fatalf("unexpected environment data: %+v", item)
	}

	// Non-existing ID (404 Not Found)
	reqNotFound := httptest.NewRequest(http.MethodGet, "/training-environments/00000000-0000-0000-0000-000000000000", nil)
	reqNotFound.Header.Set("Authorization", "Bearer "+token)
	recNotFound := httptest.NewRecorder()
	mux.ServeHTTP(recNotFound, reqNotFound)

	if recNotFound.Code != http.StatusNotFound {
		t.Fatalf("expected 404 Not Found, got: %d (%s)", recNotFound.Code, recNotFound.Body.String())
	}

	var errEnv response.ErrorEnvelope
	_ = json.Unmarshal(recNotFound.Body.Bytes(), &errEnv)
	if errEnv.Error.Code != "NOT_FOUND" {
		t.Fatalf("expected NOT_FOUND code, got: %s", errEnv.Error.Code)
	}
}

func TestEnvironmentHandler_HU004_UpdateReactivateAndLogicalDeactivate(t *testing.T) {
	mux := setupEnvironmentTestServer()
	token := createTestToken([]string{"ENV_MANAGE", "ENV_VIEW"}, testJWTSecret, false)
	environmentID := "78901234-5678-9012-3456-789012345678"

	request := func(method, path string, payload interface{}) *httptest.ResponseRecorder {
		body, _ := json.Marshal(payload)
		req := httptest.NewRequest(method, path, bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		return rec
	}

	updated := request(http.MethodPut, "/training-environments/"+environmentID, map[string]interface{}{
		"environment_type_id": "347d47f9-1ba5-4b9c-89b7-892112a23d90",
		"name":                "Aula 101 actualizada",
		"capacity":            36,
		"location":            "Piso 1, Bloque A",
		"is_active":           false,
	})
	if updated.Code != http.StatusOK {
		t.Fatalf("expected 200 for environment update, got: %d (%s)", updated.Code, updated.Body.String())
	}
	var item dto.EnvironmentResponse
	if err := json.Unmarshal(updated.Body.Bytes(), &item); err != nil {
		t.Fatalf("failed to decode updated environment: %v", err)
	}
	if item.ID != environmentID || item.Name != "Aula 101 actualizada" || item.Capacity != 36 || item.IsActive {
		t.Fatalf("unexpected updated environment: %+v", item)
	}

	reactivated := request(http.MethodPatch, "/training-environments/"+environmentID, map[string]interface{}{"is_active": true})
	if reactivated.Code != http.StatusOK {
		t.Fatalf("expected 200 for environment reactivation, got: %d (%s)", reactivated.Code, reactivated.Body.String())
	}
	if err := json.Unmarshal(reactivated.Body.Bytes(), &item); err != nil {
		t.Fatalf("failed to decode reactivated environment: %v", err)
	}
	if !item.IsActive {
		t.Fatalf("expected environment to be reactivated: %+v", item)
	}

	invalid := request(http.MethodPatch, "/training-environments/"+environmentID, map[string]interface{}{"capacity": 0})
	if invalid.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 for invalid update payload, got: %d (%s)", invalid.Code, invalid.Body.String())
	}

	for _, length := range []int{150, 151, 199, 200} {
		locationUpdate := request(http.MethodPatch, "/training-environments/"+environmentID, map[string]interface{}{"location": strings.Repeat("L", length)})
		if locationUpdate.Code != http.StatusOK {
			t.Fatalf("expected 200 for a %d-character location, got: %d (%s)", length, locationUpdate.Code, locationUpdate.Body.String())
		}
	}

	location201 := request(http.MethodPatch, "/training-environments/"+environmentID, map[string]interface{}{"location": strings.Repeat("L", 201)})
	if location201.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 for a 201-character location, got: %d (%s)", location201.Code, location201.Body.String())
	}

	missing := request(http.MethodPatch, "/training-environments/00000000-0000-0000-0000-000000000000", map[string]interface{}{"name": "No existe"})
	if missing.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for unknown environment, got: %d (%s)", missing.Code, missing.Body.String())
	}

	invalidID := request(http.MethodPatch, "/training-environments/not-a-uuid", map[string]interface{}{"name": "ID inválido"})
	if invalidID.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 for malformed environment id, got: %d (%s)", invalidID.Code, invalidID.Body.String())
	}

	invalidDeleteID := request(http.MethodDelete, "/training-environments/not-a-uuid", nil)
	if invalidDeleteID.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 for malformed delete id, got: %d (%s)", invalidDeleteID.Code, invalidDeleteID.Body.String())
	}

	deactivated := request(http.MethodDelete, "/training-environments/"+environmentID, nil)
	if deactivated.Code != http.StatusNoContent {
		t.Fatalf("expected 204 for logical deactivation, got: %d (%s)", deactivated.Code, deactivated.Body.String())
	}

	repeated := request(http.MethodDelete, "/training-environments/"+environmentID, nil)
	if repeated.Code != http.StatusNoContent {
		t.Fatalf("expected idempotent 204 for repeated deactivation, got: %d (%s)", repeated.Code, repeated.Body.String())
	}

	stillPresent := request(http.MethodGet, "/training-environments/"+environmentID, nil)
	if stillPresent.Code != http.StatusOK {
		t.Fatalf("expected logically deleted environment to remain queryable, got: %d (%s)", stillPresent.Code, stillPresent.Body.String())
	}
	if err := json.Unmarshal(stillPresent.Body.Bytes(), &item); err != nil {
		t.Fatalf("failed to decode logically deleted environment: %v", err)
	}
	if item.IsActive {
		t.Fatalf("expected persisted environment to be inactive: %+v", item)
	}
}

// Security / Auth: 401 UNAUTHENTICATED without token, 403 FORBIDDEN without feature
func TestEnvironmentHandler_Security(t *testing.T) {
	mux := setupEnvironmentTestServer()

	// 1. Missing JWT on POST (401 UNAUTHENTICATED)
	reqNoAuth := httptest.NewRequest(http.MethodPost, "/training-environments", bytes.NewReader([]byte("{}")))
	recNoAuth := httptest.NewRecorder()
	mux.ServeHTTP(recNoAuth, reqNoAuth)

	if recNoAuth.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized, got: %d (%s)", recNoAuth.Code, recNoAuth.Body.String())
	}

	// 2. Token with ENV_VIEW trying to POST (Requires ENV_MANAGE) -> 403 FORBIDDEN
	viewToken := createTestToken([]string{"ENV_VIEW"}, testJWTSecret, false)
	reqWrongPerm := httptest.NewRequest(http.MethodPost, "/training-environments", bytes.NewReader([]byte("{}")))
	reqWrongPerm.Header.Set("Authorization", "Bearer "+viewToken)
	recWrongPerm := httptest.NewRecorder()
	mux.ServeHTTP(recWrongPerm, reqWrongPerm)

	if recWrongPerm.Code != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden, got: %d (%s)", recWrongPerm.Code, recWrongPerm.Body.String())
	}
}

func TestEnvironmentHandler_HU004_Security(t *testing.T) {
	methods := []string{http.MethodPut, http.MethodPatch, http.MethodDelete}
	path := "/training-environments/78901234-5678-9012-3456-789012345678"

	for _, method := range methods {
		t.Run(method+"_without_authentication", func(t *testing.T) {
			mux := setupEnvironmentTestServer()
			body := bytes.NewReader([]byte("{\"name\":\"Aula protegida\"}"))
			req := httptest.NewRequest(method, path, body)
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)

			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("expected 401 for %s without authentication, got: %d (%s)", method, rec.Code, rec.Body.String())
			}
		})

		t.Run(method+"_without_manage_feature", func(t *testing.T) {
			mux := setupEnvironmentTestServer()
			viewToken := createTestToken([]string{"ENV_VIEW"}, testJWTSecret, false)
			body := bytes.NewReader([]byte("{\"name\":\"Aula protegida\"}"))
			req := httptest.NewRequest(method, path, body)
			req.Header.Set("Authorization", "Bearer "+viewToken)
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)

			if rec.Code != http.StatusForbidden {
				t.Fatalf("expected 403 for %s without ENV_MANAGE, got: %d (%s)", method, rec.Code, rec.Body.String())
			}
		})
	}
}

func TestEnvironmentHandler_HU003_ListPaginationAndFilters(t *testing.T) {
	mux := setupEnvironmentTestServer()
	token := createTestToken([]string{"ENV_VIEW"}, testJWTSecret, false)

	request := func(path string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		return rec
	}

	rec := request("/training-environments?page=1&page_size=1&sort=name:asc")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for paginated list, got: %d (%s)", rec.Code, rec.Body.String())
	}
	var page dto.EnvironmentListResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil {
		t.Fatalf("failed to decode paginated response: %v", err)
	}
	if len(page.Data) != 1 || page.Pagination.Page != 1 || page.Pagination.PageSize != 1 || page.Pagination.TotalItems != 2 || page.Pagination.TotalPages != 2 {
		t.Fatalf("unexpected pagination response: %+v", page)
	}
	if page.Data[0].Name != "Aula 101 - Informática" {
		t.Fatalf("expected ascending name order, got: %+v", page.Data[0])
	}

	rec = request("/training-environments?environment_type_id=347d47f9-1ba5-4b9c-89b7-892112a23d90&training_center_id=c1d2e3f4-a5b6-7890-1234-56789abcdef0&is_active=true")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for combined filters, got: %d (%s)", rec.Code, rec.Body.String())
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil {
		t.Fatalf("failed to decode filtered response: %v", err)
	}
	if len(page.Data) != 1 || page.Data[0].EnvironmentTypeID != "347d47f9-1ba5-4b9c-89b7-892112a23d90" {
		t.Fatalf("unexpected AND filter response: %+v", page.Data)
	}

	for _, invalid := range []string{"/training-environments?page_size=0", "/training-environments?page_size=101"} {
		rec = request(invalid)
		if rec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("expected 422 for %s, got: %d (%s)", invalid, rec.Code, rec.Body.String())
		}
	}
}

func TestEnvironmentHandler_HU003_AvailabilityWindow(t *testing.T) {
	mux := setupEnvironmentTestServer()
	token := createTestToken([]string{"ENV_VIEW"}, testJWTSecret, false)
	request := func(path string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		return rec
	}

	// The second seed is under maintenance on this date; the first remains available.
	rec := request("/training-environments?available_date=2026-09-15&available_start_time=09:00&available_end_time=10:00")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for availability query, got: %d (%s)", rec.Code, rec.Body.String())
	}
	var page dto.EnvironmentListResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil {
		t.Fatalf("failed to decode availability response: %v", err)
	}
	if len(page.Data) != 1 || page.Data[0].ID != "78901234-5678-9012-3456-789012345678" {
		t.Fatalf("unexpected availability response: %+v", page.Data)
	}

	// A partial availability window is rejected instead of being interpreted ambiguously.
	rec = request("/training-environments?available_date=2026-09-15&available_start_time=09:00")
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 for incomplete availability window, got: %d (%s)", rec.Code, rec.Body.String())
	}

	// The first seed has a pending reservation on this date and time.
	rec = request("/training-environments?available_date=2026-09-16&available_start_time=10:30&available_end_time=11:00")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for reservation availability query, got: %d (%s)", rec.Code, rec.Body.String())
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil {
		t.Fatalf("failed to decode reservation availability response: %v", err)
	}
	if len(page.Data) != 1 || page.Data[0].ID != "89012345-6789-0123-4567-890123456789" {
		t.Fatalf("expected only the second environment after reservation filtering, got: %+v", page.Data)
	}
}
