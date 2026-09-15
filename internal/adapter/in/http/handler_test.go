package http_test

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	httpAdapter "github.com/code-sena/design-software-training-environment-service/internal/adapter/in/http"
	"github.com/code-sena/design-software-training-environment-service/internal/adapter/in/http/dto"
	"github.com/code-sena/design-software-training-environment-service/internal/adapter/in/http/middleware"
	"github.com/code-sena/design-software-training-environment-service/internal/adapter/in/http/response"
	"github.com/code-sena/design-software-training-environment-service/internal/adapter/out/persistence"
	"github.com/code-sena/design-software-training-environment-service/internal/application/usecase"
)

const testJWTSecret = "test-secret-key-123456"

func createTestToken(features []string, secret string, expired bool) string {
	return createTestTokenWithSubject(features, "user-123", secret, expired)
}

func createTestTokenWithSubject(features []string, subject, secret string, expired bool) string {
	return createCustomTestTokenWithSubject(features, nil, 0, subject, secret, expired)
}

func createCustomTestToken(features []string, roles []string, nbf int64, secret string, expired bool) string {
	return createCustomTestTokenWithSubject(features, roles, nbf, "user-123", secret, expired)
}

func createCustomTestTokenWithSubject(features []string, roles []string, nbf int64, subject, secret string, expired bool) string {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))

	exp := time.Now().Add(1 * time.Hour).Unix()
	if expired {
		exp = time.Now().Add(-1 * time.Hour).Unix()
	}

	payloadMap := map[string]interface{}{
		"sub": subject,
		"exp": exp,
		"iat": time.Now().Unix(),
	}
	if len(features) > 0 {
		payloadMap["features"] = features
	}
	if len(roles) > 0 {
		payloadMap["roles"] = roles
	}
	if nbf > 0 {
		payloadMap["nbf"] = nbf
	}

	payloadBytes, _ := json.Marshal(payloadMap)
	payload := base64.RawURLEncoding.EncodeToString(payloadBytes)

	signingInput := header + "." + payload
	h := hmac.New(sha256.New, []byte(secret))
	h.Write([]byte(signingInput))
	sig := base64.RawURLEncoding.EncodeToString(h.Sum(nil))

	return header + "." + payload + "." + sig
}

func setupTestServer() *http.ServeMux {
	repo := persistence.NewMemoryEnvironmentTypeRepository()
	uc := usecase.NewEnvironmentTypeUseCase(repo)
	auth := middleware.NewAuthMiddleware(testJWTSecret)

	mux := http.NewServeMux()
	handler := httpAdapter.NewEnvironmentTypeHandler(uc)
	handler.RegisterRoutes(mux, auth)
	return mux
}

// E1: GET /environment-types with ENV_VIEW returns 200 array of EnvironmentType
func TestHandler_E1_ListEnvironmentTypes(t *testing.T) {
	mux := setupTestServer()
	token := createTestToken([]string{"ENV_VIEW"}, testJWTSecret, false)

	req := httptest.NewRequest(http.MethodGet, "/environment-types", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-Trace-ID", "test-trace-e1")

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got: %d (%s)", rec.Code, rec.Body.String())
	}

	var items []dto.EnvironmentTypeResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &items); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if len(items) != 3 {
		t.Fatalf("expected 3 seed items, got %d", len(items))
	}

	for _, it := range items {
		if it.ID == "" || it.Code == "" || it.Name == "" || it.CreatedAt == "" {
			t.Fatalf("item missing mandatory contract fields: %+v", it)
		}
	}
}

// E2: POST /environment-types with ENV_MANAGE creates resource or returns 409/422
func TestHandler_E2_CreateEnvironmentType(t *testing.T) {
	mux := setupTestServer()
	token := createTestToken([]string{"ENV_MANAGE"}, testJWTSecret, false)

	// Valid creation (201 Created)
	desc := "Espacio de cómputo avanzado"
	validBody, _ := json.Marshal(dto.CreateEnvironmentTypeRequest{
		Code:        "COMP_LAB",
		Name:        "Sala de Cómputo",
		Description: &desc,
	})

	req := httptest.NewRequest(http.MethodPost, "/environment-types", bytes.NewReader(validBody))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created, got: %d (%s)", rec.Code, rec.Body.String())
	}

	var created dto.EnvironmentTypeResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if created.Code != "COMP_LAB" || created.Name != "Sala de Cómputo" {
		t.Fatalf("unexpected created resource: %+v", created)
	}

	// Valid creation with 100 Multibyte UTF-8 characters
	multibyte100 := strings.Repeat("á", 100)
	multibyteBody, _ := json.Marshal(dto.CreateEnvironmentTypeRequest{
		Code: "MULTI_CODE",
		Name: multibyte100,
	})
	reqMulti := httptest.NewRequest(http.MethodPost, "/environment-types", bytes.NewReader(multibyteBody))
	reqMulti.Header.Set("Authorization", "Bearer "+token)
	reqMulti.Header.Set("Content-Type", "application/json")
	recMulti := httptest.NewRecorder()
	mux.ServeHTTP(recMulti, reqMulti)

	if recMulti.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created for 100 multibyte characters, got: %d (%s)", recMulti.Code, recMulti.Body.String())
	}

	// Duplicate Name conflict (409 Conflict)
	dupNameBody, _ := json.Marshal(dto.CreateEnvironmentTypeRequest{
		Code: "OTHER_CODE",
		Name: "Sala de Cómputo",
	})
	reqDup := httptest.NewRequest(http.MethodPost, "/environment-types", bytes.NewReader(dupNameBody))
	reqDup.Header.Set("Authorization", "Bearer "+token)
	reqDup.Header.Set("Content-Type", "application/json")
	recDup := httptest.NewRecorder()
	mux.ServeHTTP(recDup, reqDup)

	if recDup.Code != http.StatusConflict {
		t.Fatalf("expected 409 Conflict for duplicate name, got: %d (%s)", recDup.Code, recDup.Body.String())
	}

	var errResp response.ErrorEnvelope
	_ = json.Unmarshal(recDup.Body.Bytes(), &errResp)
	if errResp.Error.Code != "CONFLICT" {
		t.Fatalf("expected error code CONFLICT, got: %s", errResp.Error.Code)
	}

	// Invalid payload: empty name (422 Unprocessable Entity)
	invalidBody, _ := json.Marshal(dto.CreateEnvironmentTypeRequest{
		Code: "CODE_ONLY",
		Name: "   ",
	})
	reqInvalid := httptest.NewRequest(http.MethodPost, "/environment-types", bytes.NewReader(invalidBody))
	reqInvalid.Header.Set("Authorization", "Bearer "+token)
	reqInvalid.Header.Set("Content-Type", "application/json")
	recInvalid := httptest.NewRecorder()
	mux.ServeHTTP(recInvalid, reqInvalid)

	if recInvalid.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 Unprocessable Entity, got: %d (%s)", recInvalid.Code, recInvalid.Body.String())
	}

	// Invalid payload: code exceeds 20 chars (422 Unprocessable Entity)
	tooLongCodeBody, _ := json.Marshal(dto.CreateEnvironmentTypeRequest{
		Code: strings.Repeat("X", 21),
		Name: "Valid Name",
	})
	reqTooLong := httptest.NewRequest(http.MethodPost, "/environment-types", bytes.NewReader(tooLongCodeBody))
	reqTooLong.Header.Set("Authorization", "Bearer "+token)
	reqTooLong.Header.Set("Content-Type", "application/json")
	recTooLong := httptest.NewRecorder()
	mux.ServeHTTP(recTooLong, reqTooLong)

	if recTooLong.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 for code > 20, got: %d (%s)", recTooLong.Code, recTooLong.Body.String())
	}
}

// E3: GET /environment-types/{id} with ENV_VIEW returns 200 or 404
func TestHandler_E3_GetByID(t *testing.T) {
	mux := setupTestServer()
	token := createTestToken([]string{"ENV_VIEW"}, testJWTSecret, false)

	// Existing seed (200 OK)
	req := httptest.NewRequest(http.MethodGet, "/environment-types/347d47f9-1ba5-4b9c-89b7-892112a23d90", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got: %d (%s)", rec.Code, rec.Body.String())
	}

	var item dto.EnvironmentTypeResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &item)
	if item.Code != "CLASSROOM" || item.Name != "Aula" {
		t.Fatalf("unexpected item: %+v", item)
	}

	// Inexistent ID (404 Not Found)
	reqNotFound := httptest.NewRequest(http.MethodGet, "/environment-types/00000000-0000-0000-0000-000000000000", nil)
	reqNotFound.Header.Set("Authorization", "Bearer "+token)
	recNotFound := httptest.NewRecorder()
	mux.ServeHTTP(recNotFound, reqNotFound)

	if recNotFound.Code != http.StatusNotFound {
		t.Fatalf("expected 404 Not Found, got: %d (%s)", recNotFound.Code, recNotFound.Body.String())
	}

	var errEnvelope response.ErrorEnvelope
	_ = json.Unmarshal(recNotFound.Body.Bytes(), &errEnvelope)
	if errEnvelope.Error.Code != "NOT_FOUND" {
		t.Fatalf("expected NOT_FOUND code, got: %s", errEnvelope.Error.Code)
	}
}

// E4: Autenticación / Autorización (401 UNAUTHENTICATED, 403 FORBIDDEN, Envelope format)
func TestHandler_E4_AuthAndForbidden(t *testing.T) {
	mux := setupTestServer()

	// Missing JWT (401 UNAUTHENTICATED)
	reqNoAuth := httptest.NewRequest(http.MethodGet, "/environment-types", nil)
	recNoAuth := httptest.NewRecorder()
	mux.ServeHTTP(recNoAuth, reqNoAuth)

	if recNoAuth.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized, got: %d (%s)", recNoAuth.Code, recNoAuth.Body.String())
	}

	var errNoAuth response.ErrorEnvelope
	if err := json.Unmarshal(recNoAuth.Body.Bytes(), &errNoAuth); err != nil {
		t.Fatalf("failed to parse envelope: %v", err)
	}
	if errNoAuth.Error.Code != "UNAUTHENTICATED" || errNoAuth.Error.TraceID == "" {
		t.Fatalf("invalid error structure for 401: %+v", errNoAuth)
	}

	// Valid JWT but missing required feature (403 FORBIDDEN)
	tokenWrongFeature := createTestToken([]string{"ENV_OTHER"}, testJWTSecret, false)
	reqForbidden := httptest.NewRequest(http.MethodGet, "/environment-types", nil)
	reqForbidden.Header.Set("Authorization", "Bearer "+tokenWrongFeature)
	recForbidden := httptest.NewRecorder()
	mux.ServeHTTP(recForbidden, reqForbidden)

	if recForbidden.Code != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden, got: %d (%s)", recForbidden.Code, recForbidden.Body.String())
	}

	var errForbidden response.ErrorEnvelope
	_ = json.Unmarshal(recForbidden.Body.Bytes(), &errForbidden)
	if errForbidden.Error.Code != "FORBIDDEN" {
		t.Fatalf("expected FORBIDDEN error code, got: %s", errForbidden.Error.Code)
	}

	// E4 / CA-07: Role ADMIN without specific feature must be rejected with 403 FORBIDDEN (No bypass)
	tokenAdminNoFeature := createCustomTestToken(nil, []string{"ADMIN"}, 0, testJWTSecret, false)
	reqAdmin := httptest.NewRequest(http.MethodGet, "/environment-types", nil)
	reqAdmin.Header.Set("Authorization", "Bearer "+tokenAdminNoFeature)
	recAdmin := httptest.NewRecorder()
	mux.ServeHTTP(recAdmin, reqAdmin)

	if recAdmin.Code != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden for ADMIN without ENV_VIEW, got: %d (%s)", recAdmin.Code, recAdmin.Body.String())
	}

	// E4: Token with future nbf (Not Before) must be rejected with 401 UNAUTHENTICATED
	futureNbf := time.Now().Add(1 * time.Hour).Unix()
	tokenFutureNbf := createCustomTestToken([]string{"ENV_VIEW"}, nil, futureNbf, testJWTSecret, false)
	reqNbf := httptest.NewRequest(http.MethodGet, "/environment-types", nil)
	reqNbf.Header.Set("Authorization", "Bearer "+tokenFutureNbf)
	recNbf := httptest.NewRecorder()
	mux.ServeHTTP(recNbf, reqNbf)

	if recNbf.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized for token with future nbf, got: %d (%s)", recNbf.Code, recNbf.Body.String())
	}

	// E4: Tampered signature must be rejected with 401 UNAUTHENTICATED
	validToken := createTestToken([]string{"ENV_VIEW"}, testJWTSecret, false)
	tamperedToken := validToken[:len(validToken)-5] + "XXXXX"
	reqTampered := httptest.NewRequest(http.MethodGet, "/environment-types", nil)
	reqTampered.Header.Set("Authorization", "Bearer "+tamperedToken)
	recTampered := httptest.NewRecorder()
	mux.ServeHTTP(recTampered, reqTampered)

	if recTampered.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized for tampered token, got: %d (%s)", recTampered.Code, recTampered.Body.String())
	}
}
