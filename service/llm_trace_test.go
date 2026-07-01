package service

import (
	"NewAPI-Gateway/common"
	"NewAPI-Gateway/model"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func setupServiceTraceTestDB(t *testing.T) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	model.DB = db
	if err := model.DB.AutoMigrate(&model.LLMTrace{}); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}
}

func TestCaptureLLMTraceDisabledDoesNotWrite(t *testing.T) {
	setupServiceTraceTestDB(t)
	common.LLMTraceEnabled = false

	ctx, _ := gin.CreateTestContext(nil)
	input := llmTraceInput{
		AggToken:     &model.AggregatedToken{Id: 1, UserId: 2},
		Provider:     &model.Provider{Id: 3, Name: "openai"},
		Token:        &model.ProviderToken{Id: 4},
		Context:      ctx,
		RequestId:    "req-disabled",
		ModelName:    "gpt-4.1",
		Method:       "POST",
		Path:         "/v1/chat/completions",
		StatusCode:   200,
		RequestBody:  []byte(`{"messages":[]}`),
		ResponseBody: []byte(`{"ok":true}`),
	}

	captureLLMTrace(input)

	var count int64
	if err := model.DB.Model(&model.LLMTrace{}).Count(&count).Error; err != nil {
		t.Fatalf("count traces: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected no traces, got %d", count)
	}
}

func TestCaptureLLMTraceEnabledWritesTrace(t *testing.T) {
	setupServiceTraceTestDB(t)
	common.LLMTraceEnabled = true
	defer func() { common.LLMTraceEnabled = false }()

	ctx, _ := gin.CreateTestContext(nil)
	ctx.Request = httptestRequest("POST", "/v1/chat/completions", "agent")
	input := llmTraceInput{
		AggToken:         &model.AggregatedToken{Id: 1, UserId: 2},
		Provider:         &model.Provider{Id: 3, Name: "openai"},
		Token:            &model.ProviderToken{Id: 4},
		Context:          ctx,
		RequestId:        "req-enabled",
		ModelName:        "gpt-4.1",
		Method:           "POST",
		Path:             "/v1/chat/completions",
		StatusCode:       200,
		RequestedStream:  false,
		ResponseIsStream: false,
		RequestBody:      []byte(`{"model":"gpt-4.1"}`),
		ResponseBody:     []byte(`{"choices":[]}`),
	}

	captureLLMTrace(input)

	var trace model.LLMTrace
	if err := model.DB.First(&trace, "request_id = ?", "req-enabled").Error; err != nil {
		t.Fatalf("find trace: %v", err)
	}
	if trace.RequestBody != `{"model":"gpt-4.1"}` || trace.ResponseBody != `{"choices":[]}` {
		t.Fatalf("unexpected bodies: request=%q response=%q", trace.RequestBody, trace.ResponseBody)
	}
	if strings.Contains(trace.RequestBody, "sk-") || strings.Contains(trace.ResponseBody, "sk-") {
		t.Fatalf("trace body contains key-like content")
	}
}

func TestTraceStreamCaptureRespectsEnabledFlag(t *testing.T) {
	common.LLMTraceEnabled = false
	disabled := newTraceStreamCapture()
	disabled.appendLine("data: one")
	if disabled.String() != "" {
		t.Fatalf("disabled capture should stay empty")
	}

	common.LLMTraceEnabled = true
	defer func() { common.LLMTraceEnabled = false }()
	enabled := newTraceStreamCapture()
	enabled.appendLine("data: one")
	enabled.appendLine("data: two")
	if got := enabled.String(); got != "data: one\ndata: two\n" {
		t.Fatalf("unexpected stream capture %q", got)
	}
}

func httptestRequest(method string, path string, userAgent string) *http.Request {
	req := httptest.NewRequest(method, path, nil)
	req.Header.Set("User-Agent", userAgent)
	return req
}
