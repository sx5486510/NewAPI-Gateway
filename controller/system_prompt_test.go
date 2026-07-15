package controller

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"NewAPI-Gateway/model"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

type systemPromptResponse struct {
	Success bool            `json:"success"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

func setupSystemPromptControllerTest(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	model.DB = db
	if err := model.DB.AutoMigrate(&model.SystemPrompt{}, &model.ModelRoute{}); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}
	router := gin.New()
	router.GET("/api/system-prompt/", GetSystemPrompts)
	router.POST("/api/system-prompt/", CreateSystemPrompt)
	router.PUT("/api/system-prompt/:id", UpdateSystemPrompt)
	router.DELETE("/api/system-prompt/:id", DeleteSystemPrompt)
	return router
}

func performSystemPromptRequest(t *testing.T, router *gin.Engine, method, target, body string) systemPromptResponse {
	t.Helper()
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(method, target, bytes.NewBufferString(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	router.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
	var response systemPromptResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v; body=%s", err, recorder.Body.String())
	}
	return response
}

func TestSystemPromptCreateListAndUpdate(t *testing.T) {
	router := setupSystemPromptControllerTest(t)
	created := performSystemPromptRequest(t, router, http.MethodPost, "/api/system-prompt/", `{"name":" main ","model_name":" gpt-4 ","content":" old "}`)
	if !created.Success || created.Message != "" {
		t.Fatalf("create failed: %+v", created)
	}
	var prompt model.SystemPrompt
	if err := json.Unmarshal(created.Data, &prompt); err != nil {
		t.Fatalf("decode created prompt: %v", err)
	}
	if prompt.Id <= 0 || prompt.Name != "main" || prompt.ModelName != "gpt-4" || prompt.Content != "old" {
		t.Fatalf("unexpected created prompt: %+v", prompt)
	}

	updated := performSystemPromptRequest(t, router, http.MethodPut, "/api/system-prompt/"+strconv.Itoa(prompt.Id), `{"id":999,"name":"revised","model_name":"gpt-4o","content":"new"}`)
	if !updated.Success {
		t.Fatalf("update failed: %s", updated.Message)
	}
	var updatedPrompt model.SystemPrompt
	if err := json.Unmarshal(updated.Data, &updatedPrompt); err != nil {
		t.Fatalf("decode updated prompt: %v", err)
	}
	if updatedPrompt.Id != prompt.Id || updatedPrompt.Name != "revised" || updatedPrompt.ModelName != "gpt-4o" || updatedPrompt.Content != "new" {
		t.Fatalf("path id was not authoritative or fields not updated: %+v", updatedPrompt)
	}

	listed := performSystemPromptRequest(t, router, http.MethodGet, "/api/system-prompt/", "")
	var prompts []model.SystemPrompt
	if err := json.Unmarshal(listed.Data, &prompts); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if !listed.Success || len(prompts) != 1 || prompts[0].Id != prompt.Id {
		t.Fatalf("unexpected list response: %+v data=%s", listed, listed.Data)
	}
}

func TestSystemPromptListFiltersExactModelAndNameKeyword(t *testing.T) {
	router := setupSystemPromptControllerTest(t)
	for _, prompt := range []*model.SystemPrompt{
		{Name: "Alpha helper", ModelName: "gpt-4", Content: "one"},
		{Name: "Beta helper", ModelName: "gpt-4o", Content: "two"},
		{Name: "Gamma", ModelName: "gpt-4", Content: "three"},
	} {
		if err := model.CreateSystemPrompt(prompt); err != nil {
			t.Fatal(err)
		}
	}
	response := performSystemPromptRequest(t, router, http.MethodGet, "/api/system-prompt/?model=gpt-4&keyword=helper", "")
	var prompts []model.SystemPrompt
	if err := json.Unmarshal(response.Data, &prompts); err != nil {
		t.Fatal(err)
	}
	if !response.Success || len(prompts) != 1 || prompts[0].Name != "Alpha helper" {
		t.Fatalf("filters were not applied exactly: %+v", prompts)
	}
}

func TestSystemPromptDuplicateNameReturnsUsefulError(t *testing.T) {
	router := setupSystemPromptControllerTest(t)
	if !performSystemPromptRequest(t, router, http.MethodPost, "/api/system-prompt/", `{"name":"main","model_name":"gpt-4","content":"one"}`).Success {
		t.Fatal("initial create failed")
	}
	response := performSystemPromptRequest(t, router, http.MethodPost, "/api/system-prompt/", `{"name":"main","model_name":"gpt-4","content":"two"}`)
	if response.Success || response.Message == "" {
		t.Fatalf("expected duplicate-name error, got %+v", response)
	}
}

func TestSystemPromptRejectsInvalidAndNonpositiveIDs(t *testing.T) {
	for _, tc := range []struct{ method, id, body string }{
		{http.MethodPut, "abc", `{}`},
		{http.MethodPut, "0", `{}`},
		{http.MethodDelete, "-1", ""},
	} {
		t.Run(tc.method+tc.id, func(t *testing.T) {
			router := setupSystemPromptControllerTest(t)
			response := performSystemPromptRequest(t, router, tc.method, "/api/system-prompt/"+tc.id, tc.body)
			if response.Success || response.Message == "" {
				t.Fatalf("expected useful invalid-id error, got %+v", response)
			}
		})
	}
}

func TestSystemPromptMissingRecordDoesNotSucceed(t *testing.T) {
	for _, tc := range []struct{ method, body string }{
		{http.MethodPut, `{"name":"missing","model_name":"gpt-4","content":"x"}`},
		{http.MethodDelete, ""},
	} {
		t.Run(tc.method, func(t *testing.T) {
			router := setupSystemPromptControllerTest(t)
			response := performSystemPromptRequest(t, router, tc.method, "/api/system-prompt/999", tc.body)
			if response.Success || response.Message == "" {
				t.Fatalf("missing record silently succeeded: %+v", response)
			}
		})
	}
}

func TestSystemPromptReferencedDeleteReportsCountThenUnbinds(t *testing.T) {
	router := setupSystemPromptControllerTest(t)
	prompt := &model.SystemPrompt{Name: "main", ModelName: "gpt-4", Content: "content"}
	if err := model.CreateSystemPrompt(prompt); err != nil {
		t.Fatal(err)
	}
	route := &model.ModelRoute{ModelName: "gpt-4", ProviderId: 1, ProviderTokenId: 1, SystemPromptId: &prompt.Id}
	if err := model.DB.Create(route).Error; err != nil {
		t.Fatal(err)
	}

	response := performSystemPromptRequest(t, router, http.MethodDelete, "/api/system-prompt/"+strconv.Itoa(prompt.Id), "")
	var conflictData struct {
		RouteCount int64 `json:"route_count"`
	}
	if err := json.Unmarshal(response.Data, &conflictData); err != nil {
		t.Fatalf("decode referenced-delete data: %v", err)
	}
	if response.Success || conflictData.RouteCount != 1 || response.Message == "" {
		t.Fatalf("expected referenced delete response with count: %+v data=%s", response, response.Data)
	}

	response = performSystemPromptRequest(t, router, http.MethodDelete, "/api/system-prompt/"+strconv.Itoa(prompt.Id)+"?unbind=TrUe", "")
	if !response.Success {
		t.Fatalf("explicit unbind failed: %s", response.Message)
	}
	var deleteData struct {
		Unbound int64 `json:"unbound"`
	}
	if err := json.Unmarshal(response.Data, &deleteData); err != nil {
		t.Fatalf("decode delete data: %v", err)
	}
	if deleteData.Unbound != 1 {
		t.Fatalf("expected one unbound route, got %d", deleteData.Unbound)
	}
	var storedRoute model.ModelRoute
	if err := model.DB.First(&storedRoute, route.Id).Error; err != nil {
		t.Fatal(err)
	}
	if storedRoute.SystemPromptId != nil {
		t.Fatalf("route still references prompt: %+v", storedRoute)
	}
	if err := model.DB.First(&model.SystemPrompt{}, prompt.Id).Error; err != gorm.ErrRecordNotFound {
		t.Fatalf("prompt was not deleted: %v", err)
	}
}
