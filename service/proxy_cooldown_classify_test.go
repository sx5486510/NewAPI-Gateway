package service

import (
	"NewAPI-Gateway/common"
	"context"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestCooldownReasonCodeFor(t *testing.T) {
	cases := []struct {
		status int
		err    upstreamErrorInfo
		want   string
	}{
		{500, upstreamErrorInfo{}, common.CooldownReasonUpstream5xx},
		{408, upstreamErrorInfo{}, common.CooldownReasonUpstream408},
		{429, upstreamErrorInfo{Code: "rate_limit_exceeded"}, common.CooldownReasonRateLimited},
		{403, upstreamErrorInfo{Code: "permission_denied"}, common.CooldownReasonPermissionDenied},
		{404, upstreamErrorInfo{Message: "model not found"}, common.CooldownReasonModelNotFound},
		{400, upstreamErrorInfo{}, common.CooldownReasonUpstream4xx},
		{0, upstreamErrorInfo{}, common.CooldownReasonUnknown},
	}
	for _, tc := range cases {
		if got := cooldownReasonCodeFor(tc.status, tc.err); got != tc.want {
			t.Errorf("got %q want %q", got, tc.want)
		}
	}
}

func TestBuildCooldownCauseTruncatesUTF8Message(t *testing.T) {
	cause := buildCooldownCause(500, upstreamErrorInfo{Message: strings.Repeat("界", 300)}, "")
	if cause.ReasonCode != common.CooldownReasonUpstream5xx || cause.HTTPStatus != 500 || len([]rune(cause.ReasonMessage)) != common.CoolDownCauseMessageLimit()+1 {
		t.Fatalf("unexpected cause: %+v", cause)
	}
}

func TestShouldMarkUnsupportedModel_AcrossAll4xx(t *testing.T) {
	cases := []struct {
		name       string
		statusCode int
		err        upstreamErrorInfo
		want       bool
	}{
		{
			name:       "403 permission_denied code",
			statusCode: 403,
			err:        upstreamErrorInfo{Code: "permission_denied"},
			want:       true,
		},
		{
			name:       "401 model_not_found type",
			statusCode: 401,
			err:        upstreamErrorInfo{Type: "model_not_found"},
			want:       true,
		},
		{
			name:       "400 plain invalid request",
			statusCode: 400,
			err:        upstreamErrorInfo{Code: "invalid_request"},
			want:       false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := shouldMarkUnsupportedModel(tc.statusCode, tc.err)
			if got != tc.want {
				t.Fatalf("shouldMarkUnsupportedModel(%d, %+v) = %v, want %v", tc.statusCode, tc.err, got, tc.want)
			}
		})
	}
}

func TestShouldTriggerTokenCooldown_ForAll4xxExceptUnsupportedModel(t *testing.T) {
	cases := []struct {
		name       string
		statusCode int
		err        upstreamErrorInfo
		want       bool
	}{
		{
			name:       "400 invalid request still cools token",
			statusCode: 400,
			err:        upstreamErrorInfo{Code: "invalid_request"},
			want:       true,
		},
		{
			name:       "422 semantic error cools token",
			statusCode: 422,
			err:        upstreamErrorInfo{},
			want:       true,
		},
		{
			name:       "403 permission denied excluded by unsupported model",
			statusCode: 403,
			err:        upstreamErrorInfo{Code: "permission_denied"},
			want:       false,
		},
		{
			name:       "500 server error not token cooldown",
			statusCode: 500,
			err:        upstreamErrorInfo{},
			want:       false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := shouldTriggerTokenCooldown(tc.statusCode, tc.err)
			if got != tc.want {
				t.Fatalf("shouldTriggerTokenCooldown(%d, %+v) = %v, want %v", tc.statusCode, tc.err, got, tc.want)
			}
		})
	}
}

func TestShouldTriggerRouteCooldown(t *testing.T) {
	if !shouldTriggerRouteCooldown(500, upstreamErrorInfo{}) {
		t.Fatalf("expected 500 to trigger route cooldown")
	}
	if !shouldTriggerRouteCooldown(502, upstreamErrorInfo{}) {
		t.Fatalf("expected 502 to trigger route cooldown")
	}
	if !shouldTriggerRouteCooldown(408, upstreamErrorInfo{}) {
		t.Fatalf("expected 408 to trigger route cooldown")
	}
	if shouldTriggerRouteCooldown(400, upstreamErrorInfo{Code: "invalid_request"}) {
		t.Fatalf("expected 400 invalid_request not to trigger route cooldown")
	}
}

func TestParseRetryAfterSeconds(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want int
	}{
		{name: "empty", raw: "", want: 0},
		{name: "invalid", raw: "abc", want: 0},
		{name: "negative", raw: "-5", want: 0},
		{name: "seconds", raw: "120", want: 120},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := parseRetryAfterSeconds(tc.raw)
			if got != tc.want {
				t.Fatalf("parseRetryAfterSeconds(%q) = %d, want %d", tc.raw, got, tc.want)
			}
		})
	}
}

func TestIsNonRetryableInvalidRequest(t *testing.T) {
	if !isNonRetryableInvalidRequest(413, upstreamErrorInfo{}) {
		t.Fatalf("expected 413 to be non-retryable invalid request")
	}
	if !isNonRetryableInvalidRequest(422, upstreamErrorInfo{}) {
		t.Fatalf("expected 422 to be non-retryable invalid request")
	}
	if !isNonRetryableInvalidRequest(400, upstreamErrorInfo{Type: "invalid_request_error"}) {
		t.Fatalf("expected 400 invalid_request_error to be non-retryable")
	}
	if !isNonRetryableInvalidRequest(400, upstreamErrorInfo{Message: "invalid request: foo"}) {
		t.Fatalf("expected 400 invalid message to be non-retryable")
	}
	if isNonRetryableInvalidRequest(429, upstreamErrorInfo{Code: "rate_limit"}) {
		t.Fatalf("expected 429 rate_limit to be retryable")
	}
}

func TestExtractLLMResponseErrorMessage(t *testing.T) {
	cases := []struct {
		name string
		body string
		want string
	}{
		{
			name: "newapi wrapper failure",
			body: `{"success":false,"message":"upstream model overloaded","data":null}`,
			want: "upstream model overloaded",
		},
		{
			name: "openai error object",
			body: `{"error":{"message":"quota exceeded","type":"insufficient_quota","code":"insufficient_quota"}}`,
			want: "quota exceeded",
		},
		{
			name: "responses failed status",
			body: `{"id":"resp_1","status":"failed","error":{"message":"backend unavailable","code":"server_error"}}`,
			want: "backend unavailable",
		},
		{
			name: "nested response failed event",
			body: `{"type":"response.failed","response":{"id":"resp_1","status":"failed","output":[],"error":{"code":"rate_limit_exceeded","message":"Concurrency limit exceeded for account, please retry later"}}}`,
			want: "Concurrency limit exceeded for account, please retry later",
		},
		{
			name: "normal chat completion",
			body: `{"id":"chatcmpl_1","choices":[{"message":{"content":"ok"}}]}`,
			want: "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := extractLLMResponseErrorMessage([]byte(tc.body))
			if got != tc.want {
				t.Fatalf("extractLLMResponseErrorMessage() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestExtractSSELineErrorMessage(t *testing.T) {
	cases := []struct {
		name string
		line string
		want string
	}{
		{
			name: "codex rate limits event",
			line: "event: codex.rate_limits",
			want: "upstream rate limit event detected",
		},
		{
			name: "generic type error data",
			line: `data: {"type":"error","error":{"type":"rate_limit","message":"too many requests"}}`,
			want: "upstream SSE response error: too many requests",
		},
		{
			name: "responses failed data",
			line: `data: {"type":"response.failed","response":{"status":"failed","error":{"code":"rate_limit_exceeded","message":"Concurrency limit exceeded"}}}`,
			want: "upstream SSE response error: Concurrency limit exceeded",
		},
		{
			name: "normal ping",
			line: `data: {"type":"ping"}`,
			want: "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := extractSSELineErrorMessage(tc.line)
			if got != tc.want {
				t.Fatalf("extractSSELineErrorMessage() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestSelectProxyHTTPClientForStreamDoesNotUseShortTotalTimeout(t *testing.T) {
	if got := selectProxyHTTPClient(false); got != proxyHTTPClient {
		t.Fatalf("non-stream requests should use proxyHTTPClient")
	}
	if got := selectProxyHTTPClient(true); got == proxyHTTPClient {
		t.Fatalf("stream requests should use a dedicated client")
	}
	if got := selectProxyHTTPClient(true).Timeout; got != 0 {
		t.Fatalf("stream client Timeout = %v, want 0", got)
	}
	if got := selectProxyHTTPClient(false).Timeout; got != 5*time.Minute {
		t.Fatalf("non-stream client Timeout = %v, want %v", got, 5*time.Minute)
	}
}

func TestExtractSSELineCompletion(t *testing.T) {
	cases := []struct {
		name string
		line string
		want bool
	}{
		{name: "anthropic message stop event", line: "event: message_stop", want: true},
		{name: "openai done sentinel", line: "data: [DONE]", want: true},
		{name: "openai chat finish reason", line: `data: {"choices":[{"delta":{},"finish_reason":"stop"}]}`, want: true},
		{name: "responses completed data", line: `data: {"type":"response.completed","response":{"status":"completed"}}`, want: true},
		{name: "responses completed status", line: `data: {"id":"resp_1","status":"completed"}`, want: true},
		{name: "gemini candidate finish reason", line: `data: {"candidates":[{"content":{"parts":[{"text":"done"}]},"finishReason":"STOP"}]}`, want: true},
		{name: "thinking delta incomplete", line: `data: {"delta":{"thinking":"still working","type":"thinking_delta"},"type":"content_block_delta"}`, want: false},
		{name: "ping incomplete", line: `data: {"type":"ping"}`, want: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := extractSSELineCompletion(tc.line); got != tc.want {
				t.Fatalf("extractSSELineCompletion(%q) = %v, want %v", tc.line, got, tc.want)
			}
		})
	}
}

func TestFinalizeStreamErrorMarksEOFBeforeCompletion(t *testing.T) {
	if got := finalizeStreamError("", 12, true); got != "" {
		t.Fatalf("completed stream error = %q, want empty", got)
	}
	if got := finalizeStreamError("", 12, false); got != "" {
		t.Fatalf("clean EOF after events error = %q, want empty", got)
	}
	if got := finalizeStreamError("upstream SSE response error: bad", 12, false); got != "upstream SSE response error: bad" {
		t.Fatalf("existing error = %q, want existing error unchanged", got)
	}
	if got := finalizeStreamError("", 0, false); got != "stream ended without receiving any events" {
		t.Fatalf("empty stream error = %q, want stream ended without receiving any events", got)
	}
}

func TestNewStreamIdleTimerDoesNotRunBeforeFirstReset(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var reason atomic.Value
	timer := newStreamIdleTimer(10*time.Millisecond, cancel, &reason)
	defer timer.Stop()

	time.Sleep(30 * time.Millisecond)
	if ctx.Err() != nil {
		t.Fatalf("idle timer canceled before the stream body became active")
	}
	if got := loadStreamTimeoutReason(&reason); got != "" {
		t.Fatalf("idle timeout reason before first reset = %q, want empty", got)
	}

	timer.Reset(10 * time.Millisecond)
	select {
	case <-ctx.Done():
	case <-time.After(100 * time.Millisecond):
		t.Fatalf("idle timer did not cancel after being reset")
	}
	if got := loadStreamTimeoutReason(&reason); got != "stream idle timeout" {
		t.Fatalf("idle timeout reason after reset = %q, want stream idle timeout", got)
	}
}

func TestStreamTimeoutDefaultsAreLongerThanShortProxyTimeout(t *testing.T) {
	if streamMaxDuration <= proxyHTTPClient.Timeout {
		t.Fatalf("streamMaxDuration = %v, want greater than %v", streamMaxDuration, proxyHTTPClient.Timeout)
	}
	if streamIdleTimeout <= 0 {
		t.Fatalf("streamIdleTimeout = %v, want positive timeout", streamIdleTimeout)
	}
	if _, ok := streamProxyHTTPClient.Transport.(*http.Transport); !ok {
		t.Fatalf("stream client should use an http.Transport")
	}
}

func TestStreamRouteOutcomeSkipsClientCanceled(t *testing.T) {
	if got := streamRouteOutcome("", false, false); got != streamRouteOutcomeSuccess {
		t.Fatalf("completed stream outcome = %v, want success", got)
	}
	if got := streamRouteOutcome("stream ended before completion", false, false); got != streamRouteOutcomeFailure {
		t.Fatalf("failed stream outcome = %v, want failure", got)
	}
	if got := streamRouteOutcome("client canceled", true, false); got != streamRouteOutcomeSkip {
		t.Fatalf("client canceled stream outcome = %v, want skip", got)
	}
}

func TestStreamRouteOutcomeFailureTakesPrecedenceOverClientCanceled(t *testing.T) {
	if got := streamRouteOutcome("upstream SSE response error: bad", true, false); got != streamRouteOutcomeFailure {
		t.Fatalf("upstream error with client cancel outcome = %v, want failure", got)
	}
}

func TestStreamRouteOutcomeCompletedTakesPrecedenceOverClientCanceled(t *testing.T) {
	if got := streamRouteOutcome("client canceled", true, true); got != streamRouteOutcomeSuccess {
		t.Fatalf("completed stream with client cancel outcome = %v, want success", got)
	}
}

func TestClassifyProxyRequestErrorTreatsProxyStreamTimeoutAsRouteFailure(t *testing.T) {
	outcome := classifyProxyRequestError(context.Canceled, nil, "stream idle timeout")

	if outcome.StatusCode != http.StatusBadGateway {
		t.Fatalf("status code = %d, want %d", outcome.StatusCode, http.StatusBadGateway)
	}
	if !outcome.Retryable {
		t.Fatalf("retryable = false, want true")
	}
	if !outcome.RecordRouteFailure {
		t.Fatalf("record route failure = false, want true")
	}
}
