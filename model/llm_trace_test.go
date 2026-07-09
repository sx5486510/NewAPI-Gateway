package model

import (
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func setupLLMTraceTestDB(t *testing.T) {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open in-memory sqlite: %v", err)
	}

	DB = db
	if err := DB.AutoMigrate(&LLMTrace{}, &UsageLog{}); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}
}

func TestLLMTraceInsertAndQuery(t *testing.T) {
	setupLLMTraceTestDB(t)

	trace := &LLMTrace{
		RequestId:         "req-123",
		UserId:            7,
		AggregatedTokenId: 11,
		ProviderId:        13,
		ProviderName:      "openai",
		ProviderTokenId:   17,
		TokenGroupName:    "vip",
		ModelName:         "gpt-4.1",
		Method:            "POST",
		Path:              "/v1/chat/completions",
		StatusCode:        200,
		RequestedStream:   true,
		ResponseIsStream:  true,
		RequestBody:       `{"messages":[{"role":"user","content":"hello"}]}`,
		ResponseBody:      `{"choices":[{"message":{"content":"hi"}}]}`,
		ClientIp:          "203.0.113.10",
		UserAgent:         "trace-test",
	}

	if err := trace.Insert(); err != nil {
		t.Fatalf("insert trace: %v", err)
	}
	if trace.Id == 0 {
		t.Fatal("expected inserted trace id to be set")
	}
	if trace.CreatedAt == 0 {
		t.Fatal("expected CreatedAt to be set during insert")
	}

	traces, total, err := QueryLLMTraces(LLMTraceQuery{Limit: 10})
	if err != nil {
		t.Fatalf("query traces: %v", err)
	}
	if total != 1 {
		t.Fatalf("expected total 1, got %d", total)
	}
	if len(traces) != 1 {
		t.Fatalf("expected one trace, got %d", len(traces))
	}
	if traces[0].RequestId != "req-123" {
		t.Fatalf("expected request id req-123, got %q", traces[0].RequestId)
	}
	if traces[0].TokenGroupName != "vip" {
		t.Fatalf("expected list token group vip, got %q", traces[0].TokenGroupName)
	}
	if traces[0].RequestBody != "" || traces[0].ResponseBody != "" {
		t.Fatalf("expected metadata query to omit bodies, got request=%q response=%q", traces[0].RequestBody, traces[0].ResponseBody)
	}

	fullTrace, err := GetLLMTraceByID(trace.Id)
	if err != nil {
		t.Fatalf("get trace by id: %v", err)
	}
	if fullTrace.RequestBody != trace.RequestBody {
		t.Fatalf("expected full request body, got %q", fullTrace.RequestBody)
	}
	if fullTrace.TokenGroupName != "vip" {
		t.Fatalf("expected full token group vip, got %q", fullTrace.TokenGroupName)
	}
	if fullTrace.ResponseBody != trace.ResponseBody {
		t.Fatalf("expected full response body, got %q", fullTrace.ResponseBody)
	}
}

func TestQueryLLMTracesFilters(t *testing.T) {
	setupLLMTraceTestDB(t)

	traces := []*LLMTrace{
		{
			RequestId:    "req-success-openai",
			ProviderId:   1,
			ProviderName: "openai",
			ModelName:    "gpt-4.1",
			StatusCode:   200,
			ClientIp:     "198.51.100.10",
			UserAgent:    "desktop-client",
		},
		{
			RequestId:    "req-success-anthropic",
			ProviderId:   2,
			ProviderName: "anthropic",
			ModelName:    "claude-sonnet-4",
			StatusCode:   302,
			ClientIp:     "198.51.100.11",
			UserAgent:    "mobile-client",
		},
		{
			RequestId:    "req-error-openai",
			ProviderId:   1,
			ProviderName: "openai",
			ModelName:    "gpt-4.1-mini",
			StatusCode:   500,
			ErrorMessage: "upstream timeout",
			ClientIp:     "198.51.100.12",
			UserAgent:    "worker-client",
		},
		{
			RequestId:    "req-error-message",
			ProviderId:   3,
			ProviderName: "gemini",
			ModelName:    "gemini-2.5-pro",
			StatusCode:   200,
			ErrorMessage: "quota exceeded",
			ClientIp:     "198.51.100.13",
			UserAgent:    "batch-client",
		},
	}

	for _, trace := range traces {
		if err := trace.Insert(); err != nil {
			t.Fatalf("insert trace %s: %v", trace.RequestId, err)
		}
	}

	filterTests := []struct {
		name  string
		query LLMTraceQuery
		want  []string
	}{
		{
			name:  "success status",
			query: LLMTraceQuery{Status: "success"},
			want:  []string{"req-success-anthropic", "req-success-openai"},
		},
		{
			name:  "error status",
			query: LLMTraceQuery{Status: "error"},
			want:  []string{"req-error-message", "req-error-openai"},
		},
		{
			name:  "provider exact",
			query: LLMTraceQuery{ProviderName: "openai"},
			want:  []string{"req-error-openai", "req-success-openai"},
		},
		{
			name:  "keyword",
			query: LLMTraceQuery{Keyword: "quota"},
			want:  []string{"req-error-message"},
		},
	}

	for _, tt := range filterTests {
		t.Run(tt.name, func(t *testing.T) {
			got, total, err := QueryLLMTraces(tt.query)
			if err != nil {
				t.Fatalf("query traces: %v", err)
			}
			if int(total) != len(tt.want) {
				t.Fatalf("expected total %d, got %d", len(tt.want), total)
			}
			gotIDs := make([]string, 0, len(got))
			for _, trace := range got {
				gotIDs = append(gotIDs, trace.RequestId)
			}
			if strings.Join(gotIDs, ",") != strings.Join(tt.want, ",") {
				t.Fatalf("expected request ids %v, got %v", tt.want, gotIDs)
			}
		})
	}
}

func TestDeleteAllLLMTracesDoesNotDeleteUsageLogs(t *testing.T) {
	setupLLMTraceTestDB(t)

	if err := (&LLMTrace{RequestId: "req-delete", ProviderName: "openai", ModelName: "gpt-4.1", StatusCode: 200}).Insert(); err != nil {
		t.Fatalf("insert trace: %v", err)
	}
	if err := (&UsageLog{RequestId: "usage-1", UserId: 1, Status: 1}).Insert(); err != nil {
		t.Fatalf("insert usage log: %v", err)
	}

	rowsAffected, err := DeleteAllLLMTraces()
	if err != nil {
		t.Fatalf("delete all traces: %v", err)
	}
	if rowsAffected != 1 {
		t.Fatalf("expected one deleted trace, got %d", rowsAffected)
	}

	var traceCount int64
	if err := DB.Model(&LLMTrace{}).Count(&traceCount).Error; err != nil {
		t.Fatalf("count traces: %v", err)
	}
	if traceCount != 0 {
		t.Fatalf("expected no traces, got %d", traceCount)
	}

	var usageLogCount int64
	if err := DB.Model(&UsageLog{}).Count(&usageLogCount).Error; err != nil {
		t.Fatalf("count usage logs: %v", err)
	}
	if usageLogCount != 1 {
		t.Fatalf("expected usage log to remain, got %d", usageLogCount)
	}
}
