package service

import (
	"NewAPI-Gateway/common"
	"NewAPI-Gateway/model"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func setupRequestRecorderTestDB(t *testing.T) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	model.DB = db
	if err := model.DB.AutoMigrate(&model.UsageLog{}, &model.LLMTrace{}, &model.ModelPricing{}); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}
}

func newTestRecorder(t *testing.T) (*requestRecorder, *gin.Context) {
	t.Helper()
	ctx, _ := gin.CreateTestContext(nil)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	recorder := newRequestRecorder(requestRecorderInput{
		AggToken:        &model.AggregatedToken{Id: 1, UserId: 2},
		Provider:        &model.Provider{Id: 3, Name: "openai"},
		Token:           &model.ProviderToken{Id: 4, GroupName: "g1"},
		Context:         ctx,
		RequestId:       "req-live",
		ModelName:       "gpt-4.1",
		Method:          "POST",
		Path:            "/v1/chat/completions",
		RequestedStream: true,
		RequestBody:     []byte(`{"model":"gpt-4.1"}`),
	})
	return recorder, ctx
}

// TestRecorderCreatesPendingRowOnAccept proves the row exists before the
// upstream answers, which is the whole point of the change.
func TestRecorderCreatesPendingRowOnAccept(t *testing.T) {
	setupRequestRecorderTestDB(t)
	recorder, _ := newTestRecorder(t)
	if recorder.usageLogID <= 0 {
		t.Fatalf("expected a pending usage log row, got id %d", recorder.usageLogID)
	}

	var log model.UsageLog
	if err := model.DB.First(&log, recorder.usageLogID).Error; err != nil {
		t.Fatalf("load usage log: %v", err)
	}
	if log.Status != model.UsageLogStatusPending {
		t.Fatalf("expected pending status, got %d", log.Status)
	}
	if log.ModelName != "gpt-4.1" || !log.RequestedStream {
		t.Fatalf("unexpected pending row: %+v", log)
	}
}

// TestRecorderFinishPatchesSameRow checks that progress + finish update the
// original row instead of inserting a second one.
func TestRecorderFinishPatchesSameRow(t *testing.T) {
	setupRequestRecorderTestDB(t)
	recorder, _ := newTestRecorder(t)

	recorder.MarkFirstToken(120)
	recorder.Finish(recordFinishInput{
		Usage:            usageMetrics{ModelName: "gpt-4.1", PromptTokens: 11, CompletionTokens: 22},
		StatusCode:       http.StatusOK,
		ResponseIsStream: true,
		FirstTokenMs:     120,
		ResponseTimeMs:   900,
		RequestBody:      []byte(`{"model":"gpt-4.1"}`),
		ResponseBody:     []byte(`{"ok":true}`),
	})
	WaitForPendingLLMTraces()

	var count int64
	model.DB.Model(&model.UsageLog{}).Count(&count)
	if count != 1 {
		t.Fatalf("expected exactly one usage log, got %d", count)
	}

	var log model.UsageLog
	if err := model.DB.First(&log, recorder.usageLogID).Error; err != nil {
		t.Fatalf("load usage log: %v", err)
	}
	if log.Status != model.UsageLogStatusSuccess {
		t.Fatalf("expected success status, got %d", log.Status)
	}
	if log.PromptTokens != 11 || log.CompletionTokens != 22 {
		t.Fatalf("unexpected tokens: %+v", log)
	}
	if log.FirstTokenMs != 120 || log.ResponseTimeMs != 900 {
		t.Fatalf("unexpected timings: %+v", log)
	}
}

// TestRecorderFinishRecordsError keeps failure semantics intact.
func TestRecorderFinishRecordsError(t *testing.T) {
	setupRequestRecorderTestDB(t)
	recorder, _ := newTestRecorder(t)

	recorder.Finish(recordFinishInput{
		Usage:        usageMetrics{ModelName: "gpt-4.1"},
		StatusCode:   http.StatusBadGateway,
		ErrorMessage: "upstream status 502: bad gateway",
		RequestBody:  []byte(`{"model":"gpt-4.1"}`),
	})
	WaitForPendingLLMTraces()

	var log model.UsageLog
	if err := model.DB.First(&log, recorder.usageLogID).Error; err != nil {
		t.Fatalf("load usage log: %v", err)
	}
	if log.Status != model.UsageLogStatusFailed {
		t.Fatalf("expected failed status, got %d", log.Status)
	}
}

// TestRecorderEnsureFinishedClosesPendingRow makes sure no row can be left
// in-flight when a code path returns without finishing explicitly.
func TestRecorderEnsureFinishedClosesPendingRow(t *testing.T) {
	setupRequestRecorderTestDB(t)
	recorder, _ := newTestRecorder(t)

	recorder.EnsureFinished()
	WaitForPendingLLMTraces()

	var log model.UsageLog
	if err := model.DB.First(&log, recorder.usageLogID).Error; err != nil {
		t.Fatalf("load usage log: %v", err)
	}
	if log.Status == model.UsageLogStatusPending {
		t.Fatalf("expected row to be closed, still pending")
	}
}

// TestRecorderWritesPendingTrace covers the audit side (LLM trace).
func TestRecorderWritesPendingTrace(t *testing.T) {
	setupRequestRecorderTestDB(t)
	previous := common.LLMTraceEnabled
	common.LLMTraceEnabled = true
	defer func() { common.LLMTraceEnabled = previous }()

	recorder, _ := newTestRecorder(t)
	if recorder.traceID <= 0 {
		t.Fatalf("expected a pending trace row, got id %d", recorder.traceID)
	}

	var trace model.LLMTrace
	if err := model.DB.First(&trace, recorder.traceID).Error; err != nil {
		t.Fatalf("load trace: %v", err)
	}
	if !trace.InProgress {
		t.Fatalf("expected trace to be marked in progress")
	}

	recorder.Finish(recordFinishInput{
		Usage:        usageMetrics{ModelName: "gpt-4.1"},
		StatusCode:   http.StatusOK,
		RequestBody:  []byte(`{"model":"gpt-4.1"}`),
		ResponseBody: []byte(`{"ok":true}`),
	})
	WaitForPendingLLMTraces()

	var count int64
	model.DB.Model(&model.LLMTrace{}).Count(&count)
	if count != 1 {
		t.Fatalf("expected exactly one trace, got %d", count)
	}

	if err := model.DB.First(&trace, recorder.traceID).Error; err != nil {
		t.Fatalf("reload trace: %v", err)
	}
	if trace.InProgress {
		t.Fatalf("expected trace to be finalized")
	}
	if trace.ResponseBody != `{"ok":true}` {
		t.Fatalf("unexpected response body: %q", trace.ResponseBody)
	}
}

// TestReapStalePendingRecords verifies abandoned rows get closed.
func TestReapStalePendingRecords(t *testing.T) {
	setupRequestRecorderTestDB(t)
	recorder, _ := newTestRecorder(t)

	reapStalePendingRecords(time.Now().Unix() + 1)

	var log model.UsageLog
	if err := model.DB.First(&log, recorder.usageLogID).Error; err != nil {
		t.Fatalf("load usage log: %v", err)
	}
	if log.Status != model.UsageLogStatusFailed {
		t.Fatalf("expected stale row to be failed, got %d", log.Status)
	}
}
