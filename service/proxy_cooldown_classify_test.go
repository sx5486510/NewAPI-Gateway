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

