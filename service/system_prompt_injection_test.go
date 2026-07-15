package service

import (
	"encoding/json"
	"errors"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"NewAPI-Gateway/model"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestRouteSystemPromptInactiveReturnsOriginalBody(t *testing.T) {
	body := []byte(`{"model":"gpt-4","messages":"not parsed"}`)
	for _, tc := range []struct {
		name, method, path, content string
	}{
		{name: "other method", method: http.MethodGet, path: "/v1/chat/completions", content: "preset"},
		{name: "other path", method: http.MethodPost, path: "/v1/responses", content: "preset"},
		{name: "non-exact path", method: http.MethodPost, path: "/v1/chat/completions/", content: "preset"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := injectRouteSystemPrompt(tc.method, tc.path, body, tc.content)
			if err != nil {
				t.Fatalf("injectRouteSystemPrompt: %v", err)
			}
			if string(got) != string(body) {
				t.Fatalf("body changed: got %s want %s", got, body)
			}
		})
	}
}

func TestRouteSystemPromptBoundEmptyContentInjectsEmptySystemMessage(t *testing.T) {
	got, err := injectRouteSystemPrompt(http.MethodPost, "/v1/chat/completions", []byte(`{"model":"gpt-4","messages":[{"role":"user","content":"hello"}]}`), "")
	if err != nil {
		t.Fatalf("injectRouteSystemPrompt: %v", err)
	}
	assertRoutePromptMessages(t, got, "", "hello")
}

func TestRouteSystemPromptStreamingRequestIsInjected(t *testing.T) {
	got, err := injectRouteSystemPrompt(http.MethodPost, "/v1/chat/completions", []byte(`{"model":"gpt-4","stream":true,"messages":[]}`), "preset")
	if err != nil {
		t.Fatalf("injectRouteSystemPrompt: %v", err)
	}
	var request struct {
		Stream   bool                `json:"stream"`
		Messages []map[string]string `json:"messages"`
	}
	if err := json.Unmarshal(got, &request); err != nil {
		t.Fatal(err)
	}
	if !request.Stream || !reflect.DeepEqual(request.Messages, []map[string]string{{"role": "system", "content": "preset"}}) {
		t.Fatalf("unexpected streaming request: %s", got)
	}
}

func TestRouteSystemPromptRetryUsesOriginalBodyPerRoute(t *testing.T) {
	setupRouteSystemPromptDB(t)
	first := model.SystemPrompt{Name: "first", ModelName: "gpt-4", Content: "first preset"}
	second := model.SystemPrompt{Name: "second", ModelName: "gpt-4", Content: "second preset"}
	if err := model.DB.Create(&first).Error; err != nil {
		t.Fatal(err)
	}
	if err := model.DB.Create(&second).Error; err != nil {
		t.Fatal(err)
	}
	original := []byte(`{"model":"client-model","messages":[{"role":"user","content":"hello"}]}`)

	firstBody, err := prepareRouteRequestBody(http.MethodPost, "/v1/chat/completions", original, model.ModelRoute{ModelName: "gpt-4", SystemPromptId: &first.Id})
	if err != nil {
		t.Fatalf("prepare first route: %v", err)
	}
	secondBody, err := prepareRouteRequestBody(http.MethodPost, "/v1/chat/completions", original, model.ModelRoute{ModelName: "gpt-4", SystemPromptId: &second.Id})
	if err != nil {
		t.Fatalf("prepare second route: %v", err)
	}
	if string(original) != `{"model":"client-model","messages":[{"role":"user","content":"hello"}]}` {
		t.Fatalf("original body mutated: %s", original)
	}
	assertRoutePromptMessages(t, firstBody, "first preset", "hello")
	assertRoutePromptMessages(t, secondBody, "second preset", "hello")
	if string(secondBody) == string(firstBody) {
		t.Fatal("route-specific bodies unexpectedly equal")
	}
}

func TestRouteSystemPromptNoBindingDoesNotParseMessages(t *testing.T) {
	model.DB = nil
	body := []byte(`{"model":"old","messages":"invalid"}`)
	got, err := prepareRouteRequestBody(http.MethodPost, "/v1/chat/completions", body, model.ModelRoute{ModelName: "new"})
	if err != nil {
		t.Fatalf("prepareRouteRequestBody: %v", err)
	}
	var decoded map[string]json.RawMessage
	if err := json.Unmarshal(got, &decoded); err != nil {
		t.Fatal(err)
	}
	if string(decoded["model"]) != `"new"` || string(decoded["messages"]) != `"invalid"` {
		t.Fatalf("unexpected body: %s", got)
	}
}

func TestRouteSystemPromptOtherEndpointDoesNotResolveBinding(t *testing.T) {
	model.DB = nil
	missingID := 99
	body := []byte(`{"model":"old","input":"hello"}`)
	got, err := prepareRouteRequestBody(http.MethodPost, "/v1/responses", body, model.ModelRoute{ModelName: "new", SystemPromptId: &missingID})
	if err != nil {
		t.Fatalf("prepareRouteRequestBody: %v", err)
	}
	if !strings.Contains(string(got), `"model":"new"`) || !strings.Contains(string(got), `"input":"hello"`) {
		t.Fatalf("unexpected body: %s", got)
	}
}

func TestRouteSystemPromptConfiguredPresetMustExistAndMatch(t *testing.T) {
	setupRouteSystemPromptDB(t)
	wrong := model.SystemPrompt{Name: "wrong", ModelName: "other", Content: "preset"}
	if err := model.DB.Create(&wrong).Error; err != nil {
		t.Fatal(err)
	}
	missingID := wrong.Id + 100
	for _, route := range []model.ModelRoute{
		{Id: 1, ModelName: "gpt-4", SystemPromptId: &missingID},
		{Id: 2, ModelName: "gpt-4", SystemPromptId: &wrong.Id},
	} {
		_, err := prepareRouteRequestBody(http.MethodPost, "/v1/chat/completions", []byte(`{"messages":[]}`), route)
		var unavailable *RouteSystemPromptUnavailableError
		if !errors.As(err, &unavailable) {
			t.Fatalf("route %d error = %T %v, want unavailable", route.Id, err, err)
		}
	}
}

func setupRouteSystemPromptDB(t *testing.T) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	model.DB = db
	if err := db.AutoMigrate(&model.SystemPrompt{}); err != nil {
		t.Fatal(err)
	}
}

func assertRoutePromptMessages(t *testing.T, body []byte, wantPrompt, wantUser string) {
	t.Helper()
	var decoded struct {
		Model    string `json:"model"`
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Model != "gpt-4" || len(decoded.Messages) != 2 || decoded.Messages[0].Content != wantPrompt || decoded.Messages[1].Content != wantUser {
		t.Fatalf("unexpected prepared body: %s", body)
	}
}

func TestRouteSystemPromptPrependsMessageAndPreservesRequest(t *testing.T) {
	content := "first line\nquoted: \"你好\""
	body := []byte(`{"model":"gpt-4","temperature":0.25,"metadata":{"key":"value"},"messages":[{"role":"system","content":"client system"},{"role":"user","content":"hello"}]}`)

	got, err := injectRouteSystemPrompt(http.MethodPost, "/v1/chat/completions", body, content)
	if err != nil {
		t.Fatalf("injectRouteSystemPrompt: %v", err)
	}
	var decoded map[string]json.RawMessage
	if err := json.Unmarshal(got, &decoded); err != nil {
		t.Fatalf("invalid output JSON: %v", err)
	}
	var messages []map[string]any
	if err := json.Unmarshal(decoded["messages"], &messages); err != nil {
		t.Fatalf("decode messages: %v", err)
	}
	want := []map[string]any{
		{"role": "system", "content": content},
		{"role": "system", "content": "client system"},
		{"role": "user", "content": "hello"},
	}
	if !reflect.DeepEqual(messages, want) {
		t.Fatalf("messages = %#v, want %#v", messages, want)
	}
	if string(decoded["temperature"]) != "0.25" || string(decoded["metadata"]) != `{"key":"value"}` || string(decoded["model"]) != `"gpt-4"` {
		t.Fatalf("top-level fields not preserved: %s", got)
	}
}

func TestRouteSystemPromptInsertsIntoEmptyMessages(t *testing.T) {
	got, err := injectRouteSystemPrompt(http.MethodPost, "/v1/chat/completions", []byte(`{"messages":[]}`), "preset")
	if err != nil {
		t.Fatalf("injectRouteSystemPrompt: %v", err)
	}
	var decoded struct {
		Messages []map[string]string `json:"messages"`
	}
	if err := json.Unmarshal(got, &decoded); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	want := []map[string]string{{"role": "system", "content": "preset"}}
	if !reflect.DeepEqual(decoded.Messages, want) {
		t.Fatalf("messages = %#v, want %#v", decoded.Messages, want)
	}
}

func TestRouteSystemPromptRejectsInvalidMessages(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
	}{
		{name: "missing", body: `{"model":"gpt-4"}`},
		{name: "non-array", body: `{"messages":{}}`},
		{name: "invalid JSON", body: `{"messages":`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := injectRouteSystemPrompt(http.MethodPost, "/v1/chat/completions", []byte(tc.body), "preset")
			if err == nil {
				t.Fatal("expected error")
			}
			var invalid *RouteSystemPromptInvalidRequestError
			if !errors.As(err, &invalid) {
				t.Fatalf("error type = %T, want *RouteSystemPromptInvalidRequestError", err)
			}
			if invalid.Error() != "invalid messages in chat completions request" {
				t.Fatalf("error = %q", invalid.Error())
			}
		})
	}
}
