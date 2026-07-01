# LLM Trace Audit Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build an optional LLM relay context audit trail with full request/response capture, admin viewing, and history clearing.

**Architecture:** Add a separate `LLMTrace` persistence model linked to existing `UsageLog` rows by `request_id`. Capture traces inside `service.ProxyToUpstream`, gated by a default-off `LLMTraceEnabled` option, and expose admin-only APIs plus a small React UI for settings, listing, detail viewing, and clearing history.

**Tech Stack:** Go, Gin, GORM, SQLite/MySQL/PostgreSQL via existing GORM config, React, existing API helper and UI components.

---

## Baseline Notes

The implementation worktree is `E:\NewAPI-Gateway\.worktrees\llm-trace-audit` on branch `feature/llm-trace-audit`.

Initial `go test ./...` in the worktree fails before package tests run for the root package because `main.go` embeds `web/build`, which does not exist in the checkout:

```text
main.go:28:12: pattern web/build: no matching files found
```

Package-level backend tests are usable:

```powershell
go test ./common ./model ./service ./controller ./router
```

Before final full-repo verification, either build the frontend into `web/build` or use package-level Go tests plus frontend build checks.

## File Structure

- Modify `common/constants.go`: add default `LLMTraceEnabled = false`.
- Modify `model/option.go`: expose `LLMTraceEnabled` through `OptionMap` and update `common.LLMTraceEnabled`.
- Create `model/llm_trace.go`: define `LLMTrace`, query structs, insert/list/detail/delete helpers.
- Create `model/llm_trace_test.go`: verify query behavior and delete isolation.
- Modify `model/main.go`: add `AutoMigrate(&LLMTrace{})`.
- Create `service/llm_trace.go`: trace capture helpers used by relay proxy.
- Create `service/llm_trace_test.go`: verify disabled/enabled trace writes and stream builder behavior.
- Modify `service/proxy.go`: call trace helper in transport error, upstream error, streaming success/error, non-streaming success, and response-read error paths.
- Create `controller/llm_trace.go`: admin list/detail/delete handlers.
- Create `controller/llm_trace_test.go`: handler-level tests for list/detail/delete.
- Modify `router/api-router.go`: register `/api/llm-trace` admin routes.
- Modify `web/src/components/SystemSetting.js`: add `LLMTraceEnabled` state and checkbox.
- Create `web/src/pages/LLMTrace/index.js`: trace list page with filters, detail modal, and clear button.
- Modify `web/src/App.js`: lazy-load and route `/llm-trace`.
- Modify `web/src/components/Layout.js`: add a trace navigation item for admin users.

## Task 1: Add LLM Trace Persistence Model

**Files:**
- Create: `model/llm_trace.go`
- Create: `model/llm_trace_test.go`
- Modify: `model/main.go`

- [ ] **Step 1: Write model tests**

Create `model/llm_trace_test.go`:

```go
package model

import (
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupLLMTraceTestDB(t *testing.T) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	DB = db
	if err := DB.AutoMigrate(&LLMTrace{}, &UsageLog{}); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}
}

func TestLLMTraceInsertAndQuery(t *testing.T) {
	setupLLMTraceTestDB(t)

	trace := &LLMTrace{
		RequestId:         "req-1",
		UserId:            7,
		AggregatedTokenId: 11,
		ProviderId:        13,
		ProviderName:      "openai",
		ProviderTokenId:   17,
		ModelName:         "gpt-4.1",
		Method:            "POST",
		Path:              "/v1/chat/completions",
		StatusCode:        200,
		RequestedStream:   false,
		ResponseIsStream:  false,
		RequestBody:       `{"model":"gpt-4.1","messages":[{"role":"user","content":"hi"}]}`,
		ResponseBody:      `{"choices":[{"message":{"content":"hello"}}]}`,
		ClientIp:          "127.0.0.1",
		UserAgent:         "test-agent",
	}
	if err := trace.Insert(); err != nil {
		t.Fatalf("insert trace: %v", err)
	}
	if trace.CreatedAt == 0 {
		t.Fatalf("expected CreatedAt to be set")
	}

	items, total, err := QueryLLMTraces(LLMTraceQuery{Limit: 10, Keyword: "gpt-4.1"})
	if err != nil {
		t.Fatalf("query traces: %v", err)
	}
	if total != 1 || len(items) != 1 {
		t.Fatalf("expected one trace, got total=%d len=%d", total, len(items))
	}
	if items[0].RequestBody != "" || items[0].ResponseBody != "" {
		t.Fatalf("list query must not include full bodies")
	}

	got, err := GetLLMTraceByID(trace.Id)
	if err != nil {
		t.Fatalf("get trace: %v", err)
	}
	if got.RequestBody != trace.RequestBody || got.ResponseBody != trace.ResponseBody {
		t.Fatalf("detail query did not return full bodies")
	}
}

func TestQueryLLMTracesFilters(t *testing.T) {
	setupLLMTraceTestDB(t)
	now := time.Now().Unix()
	traces := []*LLMTrace{
		{RequestId: "ok-1", ProviderName: "openai", ModelName: "gpt-4.1", StatusCode: 200, CreatedAt: now},
		{RequestId: "err-1", ProviderName: "anthropic", ModelName: "claude-3-5", StatusCode: 429, ErrorMessage: "rate_limit", CreatedAt: now + 1},
	}
	for _, trace := range traces {
		if err := trace.Insert(); err != nil {
			t.Fatalf("insert trace: %v", err)
		}
	}

	errorItems, total, err := QueryLLMTraces(LLMTraceQuery{Limit: 10, Status: "error"})
	if err != nil {
		t.Fatalf("query error traces: %v", err)
	}
	if total != 1 || errorItems[0].RequestId != "err-1" {
		t.Fatalf("expected err-1, got total=%d items=%v", total, errorItems)
	}

	providerItems, total, err := QueryLLMTraces(LLMTraceQuery{Limit: 10, ProviderName: "openai"})
	if err != nil {
		t.Fatalf("query provider traces: %v", err)
	}
	if total != 1 || providerItems[0].RequestId != "ok-1" {
		t.Fatalf("expected ok-1, got total=%d items=%v", total, providerItems)
	}
}

func TestDeleteAllLLMTracesDoesNotDeleteUsageLogs(t *testing.T) {
	setupLLMTraceTestDB(t)
	if err := (&LLMTrace{RequestId: "req-1", ModelName: "gpt-4.1"}).Insert(); err != nil {
		t.Fatalf("insert trace: %v", err)
	}
	if err := (&UsageLog{RequestId: "req-1", ModelName: "gpt-4.1"}).Insert(); err != nil {
		t.Fatalf("insert usage log: %v", err)
	}

	deleted, err := DeleteAllLLMTraces()
	if err != nil {
		t.Fatalf("delete traces: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("expected deleted=1, got %d", deleted)
	}

	var traceCount int64
	if err := DB.Model(&LLMTrace{}).Count(&traceCount).Error; err != nil {
		t.Fatalf("count traces: %v", err)
	}
	if traceCount != 0 {
		t.Fatalf("expected no traces, got %d", traceCount)
	}

	var usageCount int64
	if err := DB.Model(&UsageLog{}).Count(&usageCount).Error; err != nil {
		t.Fatalf("count usage logs: %v", err)
	}
	if usageCount != 1 {
		t.Fatalf("expected usage log to remain, got %d", usageCount)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run:

```powershell
go test ./model -run "TestLLMTrace|TestQueryLLMTraces|TestDeleteAllLLMTraces" -count=1
```

Expected: fail with undefined `LLMTrace`, `LLMTraceQuery`, `QueryLLMTraces`, `GetLLMTraceByID`, and `DeleteAllLLMTraces`.

- [ ] **Step 3: Add the model implementation**

Create `model/llm_trace.go`:

```go
package model

import (
	"errors"
	"math"
	"strings"
	"time"

	"gorm.io/gorm"
)

type LLMTrace struct {
	Id                int64  `json:"id" gorm:"primaryKey;autoIncrement"`
	RequestId         string `json:"request_id" gorm:"type:varchar(64);index"`
	UserId            int    `json:"user_id" gorm:"index"`
	AggregatedTokenId int    `json:"aggregated_token_id"`
	ProviderId        int    `json:"provider_id" gorm:"index"`
	ProviderName      string `json:"provider_name" gorm:"type:varchar(128);index"`
	ProviderTokenId   int    `json:"provider_token_id"`
	ModelName         string `json:"model_name" gorm:"type:varchar(255);index"`
	Method            string `json:"method" gorm:"type:varchar(16)"`
	Path              string `json:"path" gorm:"type:varchar(512)"`
	StatusCode        int    `json:"status_code" gorm:"index"`
	RequestedStream   bool   `json:"requested_stream"`
	ResponseIsStream  bool   `json:"response_is_stream"`
	RequestBody       string `json:"request_body" gorm:"type:text"`
	ResponseBody      string `json:"response_body" gorm:"type:text"`
	ErrorMessage      string `json:"error_message" gorm:"type:text"`
	ClientIp          string `json:"client_ip" gorm:"type:varchar(64)"`
	UserAgent         string `json:"user_agent" gorm:"type:varchar(512)"`
	CreatedAt         int64  `json:"created_at" gorm:"index"`
}

type LLMTraceQuery struct {
	Offset       int
	Limit        int
	Keyword      string
	ProviderName string
	ModelName    string
	Status       string
}

func (t *LLMTrace) Insert() error {
	if t.CreatedAt == 0 {
		t.CreatedAt = time.Now().Unix()
	}
	return DB.Model(&LLMTrace{}).Create(t).Error
}

func applyLLMTraceFilters(db *gorm.DB, query LLMTraceQuery) *gorm.DB {
	if providerName := strings.TrimSpace(query.ProviderName); providerName != "" {
		db = db.Where("provider_name = ?", providerName)
	}
	if modelName := strings.TrimSpace(query.ModelName); modelName != "" {
		db = db.Where("model_name = ?", modelName)
	}
	if keyword := strings.TrimSpace(query.Keyword); keyword != "" {
		like := "%" + keyword + "%"
		db = db.Where(
			"(request_id LIKE ? OR model_name LIKE ? OR provider_name LIKE ? OR error_message LIKE ? OR client_ip LIKE ? OR user_agent LIKE ?)",
			like, like, like, like, like, like,
		)
	}
	switch strings.TrimSpace(query.Status) {
	case "success":
		db = db.Where("status_code >= 200 AND status_code < 400 AND (error_message IS NULL OR TRIM(error_message) = '')")
	case "error":
		db = db.Where("status_code >= 400 OR (error_message IS NOT NULL AND TRIM(error_message) <> '')")
	}
	return db
}

func QueryLLMTraces(query LLMTraceQuery) ([]*LLMTrace, int64, error) {
	if query.Limit <= 0 {
		query.Limit = 15
	}
	if query.Offset < 0 {
		query.Offset = 0
	}

	baseQuery := applyLLMTraceFilters(DB.Model(&LLMTrace{}), query)
	var total int64
	if err := baseQuery.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var traces []*LLMTrace
	err := baseQuery.
		Select("id", "request_id", "user_id", "aggregated_token_id", "provider_id", "provider_name", "provider_token_id", "model_name", "method", "path", "status_code", "requested_stream", "response_is_stream", "error_message", "client_ip", "user_agent", "created_at").
		Order("id desc").
		Limit(query.Limit).
		Offset(query.Offset).
		Find(&traces).Error
	return traces, total, err
}

func GetLLMTraceByID(id int64) (*LLMTrace, error) {
	if id <= 0 || id > math.MaxInt64 {
		return nil, errors.New("invalid trace id")
	}
	var trace LLMTrace
	if err := DB.First(&trace, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &trace, nil
}

func DeleteAllLLMTraces() (int64, error) {
	result := DB.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&LLMTrace{})
	return result.RowsAffected, result.Error
}
```

- [ ] **Step 4: Add migration**

Modify `model/main.go` inside `InitDB`, immediately after `AutoMigrate(&UsageLog{})` succeeds:

```go
		err = db.AutoMigrate(&LLMTrace{})
		if err != nil {
			return err
		}
```

- [ ] **Step 5: Run model tests**

Run:

```powershell
gofmt -w model\llm_trace.go model\llm_trace_test.go model\main.go
go test ./model -run "TestLLMTrace|TestQueryLLMTraces|TestDeleteAllLLMTraces" -count=1
```

Expected: pass.

- [ ] **Step 6: Commit**

Run:

```powershell
git add model\llm_trace.go model\llm_trace_test.go model\main.go
git commit -m "feat: add llm trace model"
```

## Task 2: Add Trace Configuration Switch

**Files:**
- Modify: `common/constants.go`
- Modify: `model/option.go`
- Create: `model/option_trace_test.go`

- [ ] **Step 1: Write option tests**

Create `model/option_trace_test.go`:

```go
package model

import (
	"NewAPI-Gateway/common"
	"testing"
)

func TestLLMTraceOptionUpdatesCommonFlag(t *testing.T) {
	common.LLMTraceEnabled = false
	common.OptionMap = map[string]string{}

	updateOptionMap("LLMTraceEnabled", "true")
	if !common.LLMTraceEnabled {
		t.Fatalf("expected LLMTraceEnabled true")
	}

	updateOptionMap("LLMTraceEnabled", "false")
	if common.LLMTraceEnabled {
		t.Fatalf("expected LLMTraceEnabled false")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run:

```powershell
go test ./model -run TestLLMTraceOptionUpdatesCommonFlag -count=1
```

Expected: fail with undefined `common.LLMTraceEnabled`.

- [ ] **Step 3: Add default common flag**

Modify `common/constants.go` near the other boolean options:

```go
var LLMTraceEnabled = false
```

- [ ] **Step 4: Add option map wiring**

Modify `model/option.go`:

Add default in `InitOptionMap` near other `Enabled` options:

```go
		common.OptionMap["LLMTraceEnabled"] = strconv.FormatBool(common.LLMTraceEnabled)
```

Add a case in the `strings.HasSuffix(key, "Enabled")` switch:

```go
			case "LLMTraceEnabled":
				common.LLMTraceEnabled = boolValue
```

- [ ] **Step 5: Run option test**

Run:

```powershell
gofmt -w common\constants.go model\option.go model\option_trace_test.go
go test ./model -run TestLLMTraceOptionUpdatesCommonFlag -count=1
```

Expected: pass.

- [ ] **Step 6: Commit**

Run:

```powershell
git add common\constants.go model\option.go model\option_trace_test.go
git commit -m "feat: add llm trace option"
```

## Task 3: Add Service Trace Capture Helpers

**Files:**
- Create: `service/llm_trace.go`
- Create: `service/llm_trace_test.go`

- [ ] **Step 1: Write service helper tests**

Create `service/llm_trace_test.go`:

```go
package service

import (
	"NewAPI-Gateway/common"
	"NewAPI-Gateway/model"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
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
		AggToken:      &model.AggregatedToken{Id: 1, UserId: 2},
		Provider:      &model.Provider{Id: 3, Name: "openai"},
		Token:         &model.ProviderToken{Id: 4},
		Context:       ctx,
		RequestId:     "req-disabled",
		ModelName:     "gpt-4.1",
		Method:        "POST",
		Path:          "/v1/chat/completions",
		StatusCode:    200,
		RequestBody:   []byte(`{"messages":[]}`),
		ResponseBody:  []byte(`{"ok":true}`),
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
		AggToken:          &model.AggregatedToken{Id: 1, UserId: 2},
		Provider:          &model.Provider{Id: 3, Name: "openai"},
		Token:             &model.ProviderToken{Id: 4},
		Context:           ctx,
		RequestId:         "req-enabled",
		ModelName:         "gpt-4.1",
		Method:            "POST",
		Path:              "/v1/chat/completions",
		StatusCode:        200,
		RequestedStream:   false,
		ResponseIsStream:  false,
		RequestBody:       []byte(`{"model":"gpt-4.1"}`),
		ResponseBody:      []byte(`{"choices":[]}`),
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
```

Also add this import to the test file:

```go
import "net/http/httptest"
```

and this helper at the bottom:

```go
func httptestRequest(method string, path string, userAgent string) *http.Request {
	req := httptest.NewRequest(method, path, nil)
	req.Header.Set("User-Agent", userAgent)
	return req
}
```

Also add this import:

```go
import "net/http"
```

- [ ] **Step 2: Run tests to verify they fail**

Run:

```powershell
go test ./service -run "TestCaptureLLMTrace|TestTraceStreamCapture" -count=1
```

Expected: fail with undefined `llmTraceInput`, `captureLLMTrace`, and `newTraceStreamCapture`.

- [ ] **Step 3: Add helper implementation**

Create `service/llm_trace.go`:

```go
package service

import (
	"NewAPI-Gateway/common"
	"NewAPI-Gateway/model"
	"fmt"
	"strings"

	"github.com/gin-gonic/gin"
)

const maxTraceStreamCaptureBytes = 10 * 1024 * 1024

type llmTraceInput struct {
	AggToken         *model.AggregatedToken
	Provider         *model.Provider
	Token            *model.ProviderToken
	Context          *gin.Context
	RequestId        string
	ModelName        string
	Method           string
	Path             string
	StatusCode       int
	RequestedStream  bool
	ResponseIsStream bool
	RequestBody      []byte
	ResponseBody     []byte
	ErrorMessage     string
}

type traceStreamCapture struct {
	enabled bool
	builder strings.Builder
	limit   int
}

func newTraceStreamCapture() *traceStreamCapture {
	return &traceStreamCapture{
		enabled: common.LLMTraceEnabled,
		limit:   maxTraceStreamCaptureBytes,
	}
}

func (c *traceStreamCapture) appendLine(line string) {
	if c == nil || !c.enabled {
		return
	}
	remaining := c.limit - c.builder.Len()
	if remaining <= 0 {
		return
	}
	text := line + "\n"
	if len(text) > remaining {
		text = text[:remaining]
	}
	c.builder.WriteString(text)
}

func (c *traceStreamCapture) String() string {
	if c == nil || !c.enabled {
		return ""
	}
	return c.builder.String()
}

func captureLLMTrace(input llmTraceInput) {
	if !common.LLMTraceEnabled {
		return
	}
	if input.AggToken == nil || input.Provider == nil || input.Token == nil || input.Context == nil {
		return
	}

	trace := &model.LLMTrace{
		RequestId:         input.RequestId,
		UserId:            input.AggToken.UserId,
		AggregatedTokenId: input.AggToken.Id,
		ProviderId:        input.Provider.Id,
		ProviderName:      input.Provider.Name,
		ProviderTokenId:   input.Token.Id,
		ModelName:         input.ModelName,
		Method:            input.Method,
		Path:              input.Path,
		StatusCode:        input.StatusCode,
		RequestedStream:   input.RequestedStream,
		ResponseIsStream:  input.ResponseIsStream,
		RequestBody:       string(input.RequestBody),
		ResponseBody:      string(input.ResponseBody),
		ErrorMessage:      input.ErrorMessage,
		ClientIp:          input.Context.ClientIP(),
		UserAgent:         strings.TrimSpace(input.Context.GetHeader("User-Agent")),
	}
	if err := trace.Insert(); err != nil {
		common.SysLog(fmt.Sprintf("failed to insert llm trace: %v", err))
	}
}
```

- [ ] **Step 4: Fix test imports**

Ensure `service/llm_trace_test.go` has one import block:

```go
import (
	"NewAPI-Gateway/common"
	"NewAPI-Gateway/model"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)
```

- [ ] **Step 5: Run service helper tests**

Run:

```powershell
gofmt -w service\llm_trace.go service\llm_trace_test.go
go test ./service -run "TestCaptureLLMTrace|TestTraceStreamCapture" -count=1
```

Expected: pass.

- [ ] **Step 6: Commit**

Run:

```powershell
git add service\llm_trace.go service\llm_trace_test.go
git commit -m "feat: add llm trace capture helpers"
```

## Task 4: Capture Trace Data In Relay Proxy

**Files:**
- Modify: `service/proxy.go`
- Modify: `service/llm_trace_test.go`

- [ ] **Step 1: Add focused tests for request construction helpers**

Append to `service/llm_trace_test.go`:

```go
func TestCaptureLLMTraceStoresErrorMetadata(t *testing.T) {
	setupServiceTraceTestDB(t)
	common.LLMTraceEnabled = true
	defer func() { common.LLMTraceEnabled = false }()

	ctx, _ := gin.CreateTestContext(nil)
	ctx.Request = httptestRequest("POST", "/v1/messages", "agent")
	captureLLMTrace(llmTraceInput{
		AggToken:         &model.AggregatedToken{Id: 1, UserId: 2},
		Provider:         &model.Provider{Id: 3, Name: "anthropic"},
		Token:            &model.ProviderToken{Id: 4},
		Context:          ctx,
		RequestId:        "req-error",
		ModelName:        "claude-3-5",
		Method:           "POST",
		Path:             "/v1/messages",
		StatusCode:       429,
		RequestedStream:  true,
		ResponseIsStream: false,
		RequestBody:      []byte(`{"model":"claude-3-5"}`),
		ResponseBody:     []byte(`{"error":{"type":"rate_limit_error"}}`),
		ErrorMessage:     "upstream status 429",
	})

	var trace model.LLMTrace
	if err := model.DB.First(&trace, "request_id = ?", "req-error").Error; err != nil {
		t.Fatalf("find trace: %v", err)
	}
	if trace.StatusCode != 429 || trace.ErrorMessage != "upstream status 429" {
		t.Fatalf("unexpected error metadata: status=%d error=%q", trace.StatusCode, trace.ErrorMessage)
	}
	if !trace.RequestedStream || trace.ResponseIsStream {
		t.Fatalf("unexpected stream flags")
	}
}
```

- [ ] **Step 2: Run tests**

Run:

```powershell
gofmt -w service\llm_trace_test.go
go test ./service -run "TestCaptureLLMTrace|TestTraceStreamCapture" -count=1
```

Expected: pass. These tests protect the helper before proxy integration.

- [ ] **Step 3: Wire capture into transport error branch**

In `service/proxy.go`, inside `if err != nil` after `logUsage(...)`, add:

```go
		captureLLMTrace(llmTraceInput{
			AggToken:        aggToken,
			Provider:        provider,
			Token:           token,
			Context:         c,
			RequestId:       requestId,
			ModelName:       c.GetString("request_model"),
			Method:          c.Request.Method,
			Path:            c.Request.URL.Path,
			StatusCode:      http.StatusBadGateway,
			RequestedStream: requestedStream,
			RequestBody:     bodyBytes,
			ErrorMessage:    errorMsg,
		})
```

- [ ] **Step 4: Wire capture into upstream HTTP error branch**

In `service/proxy.go`, inside `if resp.StatusCode >= 400` after `logUsage(...)`, add:

```go
		captureLLMTrace(llmTraceInput{
			AggToken:         aggToken,
			Provider:         provider,
			Token:            token,
			Context:          c,
			RequestId:        requestId,
			ModelName:        usage.ModelName,
			Method:           c.Request.Method,
			Path:             c.Request.URL.Path,
			StatusCode:       resp.StatusCode,
			RequestedStream:  requestedStream,
			ResponseIsStream: responseIsStream,
			RequestBody:      bodyBytes,
			ResponseBody:     respBody,
			ErrorMessage:     errorMsg,
		})
```

- [ ] **Step 5: Wire capture into streaming branch**

In `service/proxy.go`, before the streaming `for scanner.Scan()` loop, add:

```go
		streamCapture := newTraceStreamCapture()
```

Inside the loop, immediately after `line := scanner.Text()`, add:

```go
			streamCapture.appendLine(line)
```

After `logUsage(...)` in the streaming branch, add:

```go
		captureLLMTrace(llmTraceInput{
			AggToken:         aggToken,
			Provider:         provider,
			Token:            token,
			Context:          c,
			RequestId:        requestId,
			ModelName:        streamUsage.ModelName,
			Method:           c.Request.Method,
			Path:             c.Request.URL.Path,
			StatusCode:       resp.StatusCode,
			RequestedStream:  requestedStream,
			ResponseIsStream: true,
			RequestBody:      bodyBytes,
			ResponseBody:     []byte(streamCapture.String()),
			ErrorMessage:     errorMsg,
		})
```

- [ ] **Step 6: Wire capture into response-read error branch**

In the non-streaming `readErr != nil` branch, after `logUsage(...)`, add:

```go
			captureLLMTrace(llmTraceInput{
				AggToken:        aggToken,
				Provider:        provider,
				Token:           token,
				Context:         c,
				RequestId:       requestId,
				ModelName:       c.GetString("request_model"),
				Method:          c.Request.Method,
				Path:            c.Request.URL.Path,
				StatusCode:      http.StatusBadGateway,
				RequestedStream: requestedStream,
				RequestBody:     bodyBytes,
				ErrorMessage:    errorMsg,
			})
```

- [ ] **Step 7: Wire capture into non-streaming success branch**

In the non-streaming success branch, after `logUsage(...)`, add:

```go
			captureLLMTrace(llmTraceInput{
				AggToken:         aggToken,
				Provider:         provider,
				Token:            token,
				Context:          c,
				RequestId:        requestId,
				ModelName:        usage.ModelName,
				Method:           c.Request.Method,
				Path:             c.Request.URL.Path,
				StatusCode:       resp.StatusCode,
				RequestedStream:  requestedStream,
				ResponseIsStream: false,
				RequestBody:      bodyBytes,
				ResponseBody:     respBody,
			})
```

- [ ] **Step 8: Format and run service tests**

Run:

```powershell
gofmt -w service\proxy.go service\llm_trace.go service\llm_trace_test.go
go test ./service -count=1
```

Expected: pass.

- [ ] **Step 9: Commit**

Run:

```powershell
git add service\proxy.go service\llm_trace.go service\llm_trace_test.go
git commit -m "feat: capture llm traces in relay"
```

## Task 5: Add Admin API For Trace Listing, Detail, And Clearing

**Files:**
- Create: `controller/llm_trace.go`
- Create: `controller/llm_trace_test.go`
- Modify: `router/api-router.go`

- [ ] **Step 1: Write controller tests**

Create `controller/llm_trace_test.go`:

```go
package controller

import (
	"NewAPI-Gateway/model"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
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
```

- [ ] **Step 2: Run tests to verify they fail**

Run:

```powershell
go test ./controller -run "TestGetLLMTrace|TestGetLLMTraces|TestDeleteLLMTraces" -count=1
```

Expected: fail with undefined `GetLLMTraces`, `GetLLMTrace`, and `DeleteLLMTraces`.

- [ ] **Step 3: Add controller handlers**

Create `controller/llm_trace.go`:

```go
package controller

import (
	"NewAPI-Gateway/common"
	"NewAPI-Gateway/model"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

func parseLLMTraceQuery(c *gin.Context) (int, int, model.LLMTraceQuery) {
	p, _ := strconv.Atoi(c.Query("p"))
	if p < 0 {
		p = 0
	}
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", strconv.Itoa(common.ItemsPerPage)))
	if pageSize <= 0 {
		pageSize = common.ItemsPerPage
	}
	query := model.LLMTraceQuery{
		Offset:       p * pageSize,
		Limit:        pageSize,
		Keyword:      strings.TrimSpace(c.Query("keyword")),
		ProviderName: strings.TrimSpace(c.Query("provider")),
		ModelName:    strings.TrimSpace(c.Query("model")),
		Status:       strings.TrimSpace(c.DefaultQuery("status", "all")),
	}
	return p, pageSize, query
}

func GetLLMTraces(c *gin.Context) {
	p, pageSize, query := parseLLMTraceQuery(c)
	traces, total, err := model.QueryLLMTraces(query)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"items":     traces,
			"total":     total,
			"page":      p,
			"page_size": pageSize,
		},
	})
}

func GetLLMTrace(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "invalid trace id"})
		return
	}
	trace, err := model.GetLLMTraceByID(id)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": trace})
}

func DeleteLLMTraces(c *gin.Context) {
	deleted, err := model.DeleteAllLLMTraces()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"deleted": deleted,
		},
	})
}
```

- [ ] **Step 4: Register routes**

Modify `router/api-router.go` after the admin log route:

```go
		llmTraceRoute := apiRouter.Group("/llm-trace")
		llmTraceRoute.Use(middleware.AdminAuth(), middleware.NoTokenAuth())
		{
			llmTraceRoute.GET("/", controller.GetLLMTraces)
			llmTraceRoute.GET("/:id", controller.GetLLMTrace)
			llmTraceRoute.DELETE("/", controller.DeleteLLMTraces)
		}
```

- [ ] **Step 5: Run controller and router tests**

Run:

```powershell
gofmt -w controller\llm_trace.go controller\llm_trace_test.go router\api-router.go
go test ./controller ./router -count=1
```

Expected: pass.

- [ ] **Step 6: Commit**

Run:

```powershell
git add controller\llm_trace.go controller\llm_trace_test.go router\api-router.go
git commit -m "feat: add llm trace admin api"
```

## Task 6: Add Settings Toggle In The Admin UI

**Files:**
- Modify: `web/src/components/SystemSetting.js`

- [ ] **Step 1: Add input state**

In `SystemSetting`, add `LLMTraceEnabled` to the initial `inputs` object:

```javascript
    LLMTraceEnabled: 'false',
```

- [ ] **Step 2: Add toggle handling**

In `updateOption`, add `LLMTraceEnabled` to the boolean switch:

```javascript
      case 'LLMTraceEnabled':
```

- [ ] **Step 3: Add checkbox UI**

In the `通用设置` card after proxy settings or before login/register settings, add:

```javascript
        <div style={{ borderTop: '1px solid var(--border-color)', margin: '1.5rem 0' }}></div>
        <h3 style={{ fontSize: '1.1rem', fontWeight: 'bold', marginBottom: '1rem' }}>审计设置</h3>
        <Checkbox
          checked={inputs.LLMTraceEnabled === 'true'}
          label='启用 LLM 上下文审计'
          name='LLMTraceEnabled'
          onChange={handleCheckboxChange}
        />
```

- [ ] **Step 4: Run frontend build check**

Run:

```powershell
cd web
npm run build
```

Expected: build succeeds and creates `web/build`.

- [ ] **Step 5: Commit**

Run from repo root:

```powershell
git add web\src\components\SystemSetting.js
git commit -m "feat: add llm trace setting toggle"
```

## Task 7: Add Trace List And Detail UI

**Files:**
- Create: `web/src/pages/LLMTrace/index.js`
- Modify: `web/src/App.js`
- Modify: `web/src/components/Layout.js`

- [ ] **Step 1: Create trace page**

Create `web/src/pages/LLMTrace/index.js`:

```javascript
import React, { useCallback, useEffect, useState } from 'react';
import { Eye, RefreshCw, Search, Trash2, X } from 'lucide-react';
import { API, showError, showSuccess } from '../../helpers';
import { ITEMS_PER_PAGE } from '../../constants';
import Badge from '../../components/ui/Badge';
import Button from '../../components/ui/Button';
import Card from '../../components/ui/Card';
import Input from '../../components/ui/Input';
import Modal from '../../components/ui/Modal';

const formatTime = (ts) => {
  if (!ts) {
    return '-';
  }
  return new Date(ts * 1000).toLocaleString();
};

const formatBody = (value) => {
  const text = String(value || '');
  if (!text.trim()) {
    return '-';
  }
  try {
    return JSON.stringify(JSON.parse(text), null, 2);
  } catch (e) {
    return text;
  }
};

const LLMTrace = () => {
  const [traces, setTraces] = useState([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(0);
  const [keyword, setKeyword] = useState('');
  const [status, setStatus] = useState('all');
  const [loading, setLoading] = useState(false);
  const [selectedTrace, setSelectedTrace] = useState(null);
  const [detailLoading, setDetailLoading] = useState(false);
  const [clearing, setClearing] = useState(false);

  const loadTraces = useCallback(async () => {
    setLoading(true);
    try {
      const params = new URLSearchParams();
      params.set('p', String(page));
      params.set('page_size', String(ITEMS_PER_PAGE));
      if (keyword.trim()) {
        params.set('keyword', keyword.trim());
      }
      if (status !== 'all') {
        params.set('status', status);
      }
      const res = await API.get(`/api/llm-trace/?${params.toString()}`);
      const { success, data, message } = res.data;
      if (!success) {
        showError(message);
        return;
      }
      setTraces(Array.isArray(data?.items) ? data.items : []);
      setTotal(Number(data?.total || 0));
    } catch (e) {
      showError('加载审计记录失败');
    } finally {
      setLoading(false);
    }
  }, [keyword, page, status]);

  useEffect(() => {
    setPage(0);
  }, [keyword, status]);

  useEffect(() => {
    loadTraces();
  }, [loadTraces]);

  const openTrace = async (trace) => {
    setDetailLoading(true);
    try {
      const res = await API.get(`/api/llm-trace/${trace.id}`);
      const { success, data, message } = res.data;
      if (!success) {
        showError(message);
        return;
      }
      setSelectedTrace(data);
    } catch (e) {
      showError('加载审计详情失败');
    } finally {
      setDetailLoading(false);
    }
  };

  const clearTraces = async () => {
    if (!window.confirm('确认清空全部 LLM 审计记录？该操作不会删除调用日志。')) {
      return;
    }
    setClearing(true);
    try {
      const res = await API.delete('/api/llm-trace/');
      const { success, data, message } = res.data;
      if (!success) {
        showError(message);
        return;
      }
      showSuccess(`已清空 ${Number(data?.deleted || 0)} 条审计记录`);
      setPage(0);
      await loadTraces();
    } catch (e) {
      showError('清空审计记录失败');
    } finally {
      setClearing(false);
    }
  };

  const canGoNext = (page + 1) * ITEMS_PER_PAGE < total;

  return (
    <>
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: '1rem', marginBottom: '1.5rem' }}>
        <h2 style={{ fontSize: '1.5rem', fontWeight: 'bold', margin: 0 }}>LLM 上下文审计</h2>
        <div style={{ display: 'flex', gap: '0.5rem' }}>
          <Button icon={RefreshCw} variant='secondary' onClick={loadTraces} disabled={loading}>刷新</Button>
          <Button icon={Trash2} variant='danger' onClick={clearTraces} disabled={clearing}>清空记录</Button>
        </div>
      </div>

      <Card padding='1rem'>
        <div style={{ display: 'flex', gap: '0.75rem', flexWrap: 'wrap', marginBottom: '1rem' }}>
          <Input
            icon={Search}
            aria-label='搜索 request id / model / provider / error'
            value={keyword}
            onChange={(e) => setKeyword(e.target.value)}
            style={{ marginBottom: 0, flex: 1, minWidth: '260px' }}
          />
          <select className='filter-select' value={status} onChange={(e) => setStatus(e.target.value)}>
            <option value='all'>全部状态</option>
            <option value='success'>成功</option>
            <option value='error'>失败</option>
          </select>
        </div>

        {loading ? (
          <div className='logs-empty'>加载中...</div>
        ) : traces.length === 0 ? (
          <div className='logs-empty'>没有审计记录</div>
        ) : (
          <div className='logs-card-list'>
            {traces.map((trace) => (
              <div key={trace.id} className='log-card'>
                <div className='log-card-top'>
                  <div className='log-card-main'>
                    <code className='log-model-code'>{trace.model_name || 'unknown-model'}</code>
                    <span className='log-provider'>@ {trace.provider_name || '-'}</span>
                  </div>
                  <div className='log-card-state'>
                    <Badge color={Number(trace.status_code) >= 400 || trace.error_message ? 'red' : 'green'}>
                      {Number(trace.status_code) || '-'}
                    </Badge>
                    <span className='log-time'>{formatTime(trace.created_at)}</span>
                  </div>
                </div>
                <div className='log-meta-inline'>
                  <div className='log-meta-pill'>
                    <span className='meta-pill-label'>Request ID</span>
                    <span className='meta-pill-value'>{trace.request_id || '-'}</span>
                  </div>
                  <div className='log-meta-pill'>
                    <span className='meta-pill-label'>路径</span>
                    <span className='meta-pill-value'>{trace.path || '-'}</span>
                  </div>
                  <div className='log-meta-pill'>
                    <span className='meta-pill-label'>请求</span>
                    <span className='meta-pill-value'>{trace.requested_stream ? '流式' : '非流式'}</span>
                  </div>
                  <div className='log-meta-pill'>
                    <span className='meta-pill-label'>响应</span>
                    <span className='meta-pill-value'>{trace.response_is_stream ? '流式' : '非流式'}</span>
                  </div>
                </div>
                <div className='log-card-actions'>
                  <Button size='sm' variant='secondary' icon={Eye} onClick={() => openTrace(trace)} disabled={detailLoading}>
                    查看上下文
                  </Button>
                </div>
              </div>
            ))}
          </div>
        )}

        <div className='logs-pagination'>
          <Button size='sm' variant='secondary' onClick={() => setPage((prev) => Math.max(prev - 1, 0))} disabled={loading || page === 0}>
            上一页
          </Button>
          <span className='logs-page-text'>第 {page + 1} 页 / 共 {total} 条</span>
          <Button size='sm' variant='secondary' onClick={() => setPage((prev) => prev + 1)} disabled={loading || !canGoNext}>
            下一页
          </Button>
        </div>
      </Card>

      {selectedTrace && (
        <Modal onClose={() => setSelectedTrace(null)} title='LLM 上下文详情'>
          <div style={{ display: 'flex', justifyContent: 'flex-end', marginBottom: '0.75rem' }}>
            <Button size='sm' variant='ghost' icon={X} onClick={() => setSelectedTrace(null)}>关闭</Button>
          </div>
          <div style={{ display: 'grid', gap: '1rem' }}>
            <section>
              <h3 style={{ fontSize: '1rem', fontWeight: 700, marginBottom: '0.5rem' }}>请求</h3>
              <pre className='log-json-detail'>{formatBody(selectedTrace.request_body)}</pre>
            </section>
            <section>
              <h3 style={{ fontSize: '1rem', fontWeight: 700, marginBottom: '0.5rem' }}>响应</h3>
              <pre className='log-json-detail'>{formatBody(selectedTrace.response_body)}</pre>
            </section>
            {selectedTrace.error_message && (
              <section>
                <h3 style={{ fontSize: '1rem', fontWeight: 700, marginBottom: '0.5rem' }}>错误</h3>
                <pre className='log-json-detail'>{selectedTrace.error_message}</pre>
              </section>
            )}
          </div>
        </Modal>
      )}
    </>
  );
};

export default LLMTrace;
```

- [ ] **Step 2: Register route in App**

Modify `web/src/App.js`:

Add lazy import:

```javascript
const LLMTrace = lazy(() => import('./pages/LLMTrace'));
```

Add authenticated route near `/log`:

```javascript
        <Route path='/llm-trace' element={<LLMTrace />} />
```

- [ ] **Step 3: Add navigation item**

Modify `web/src/components/Layout.js`:

Add `ClipboardList` to the lucide import:

```javascript
    ClipboardList,
```

Add nav item after logs:

```javascript
        { name: '审计', path: '/llm-trace', icon: ClipboardList, admin: true },
```

- [ ] **Step 4: Run frontend build**

Run:

```powershell
cd web
npm run build
```

Expected: build succeeds.

- [ ] **Step 5: Commit**

Run from repo root:

```powershell
git add web\src\pages\LLMTrace\index.js web\src\App.js web\src\components\Layout.js
git commit -m "feat: add llm trace admin ui"
```

## Task 8: Final Verification And Integration

**Files:**
- Verify only; no planned file edits.

- [ ] **Step 1: Run backend package tests**

Run:

```powershell
go test ./common ./model ./service ./controller ./router -count=1
```

Expected: pass.

- [ ] **Step 2: Run frontend build**

Run:

```powershell
cd web
npm run build
```

Expected: pass.

- [ ] **Step 3: Run full Go tests after frontend build**

Run from repo root after `web/build` exists:

```powershell
go test ./... -count=1
```

Expected: pass.

- [ ] **Step 4: Inspect git status**

Run:

```powershell
git status --short
```

Expected: no unstaged implementation changes. `web/build` may appear depending on `.gitignore`; do not commit generated build output unless the repository already tracks it.

- [ ] **Step 5: Manual smoke test**

Start the server as the project normally does, log in as an admin/root user, and verify:

1. Settings page shows the LLM context audit switch defaulted off.
2. With the switch off, a relay chat request creates a usage log but no LLM trace.
3. With the switch on, a non-streaming relay request creates a trace with request and response bodies.
4. With the switch on, a streaming relay request creates a trace with SSE response text.
5. An upstream error creates a trace containing the error response body.
6. The trace detail page shows full bodies only after opening one record.
7. The clear button removes trace records and leaves usage logs intact.

- [ ] **Step 6: Commit final verification notes if files changed**

If verification required code changes, commit them:

```powershell
git add <changed-files>
git commit -m "fix: stabilize llm trace verification"
```

If no files changed, do not create an empty commit.

## Self-Review

Spec coverage:

- Optional default-off switch: Task 2 and Task 6.
- Full LLM request/response capture: Task 3 and Task 4.
- Secret avoidance: capture reads bodies and metadata only, not auth headers; Task 3 tests guard against key-like body content introduced by capture code.
- Separate audit storage: Task 1.
- Link by `request_id`: Task 1 and Task 4.
- Admin delete history: Task 5 and Task 7.
- Streaming and non-streaming support: Task 4 and Task 7.
- Admin-only endpoints: Task 5 registers routes under `AdminAuth`.
- UI list/detail: Task 7.
- Verification: Task 8.

The scan for deferred-work markers is clean. Type names used across tasks are consistent: `LLMTrace`, `LLMTraceQuery`, `captureLLMTrace`, `llmTraceInput`, `traceStreamCapture`, and `LLMTraceEnabled`.
