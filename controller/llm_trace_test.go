package controller

import (
	"NewAPI-Gateway/model"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func setupControllerTraceTestDB(t *testing.T) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	model.DB = db
	if err := model.DB.AutoMigrate(&model.LLMTrace{}, &model.UsageLog{}); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}
}

func TestGetLLMTraces(t *testing.T) {
	setupControllerTraceTestDB(t)
	if err := (&model.LLMTrace{RequestId: "req-1", ModelName: "gpt-4.1", ProviderName: "openai", RequestBody: "secret request", ResponseBody: "secret response", StatusCode: 200}).Insert(); err != nil {
		t.Fatalf("insert trace: %v", err)
	}

	router := gin.New()
	router.GET("/api/llm-trace/", GetLLMTraces)

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/llm-trace/?keyword=gpt", nil)
	router.ServeHTTP(recorder, req)

	var payload struct {
		Success bool `json:"success"`
		Data    struct {
			Items []model.LLMTrace `json:"items"`
			Total int64            `json:"total"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !payload.Success || payload.Data.Total != 1 || len(payload.Data.Items) != 1 {
		t.Fatalf("unexpected response: %+v", payload)
	}
	if payload.Data.Items[0].RequestBody != "" || payload.Data.Items[0].ResponseBody != "" {
		t.Fatalf("list response must not include bodies")
	}
}

func TestGetLLMTrace(t *testing.T) {
	setupControllerTraceTestDB(t)
	trace := &model.LLMTrace{RequestId: "req-1", ModelName: "gpt-4.1", RequestBody: "request", ResponseBody: "response", StatusCode: 200}
	if err := trace.Insert(); err != nil {
		t.Fatalf("insert trace: %v", err)
	}

	router := gin.New()
	router.GET("/api/llm-trace/:id", GetLLMTrace)

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/llm-trace/1", nil)
	router.ServeHTTP(recorder, req)

	var payload struct {
		Success bool           `json:"success"`
		Data    model.LLMTrace `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !payload.Success || payload.Data.RequestBody != "request" || payload.Data.ResponseBody != "response" {
		t.Fatalf("unexpected detail response: %+v", payload)
	}
}

func TestDeleteLLMTraces(t *testing.T) {
	setupControllerTraceTestDB(t)
	if err := (&model.LLMTrace{RequestId: "req-1"}).Insert(); err != nil {
		t.Fatalf("insert trace: %v", err)
	}
	if err := (&model.UsageLog{RequestId: "req-1"}).Insert(); err != nil {
		t.Fatalf("insert usage log: %v", err)
	}

	router := gin.New()
	router.DELETE("/api/llm-trace/", DeleteLLMTraces)

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/llm-trace/", nil)
	router.ServeHTTP(recorder, req)

	var payload struct {
		Success bool `json:"success"`
		Data    struct {
			Deleted int64 `json:"deleted"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !payload.Success || payload.Data.Deleted != 1 {
		t.Fatalf("unexpected delete response: %+v", payload)
	}

	var usageCount int64
	if err := model.DB.Model(&model.UsageLog{}).Count(&usageCount).Error; err != nil {
		t.Fatalf("count usage logs: %v", err)
	}
	if usageCount != 1 {
		t.Fatalf("usage logs should remain, got %d", usageCount)
	}
}
