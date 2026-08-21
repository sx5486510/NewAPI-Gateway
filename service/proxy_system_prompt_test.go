package service

import (
	"NewAPI-Gateway/common"
	"NewAPI-Gateway/model"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestProxyRouteSystemPromptRetryUsesOnlyCurrentRoutePresetAndTracesSentBody(t *testing.T) {
	setupRouteSystemPromptProxyTest(t)
	firstPrompt := model.SystemPrompt{Name: "first", ModelName: "gpt-4", Content: "FIRST ROUTE PRESET"}
	secondPrompt := model.SystemPrompt{Name: "second", ModelName: "gpt-4", Content: "SECOND ROUTE PRESET"}
	if err := model.DB.Create(&firstPrompt).Error; err != nil {
		t.Fatal(err)
	}
	if err := model.DB.Create(&secondPrompt).Error; err != nil {
		t.Fatal(err)
	}

	var firstBody, secondBody []byte
	firstUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		firstBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":{"type":"server_error"}}`))
	}))
	defer firstUpstream.Close()
	secondUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		secondBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[]}`))
	}))
	defer secondUpstream.Close()

	c, recorder := newRouteSystemPromptProxyContext(`{"model":"client-model","messages":[{"role":"user","content":"hello"}]}`)
	firstRoute := model.ModelRoute{Id: 1, ModelName: "gpt-4", SystemPromptId: &firstPrompt.Id}
	secondRoute := model.ModelRoute{Id: 2, ModelName: "gpt-4", SystemPromptId: &secondPrompt.Id}
	firstErr := ProxyToUpstream(c, firstRoute, &model.ProviderToken{Id: 101, SkKey: "key-one"}, &model.Provider{Id: 11, Name: "first", BaseURL: firstUpstream.URL})
	if firstErr == nil || !firstErr.Retryable {
		t.Fatalf("first attempt error = %#v, want retryable", firstErr)
	}
	if secondErr := ProxyToUpstream(c, secondRoute, &model.ProviderToken{Id: 102, SkKey: "key-two"}, &model.Provider{Id: 12, Name: "second", BaseURL: secondUpstream.URL}); secondErr != nil {
		t.Fatalf("second attempt: %v", secondErr)
	}
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", recorder.Code, recorder.Body.String())
	}
	assertSingleRoutePrompt(t, firstBody, "FIRST ROUTE PRESET", "SECOND ROUTE PRESET")
	assertSingleRoutePrompt(t, secondBody, "SECOND ROUTE PRESET", "FIRST ROUTE PRESET")

	// Traces are written off the request goroutine; drain before asserting.
	WaitForPendingLLMTraces()

	var trace model.LLMTrace
	if err := model.DB.Where("provider_id = ?", 12).First(&trace).Error; err != nil {
		t.Fatalf("find second trace: %v", err)
	}
	assertSingleRoutePrompt(t, []byte(trace.RequestBody), "SECOND ROUTE PRESET", "FIRST ROUTE PRESET")
}

func TestProxyRouteSystemPromptInvalidMessagesIsNonRetryable400WithoutUpstream(t *testing.T) {
	setupRouteSystemPromptProxyTest(t)
	prompt := model.SystemPrompt{Name: "preset", ModelName: "gpt-4", Content: "preset"}
	if err := model.DB.Create(&prompt).Error; err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
	}))
	defer upstream.Close()
	c, _ := newRouteSystemPromptProxyContext(`{"model":"gpt-4","messages":{}}`)
	err := ProxyToUpstream(c, model.ModelRoute{Id: 1, ModelName: "gpt-4", SystemPromptId: &prompt.Id}, &model.ProviderToken{Id: 201}, &model.Provider{BaseURL: upstream.URL})
	if err == nil || err.StatusCode != http.StatusBadRequest || err.Retryable || calls.Load() != 0 {
		t.Fatalf("error = %#v, upstream calls = %d", err, calls.Load())
	}
	var response struct {
		Error struct {
			Type string `json:"type"`
			Code string `json:"code"`
		} `json:"error"`
	}
	if json.Unmarshal(err.UpstreamBody, &response) != nil || response.Error.Type != "invalid_request_error" || response.Error.Code != "invalid_messages" {
		t.Fatalf("unexpected error response: %s", err.UpstreamBody)
	}
}

func TestProxyRouteSystemPromptNoBindingLeavesMessagesUnparsed(t *testing.T) {
	setupRouteSystemPromptProxyTest(t)
	var sent []byte
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sent, _ = io.ReadAll(r.Body)
		_, _ = w.Write([]byte(`{"choices":[]}`))
	}))
	defer upstream.Close()
	c, _ := newRouteSystemPromptProxyContext(`{"model":"old","messages":"not-an-array"}`)
	err := ProxyToUpstream(c, model.ModelRoute{ModelName: "new"}, &model.ProviderToken{Id: 301}, &model.Provider{BaseURL: upstream.URL})
	if err != nil {
		t.Fatalf("proxy attempt: %v", err)
	}
	if !strings.Contains(string(sent), `"messages":"not-an-array"`) || !strings.Contains(string(sent), `"model":"new"`) {
		t.Fatalf("unexpected upstream body: %s", sent)
	}
}

func setupRouteSystemPromptProxyTest(t *testing.T) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "proxy-system-prompt.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	model.DB = db
	if err := db.AutoMigrate(&model.SystemPrompt{}, &model.UsageLog{}, &model.LLMTrace{}, &model.ModelPricing{}); err != nil {
		t.Fatal(err)
	}
	oldCooldown := common.GlobalRouteCooldown
	common.GlobalRouteCooldown = common.NewRouteCooldownManager(func() common.RouteCooldownConfig { return common.RouteCooldownConfig{Enabled: false} })
	oldTrace := common.LLMTraceEnabled
	common.LLMTraceEnabled = true
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		common.GlobalRouteCooldown = oldCooldown
		common.LLMTraceEnabled = oldTrace
		_ = sqlDB.Close()
	})
}

func newRouteSystemPromptProxyContext(body string) (*gin.Context, *httptest.ResponseRecorder) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("agg_token", &model.AggregatedToken{Id: 1, UserId: 1})
	c.Set("request_model", "gpt-4")
	c.Set("request_model_resolved", "gpt-4")
	return c, recorder
}

func assertSingleRoutePrompt(t *testing.T, body []byte, want, forbidden string) {
	t.Helper()
	if strings.Count(string(body), want) != 1 || strings.Contains(string(body), forbidden) {
		t.Fatalf("body does not contain exactly current route prompt: %s", body)
	}
	var request struct {
		Messages []map[string]any `json:"messages"`
	}
	if err := json.Unmarshal(body, &request); err != nil {
		t.Fatal(err)
	}
	if len(request.Messages) != 2 || request.Messages[0]["role"] != "system" || request.Messages[0]["content"] != want {
		t.Fatalf("unexpected messages: %#v", request.Messages)
	}
}
