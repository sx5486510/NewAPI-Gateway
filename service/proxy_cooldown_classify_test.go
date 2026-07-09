package service

import "testing"

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
