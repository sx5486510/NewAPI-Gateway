# Route Group in Logs Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Persist and display the selected provider token group name on each usage log and LLM audit record.

**Architecture:** Store the route group at request time from the selected `ProviderToken.GroupName` into `usage_logs.token_group_name` and `llm_traces.token_group_name`. Return the new field through existing list/detail APIs and render it as a metadata pill on the logs and audit pages.

**Tech Stack:** Go, Gin, GORM, SQLite tests, React.

---

### Task 1: Backend Tests for Persisted Group Name

**Files:**
- Modify: `model/llm_trace_test.go`
- Modify: `service/llm_trace_test.go`

- [ ] **Step 1: Add failing model-level assertions**

In `model/llm_trace_test.go`, set `TokenGroupName: "vip"` in the `trace` literal inside `TestLLMTraceInsertAndQuery`, then assert the list and detail reads preserve it:

```go
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
```

Add after the request id assertion:

```go
if traces[0].TokenGroupName != "vip" {
	t.Fatalf("expected list token group vip, got %q", traces[0].TokenGroupName)
}
```

Add after `GetLLMTraceByID` succeeds:

```go
if fullTrace.TokenGroupName != "vip" {
	t.Fatalf("expected full token group vip, got %q", fullTrace.TokenGroupName)
}
```

- [ ] **Step 2: Add failing service-level assertion**

In `service/llm_trace_test.go`, change the token in `TestCaptureLLMTraceEnabledWritesTrace` to:

```go
Token: &model.ProviderToken{Id: 4, GroupName: "vip"},
```

Add after fetching `trace`:

```go
if trace.TokenGroupName != "vip" {
	t.Fatalf("expected token group vip, got %q", trace.TokenGroupName)
}
```

- [ ] **Step 3: Run tests to verify failure**

Run:

```bash
go test ./model ./service
```

Expected: fail because `TokenGroupName` does not exist on `LLMTrace`.

### Task 2: Backend Persistence

**Files:**
- Modify: `model/usage_log.go`
- Modify: `model/llm_trace.go`
- Modify: `service/llm_trace.go`

- [ ] **Step 1: Add model fields**

Add to `model.UsageLog` after `ProviderTokenId`:

```go
TokenGroupName string `json:"token_group_name" gorm:"type:varchar(64);index"`
```

Add to `model.LLMTrace` after `ProviderTokenId`:

```go
TokenGroupName string `json:"token_group_name" gorm:"type:varchar(64);index"`
```

- [ ] **Step 2: Persist usage log group name**

In `UsageLog.Insert`, add this entry after `provider_token_id`:

```go
"token_group_name": l.TokenGroupName,
```

In `service.logUsage`, add this field after `ProviderTokenId`:

```go
TokenGroupName: strings.TrimSpace(token.GroupName),
```

- [ ] **Step 3: Persist LLM trace group name**

In `service.captureLLMTrace`, add this field after `ProviderTokenId`:

```go
TokenGroupName: strings.TrimSpace(input.Token.GroupName),
```

In `model.QueryLLMTraces`, add `"token_group_name"` to the `Select(...)` field list after `"provider_token_id"`.

- [ ] **Step 4: Run backend tests**

Run:

```bash
go test ./model ./service
```

Expected: pass.

### Task 3: Frontend Display

**Files:**
- Modify: `web/src/components/LogsTable.js`
- Modify: `web/src/pages/LLMTrace/index.js`

- [ ] **Step 1: Display group on logs page**

In `web/src/components/LogsTable.js`, inside the metadata pill list near request ID, add:

```jsx
<div className='log-meta-pill'>
  <span className='meta-pill-label'>分组</span>
  <span className='meta-pill-value'>{log.token_group_name || '-'}</span>
</div>
```

- [ ] **Step 2: Display group on audit page**

In `web/src/pages/LLMTrace/index.js`, inside each trace card metadata section near request ID/path, add:

```jsx
<div className='log-meta-pill'>
  <span className='meta-pill-label'>分组</span>
  <span className='meta-pill-value'>{trace.token_group_name || '-'}</span>
</div>
```

- [ ] **Step 3: Run frontend build**

Run:

```bash
cd web
npm run build
```

Expected: build succeeds.

### Task 4: Final Verification

**Files:**
- Verify only.

- [ ] **Step 1: Run focused backend tests**

Run:

```bash
go test ./model ./service ./controller
```

Expected: pass.

- [ ] **Step 2: Review git diff**

Run:

```bash
git diff -- model/usage_log.go model/llm_trace.go service/llm_trace.go service/llm_trace_test.go model/llm_trace_test.go web/src/components/LogsTable.js web/src/pages/LLMTrace/index.js
```

Expected: diff only adds `token_group_name` persistence and display.
