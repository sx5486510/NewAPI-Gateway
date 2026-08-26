package service

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"NewAPI-Gateway/model"

	"github.com/gin-gonic/gin"
)

func newMessagesProxyContext(body string) (*gin.Context, *httptest.ResponseRecorder) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Request.Header.Set("x-api-key", "client-anthropic-key")
	c.Request.Header.Set("anthropic-version", "2023-06-01")
	c.Set("agg_token", &model.AggregatedToken{Id: 1, UserId: 1})
	c.Set("request_model", "claude-3")
	c.Set("request_model_resolved", "claude-3")
	return c, recorder
}

func TestProxyConvertMessagesRequestHeadersAndURL(t *testing.T) {
	setupRouteSystemPromptProxyTest(t)
	var gotPath string
	var gotAuth string
	var gotAPIKey string
	var gotAnthropicVersion string
	var gotBody []byte
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		gotAPIKey = r.Header.Get("x-api-key")
		gotAnthropicVersion = r.Header.Get("anthropic-version")
		gotBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"chatcmpl-1","model":"gpt-4","choices":[{"finish_reason":"stop","message":{"content":"hi"}}],"usage":{"prompt_tokens":1,"completion_tokens":1}}`))
	}))
	defer upstream.Close()

	c, recorder := newMessagesProxyContext(`{"model":"claude-3","max_tokens":100,"messages":[{"role":"user","content":"hello"}]}`)
	route := model.ModelRoute{Id: 1, ModelName: "gpt-4", ConvertMessagesToChat: true}
	token := &model.ProviderToken{Id: 201, SkKey: "upstream-sk-key"}
	provider := &model.Provider{Id: 21, Name: "openai", BaseURL: upstream.URL}

	if err := ProxyToUpstream(c, route, token, provider); err != nil {
		t.Fatalf("ProxyToUpstream: %v", err)
	}
	if gotPath != "/v1/chat/completions" {
		t.Fatalf("expected upstream path /v1/chat/completions, got %s", gotPath)
	}
	if gotAuth != "Bearer upstream-sk-key" {
		t.Fatalf("expected Bearer auth, got %s", gotAuth)
	}
	if gotAPIKey != "" {
		t.Fatalf("expected no x-api-key header, got %s", gotAPIKey)
	}
	if gotAnthropicVersion != "" {
		t.Fatalf("expected no anthropic-version header, got %s", gotAnthropicVersion)
	}
	var upstreamReq map[string]interface{}
	if err := json.Unmarshal(gotBody, &upstreamReq); err != nil {
		t.Fatalf("upstream body not JSON: %v", err)
	}
	if upstreamReq["model"] != "gpt-4" {
		t.Fatalf("expected model rewritten to route model, got %#v", upstreamReq["model"])
	}
	if _, ok := upstreamReq["messages"]; !ok {
		t.Fatalf("expected chat-shaped messages field: %#v", upstreamReq)
	}

	if recorder.Code != http.StatusOK {
		t.Fatalf("client status = %d, body %s", recorder.Code, recorder.Body.String())
	}
	var clientResp map[string]interface{}
	if err := json.Unmarshal(recorder.Body.Bytes(), &clientResp); err != nil {
		t.Fatalf("client body not JSON: %v", err)
	}
	if clientResp["type"] != "message" {
		t.Fatalf("expected Anthropic message envelope, got %#v", clientResp)
	}
}

func TestProxyConvertMessagesDisabledStaysTransparent(t *testing.T) {
	setupRouteSystemPromptProxyTest(t)
	var gotPath string
	var gotAPIKey string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAPIKey = r.Header.Get("x-api-key")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"msg_1","type":"message","role":"assistant","content":[{"type":"text","text":"hi"}]}`))
	}))
	defer upstream.Close()

	c, recorder := newMessagesProxyContext(`{"model":"claude-3","max_tokens":100,"messages":[{"role":"user","content":"hello"}]}`)
	route := model.ModelRoute{Id: 1, ModelName: "claude-3", ConvertMessagesToChat: false}
	token := &model.ProviderToken{Id: 202, SkKey: "upstream-sk-key"}
	provider := &model.Provider{Id: 22, Name: "anthropic", BaseURL: upstream.URL}

	if err := ProxyToUpstream(c, route, token, provider); err != nil {
		t.Fatalf("ProxyToUpstream: %v", err)
	}
	if gotPath != "/v1/messages" {
		t.Fatalf("expected transparent path /v1/messages, got %s", gotPath)
	}
	if gotAPIKey != "upstream-sk-key" {
		t.Fatalf("expected x-api-key forwarded, got %s", gotAPIKey)
	}
	if recorder.Code != http.StatusOK {
		t.Fatalf("client status = %d", recorder.Code)
	}
	if !strings.Contains(recorder.Body.String(), `"id":"msg_1"`) {
		t.Fatalf("expected untouched Anthropic body, got %s", recorder.Body.String())
	}
}

func TestProxyConvertMessagesUpstream4xxReturnsAnthropicError(t *testing.T) {
	setupRouteSystemPromptProxyTest(t)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"type":"invalid_request_error","message":"bad request"}}`))
	}))
	defer upstream.Close()

	c, _ := newMessagesProxyContext(`{"model":"claude-3","max_tokens":100,"messages":[{"role":"user","content":"hello"}]}`)
	route := model.ModelRoute{Id: 1, ModelName: "gpt-4", ConvertMessagesToChat: true}
	token := &model.ProviderToken{Id: 203, SkKey: "upstream-sk-key"}
	provider := &model.Provider{Id: 23, Name: "openai", BaseURL: upstream.URL}

	err := ProxyToUpstream(c, route, token, provider)
	if err == nil {
		t.Fatal("expected non-nil ProxyAttemptError")
	}
	if err.Retryable {
		t.Fatalf("expected non-retryable 400 error, got %#v", err)
	}
	var body map[string]interface{}
	if unmarshalErr := json.Unmarshal(err.UpstreamBody, &body); unmarshalErr != nil {
		t.Fatalf("upstream body not JSON: %v", unmarshalErr)
	}
	if body["type"] != "error" {
		t.Fatalf("expected Anthropic error envelope, got %#v", body)
	}
	errObj := body["error"].(map[string]interface{})
	if errObj["type"] != "invalid_request_error" {
		t.Fatalf("expected invalid_request_error, got %#v", errObj)
	}
}

func TestProxyConvertMessagesUpstream5xxIsRetryableAndDoesNotLeakOpenAIShape(t *testing.T) {
	setupRouteSystemPromptProxyTest(t)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":{"type":"server_error","message":"boom"}}`))
	}))
	defer upstream.Close()

	c, _ := newMessagesProxyContext(`{"model":"claude-3","max_tokens":100,"messages":[{"role":"user","content":"hello"}]}`)
	route := model.ModelRoute{Id: 1, ModelName: "gpt-4", ConvertMessagesToChat: true}
	token := &model.ProviderToken{Id: 204, SkKey: "upstream-sk-key"}
	provider := &model.Provider{Id: 24, Name: "openai", BaseURL: upstream.URL}

	err := ProxyToUpstream(c, route, token, provider)
	if err == nil {
		t.Fatal("expected non-nil ProxyAttemptError")
	}
	if !err.Retryable {
		t.Fatalf("expected retryable 500 error, got %#v", err)
	}
}

func TestProxyConvertMessagesRequestInvalidBlockReturns400(t *testing.T) {
	setupRouteSystemPromptProxyTest(t)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("upstream should not be called for a request that fails conversion")
	}))
	defer upstream.Close()

	c, _ := newMessagesProxyContext(`{"model":"claude-3","messages":[{"role":"user","content":[{"type":"mystery"}]}]}`)
	route := model.ModelRoute{Id: 1, ModelName: "gpt-4", ConvertMessagesToChat: true}
	token := &model.ProviderToken{Id: 205, SkKey: "upstream-sk-key"}
	provider := &model.Provider{Id: 25, Name: "openai", BaseURL: upstream.URL}

	err := ProxyToUpstream(c, route, token, provider)
	if err == nil {
		t.Fatal("expected conversion failure error")
	}
	if err.Retryable {
		t.Fatalf("expected non-retryable error, got %#v", err)
	}
	if err.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", err.StatusCode)
	}
}

func TestProxyConvertMessagesStreamingFullLifecycle(t *testing.T) {
	setupRouteSystemPromptProxyTest(t)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher := w.(http.Flusher)
		chunks := []string{
			`data: {"id":"chatcmpl-1","model":"gpt-4","choices":[{"delta":{"role":"assistant"}}]}`,
			`data: {"id":"chatcmpl-1","model":"gpt-4","choices":[{"delta":{"content":"Hello"},"finish_reason":"stop"}],"usage":{"prompt_tokens":3,"completion_tokens":1}}`,
			`data: [DONE]`,
		}
		for _, chunk := range chunks {
			_, _ = w.Write([]byte(chunk + "\n\n"))
			flusher.Flush()
		}
	}))
	defer upstream.Close()

	c, recorder := newMessagesProxyContext(`{"model":"claude-3","max_tokens":100,"stream":true,"messages":[{"role":"user","content":"hello"}]}`)
	route := model.ModelRoute{Id: 1, ModelName: "gpt-4", ConvertMessagesToChat: true}
	token := &model.ProviderToken{Id: 206, SkKey: "upstream-sk-key"}
	provider := &model.Provider{Id: 26, Name: "openai", BaseURL: upstream.URL}

	if err := ProxyToUpstream(c, route, token, provider); err != nil {
		t.Fatalf("ProxyToUpstream: %v", err)
	}

	body := recorder.Body.String()
	wantEvents := []string{"message_start", "content_block_start", "content_block_delta", "content_block_stop", "message_delta", "message_stop"}
	for _, event := range wantEvents {
		if !strings.Contains(body, "event: "+event) {
			t.Fatalf("expected event %s in output, got: %s", event, body)
		}
	}
	if strings.Contains(body, `"object":"chat.completion.chunk"`) {
		t.Fatalf("must not leak raw OpenAI SSE shape: %s", body)
	}
}

func TestProxyConvertMessagesStreamingMidStreamErrorDoesNotRetry(t *testing.T) {
	setupRouteSystemPromptProxyTest(t)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher := w.(http.Flusher)
		chunks := []string{
			`data: {"id":"chatcmpl-1","model":"gpt-4","choices":[{"delta":{"content":"Hello"}}]}`,
			`data: {"error":{"message":"mid-stream boom"}}`,
		}
		for _, chunk := range chunks {
			_, _ = w.Write([]byte(chunk + "\n\n"))
			flusher.Flush()
		}
	}))
	defer upstream.Close()

	c, recorder := newMessagesProxyContext(`{"model":"claude-3","max_tokens":100,"stream":true,"messages":[{"role":"user","content":"hello"}]}`)
	route := model.ModelRoute{Id: 1, ModelName: "gpt-4", ConvertMessagesToChat: true}
	token := &model.ProviderToken{Id: 207, SkKey: "upstream-sk-key"}
	provider := &model.Provider{Id: 27, Name: "openai", BaseURL: upstream.URL}

	err := ProxyToUpstream(c, route, token, provider)
	if err != nil {
		t.Fatalf("expected nil error (stream cannot be retried after first event), got %#v", err)
	}
	body := recorder.Body.String()
	if !strings.Contains(body, "event: message_start") {
		t.Fatalf("expected message_start already sent: %s", body)
	}
	if !strings.Contains(body, "event: error") {
		t.Fatalf("expected terminal error event, got: %s", body)
	}
}

func TestProxyConvertMessagesStreamingErrorBeforeFirstEventIsRetryable(t *testing.T) {
	setupRouteSystemPromptProxyTest(t)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher := w.(http.Flusher)
		_, _ = w.Write([]byte(`data: {"error":{"message":"boom before first event"}}` + "\n\n"))
		flusher.Flush()
	}))
	defer upstream.Close()

	c, _ := newMessagesProxyContext(`{"model":"claude-3","max_tokens":100,"stream":true,"messages":[{"role":"user","content":"hello"}]}`)
	route := model.ModelRoute{Id: 1, ModelName: "gpt-4", ConvertMessagesToChat: true}
	token := &model.ProviderToken{Id: 208, SkKey: "upstream-sk-key"}
	provider := &model.Provider{Id: 28, Name: "openai", BaseURL: upstream.URL}

	err := ProxyToUpstream(c, route, token, provider)
	if err == nil {
		t.Fatal("expected retryable error before first event was emitted")
	}
	if !err.Retryable {
		t.Fatalf("expected retryable error, got %#v", err)
	}
}
