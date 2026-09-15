package http_test

import (
	"bytes"
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

func setupAvailabilityRuleTestServer() (*http.ServeMux, *persistence.MemoryAvailabilityRuleRepository) {
	envRepo := persistence.NewMemoryEnvironmentRepository()
	ruleRepo := persistence.NewMemoryAvailabilityRuleRepository(envRepo)
	ruleUseCase := usecase.NewAvailabilityRuleUseCase(ruleRepo)
	auth := middleware.NewAuthMiddleware(testJWTSecret)
	mux := http.NewServeMux()
	httpAdapter.NewAvailabilityRuleHandler(ruleUseCase).RegisterRoutes(mux, auth)
	return mux, ruleRepo
}

func TestAvailabilityRuleHandler_HU005_CRUDAndFiltering(t *testing.T) {
	mux, ruleRepo := setupAvailabilityRuleTestServer()
	manageToken := createTestToken([]string{"ENV_AVAILABILITY_QUERY"}, testJWTSecret, false)
	viewToken := createTestToken([]string{"ENV_VIEW"}, testJWTSecret, false)
	environmentID := "78901234-5678-9012-3456-789012345678"

	request := func(method, path, token string, payload interface{}) *httptest.ResponseRecorder {
		var body []byte
		if payload != nil {
			body, _ = json.Marshal(payload)
		}
		req := httptest.NewRequest(method, path, bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		return rec
	}

	payload := dto.AvailabilityRuleRequest{
		DayOfWeek: 2, StartTime: "08:00:00", EndTime: "12:00:00",
		EffectiveFrom: "2026-01-01", EffectiveUntil: stringPointer("2026-12-31"),
	}
	created := request(http.MethodPost, "/training-environments/"+environmentID+"/availability-rules", manageToken, payload)
	if created.Code != http.StatusCreated {
		t.Fatalf("expected 201 for rule creation, got %d (%s)", created.Code, created.Body.String())
	}
	var createdRule dto.AvailabilityRuleResponse
	if err := json.Unmarshal(created.Body.Bytes(), &createdRule); err != nil {
		t.Fatalf("failed to decode created rule: %v", err)
	}
	if createdRule.EnvironmentID != environmentID || createdRule.DayOfWeek != 2 || createdRule.EffectiveUntil == nil {
		t.Fatalf("unexpected created rule: %+v", createdRule)
	}

	events := ruleRepo.OutboxEvents()
	if len(events) != 1 || events[0].Payload["change_type"] != "RULE_ADDED" {
		t.Fatalf("expected RULE_ADDED outbox event, got %+v", events)
	}

	listed := request(http.MethodGet, "/training-environments/"+environmentID+"/availability-rules?day_of_week=2", viewToken, nil)
	if listed.Code != http.StatusOK {
		t.Fatalf("expected 200 for filtered list, got %d (%s)", listed.Code, listed.Body.String())
	}
	var rules []dto.AvailabilityRuleResponse
	if err := json.Unmarshal(listed.Body.Bytes(), &rules); err != nil {
		t.Fatalf("failed to decode rule list: %v", err)
	}
	if len(rules) != 1 || rules[0].ID != createdRule.ID {
		t.Fatalf("unexpected filtered rule list: %+v", rules)
	}

	otherEnvironmentID := "89012345-6789-0123-4567-890123456789"
	wrongOwnerGet := request(http.MethodGet, "/training-environments/"+otherEnvironmentID+"/availability-rules/"+createdRule.ID, viewToken, nil)
	if wrongOwnerGet.Code != http.StatusNotFound {
		t.Fatalf("expected 404 when reading a rule through another environment, got %d", wrongOwnerGet.Code)
	}
	wrongOwnerUpdate := request(http.MethodPut, "/training-environments/"+otherEnvironmentID+"/availability-rules/"+createdRule.ID, manageToken, updatedPayloadForTest())
	if wrongOwnerUpdate.Code != http.StatusNotFound {
		t.Fatalf("expected 404 when updating a rule through another environment, got %d", wrongOwnerUpdate.Code)
	}
	wrongOwnerDelete := request(http.MethodDelete, "/training-environments/"+otherEnvironmentID+"/availability-rules/"+createdRule.ID, manageToken, nil)
	if wrongOwnerDelete.Code != http.StatusNotFound {
		t.Fatalf("expected 404 when deleting a rule through another environment, got %d", wrongOwnerDelete.Code)
	}

	updatedPayload := payload
	updatedPayload.DayOfWeek = 3
	updatedPayload.EndTime = "13:00:00"
	updated := request(http.MethodPut, "/training-environments/"+environmentID+"/availability-rules/"+createdRule.ID, manageToken, updatedPayload)
	if updated.Code != http.StatusOK {
		t.Fatalf("expected 200 for rule update, got %d (%s)", updated.Code, updated.Body.String())
	}
	events = ruleRepo.OutboxEvents()
	if len(events) != 2 || events[1].Payload["change_type"] != "RULE_MODIFIED" {
		t.Fatalf("expected RULE_MODIFIED outbox event, got %+v", events)
	}

	deleted := request(http.MethodDelete, "/training-environments/"+environmentID+"/availability-rules/"+createdRule.ID, manageToken, nil)
	if deleted.Code != http.StatusNoContent {
		t.Fatalf("expected 204 for physical deletion, got %d (%s)", deleted.Code, deleted.Body.String())
	}
	missing := request(http.MethodGet, "/training-environments/"+environmentID+"/availability-rules/"+createdRule.ID, viewToken, nil)
	if missing.Code != http.StatusNotFound {
		t.Fatalf("expected deleted rule to be unavailable, got %d (%s)", missing.Code, missing.Body.String())
	}
	events = ruleRepo.OutboxEvents()
	if len(events) != 3 || events[2].Payload["change_type"] != "RULE_REMOVED" {
		t.Fatalf("expected RULE_REMOVED outbox event, got %+v", events)
	}
}

func TestAvailabilityRuleHandler_HU005_ValidationAndNoEventOn4xx(t *testing.T) {
	mux, ruleRepo := setupAvailabilityRuleTestServer()
	token := createTestToken([]string{"ENV_AVAILABILITY_QUERY"}, testJWTSecret, false)
	environmentID := "78901234-5678-9012-3456-789012345678"
	request := func(payload dto.AvailabilityRuleRequest) *httptest.ResponseRecorder {
		body, _ := json.Marshal(payload)
		req := httptest.NewRequest(http.MethodPost, "/training-environments/"+environmentID+"/availability-rules", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		return rec
	}

	cases := []dto.AvailabilityRuleRequest{
		{DayOfWeek: 0, StartTime: "08:00:00", EndTime: "12:00:00", EffectiveFrom: "2026-01-01"},
		{DayOfWeek: 8, StartTime: "08:00:00", EndTime: "12:00:00", EffectiveFrom: "2026-01-01"},
		{DayOfWeek: 1, StartTime: "12:00:00", EndTime: "12:00:00", EffectiveFrom: "2026-01-01"},
		{DayOfWeek: 1, StartTime: "13:00:00", EndTime: "12:00:00", EffectiveFrom: "2026-01-01"},
		{DayOfWeek: 1, StartTime: "08:00:00", EndTime: "12:00:00", EffectiveFrom: "2026-02-01", EffectiveUntil: stringPointer("2026-01-31")},
	}
	for index, testCase := range cases {
		rec := request(testCase)
		if rec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("case %d: expected 422, got %d (%s)", index, rec.Code, rec.Body.String())
		}
	}
	if events := ruleRepo.OutboxEvents(); len(events) != 0 {
		t.Fatalf("invalid 4xx operations must not create outbox events: %+v", events)
	}

	nonexistent := httptest.NewRequest(http.MethodPost, "/training-environments/00000000-0000-0000-0000-000000000000/availability-rules", bytes.NewReader([]byte(`{"day_of_week":1,"start_time":"08:00:00","end_time":"12:00:00","effective_from":"2026-01-01"}`)))
	nonexistent.Header.Set("Authorization", "Bearer "+token)
	nonexistent.Header.Set("Content-Type", "application/json")
	nonexistentRec := httptest.NewRecorder()
	mux.ServeHTTP(nonexistentRec, nonexistent)
	if nonexistentRec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for nonexistent environment, got %d (%s)", nonexistentRec.Code, nonexistentRec.Body.String())
	}
}

func TestAvailabilityRuleHandler_HU005_Maximum24Rules(t *testing.T) {
	mux, ruleRepo := setupAvailabilityRuleTestServer()
	token := createTestToken([]string{"ENV_AVAILABILITY_QUERY", "ENV_VIEW"}, testJWTSecret, false)
	environmentID := "78901234-5678-9012-3456-789012345678"
	createRule := func(day int) *httptest.ResponseRecorder {
		body, _ := json.Marshal(dto.AvailabilityRuleRequest{
			DayOfWeek: day, StartTime: "08:00:00", EndTime: "12:00:00", EffectiveFrom: "2026-01-01",
		})
		req := httptest.NewRequest(http.MethodPost, "/training-environments/"+environmentID+"/availability-rules", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		return rec
	}

	for index := 0; index < 24; index++ {
		rec := createRule((index % 7) + 1)
		if rec.Code != http.StatusCreated {
			t.Fatalf("rule %d: expected 201 within the limit, got %d (%s)", index+1, rec.Code, rec.Body.String())
		}
	}

	twentyFifth := createRule(1)
	if twentyFifth.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected rule 25 to return 422, got %d (%s)", twentyFifth.Code, twentyFifth.Body.String())
	}
	var errorResponse response.ErrorEnvelope
	if err := json.Unmarshal(twentyFifth.Body.Bytes(), &errorResponse); err != nil {
		t.Fatalf("failed to decode rule limit error: %v", err)
	}
	if errorResponse.Error.Code != "UNPROCESSABLE_ENTITY" {
		t.Fatalf("expected UNPROCESSABLE_ENTITY for rule 25, got %q", errorResponse.Error.Code)
	}
	if events := ruleRepo.OutboxEvents(); len(events) != 24 {
		t.Fatalf("rule 25 must not create an Outbox event, got %d events", len(events))
	}
}

func TestAvailabilityRuleHandler_HU005_SecurityAndOwnership(t *testing.T) {
	mux, _ := setupAvailabilityRuleTestServer()
	environmentID := "78901234-5678-9012-3456-789012345678"
	ruleID := "00000000-0000-0000-0000-000000000001"
	request := func(method, path, token string) int {
		req := httptest.NewRequest(method, path, nil)
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		return rec.Code
	}
	protectedRoutes := []struct {
		method  string
		path    string
		feature string
	}{
		{http.MethodGet, "/training-environments/" + environmentID + "/availability-rules", "ENV_VIEW"},
		{http.MethodPost, "/training-environments/" + environmentID + "/availability-rules", "ENV_AVAILABILITY_QUERY"},
		{http.MethodGet, "/training-environments/" + environmentID + "/availability-rules/" + ruleID, "ENV_VIEW"},
		{http.MethodPut, "/training-environments/" + environmentID + "/availability-rules/" + ruleID, "ENV_AVAILABILITY_QUERY"},
		{http.MethodDelete, "/training-environments/" + environmentID + "/availability-rules/" + ruleID, "ENV_AVAILABILITY_QUERY"},
	}
	for _, route := range protectedRoutes {
		if status := request(route.method, route.path, ""); status != http.StatusUnauthorized {
			t.Fatalf("expected 401 for unauthenticated %s %s, got %d", route.method, route.path, status)
		}
		wrongFeature := "ENV_VIEW"
		if route.feature == "ENV_VIEW" {
			wrongFeature = "ENV_AVAILABILITY_QUERY"
		}
		if status := request(route.method, route.path, createTestToken([]string{wrongFeature}, testJWTSecret, false)); status != http.StatusForbidden {
			t.Fatalf("expected 403 for %s %s without %s, got %d", route.method, route.path, route.feature, status)
		}
	}
	if status := request(http.MethodGet, "/training-environments/"+environmentID+"/availability-rules/"+ruleID, createTestToken([]string{"ENV_VIEW"}, testJWTSecret, false)); status != http.StatusNotFound {
		t.Fatalf("expected 404 for unknown rule, got %d", status)
	}
}

func stringPointer(value string) *string { return &value }

func updatedPayloadForTest() dto.AvailabilityRuleRequest {
	return dto.AvailabilityRuleRequest{
		DayOfWeek: 3, StartTime: "09:00:00", EndTime: "13:00:00", EffectiveFrom: "2026-02-01",
	}
}
