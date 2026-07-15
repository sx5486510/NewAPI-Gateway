package service

import (
	"NewAPI-Gateway/model"
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
)

const routeSystemPromptInvalidRequestMessage = "invalid messages in chat completions request"

type RouteSystemPromptInvalidRequestError struct{}

func (*RouteSystemPromptInvalidRequestError) Error() string {
	return routeSystemPromptInvalidRequestMessage
}

type RouteSystemPromptUnavailableError struct {
	RouteID int
	Cause   error
}

func (e *RouteSystemPromptUnavailableError) Error() string {
	return fmt.Sprintf("system prompt unavailable for route %d: %v", e.RouteID, e.Cause)
}

func (e *RouteSystemPromptUnavailableError) Unwrap() error {
	return e.Cause
}

func prepareRouteRequestBody(method, path string, original []byte, route model.ModelRoute) ([]byte, error) {
	body := rewriteRequestModel(original, route.ModelName)
	if route.SystemPromptId == nil || method != http.MethodPost || path != "/v1/chat/completions" {
		return body, nil
	}
	prompt, err := model.GetSystemPromptByID(*route.SystemPromptId)
	if err != nil {
		return nil, &RouteSystemPromptUnavailableError{RouteID: route.Id, Cause: err}
	}
	if prompt.ModelName != route.ModelName {
		return nil, &RouteSystemPromptUnavailableError{RouteID: route.Id, Cause: model.ErrSystemPromptModelMismatch}
	}
	return injectRouteSystemPrompt(method, path, body, prompt.Content)
}

func injectRouteSystemPrompt(method, path string, body []byte, content string) ([]byte, error) {
	if content == "" || method != http.MethodPost || path != "/v1/chat/completions" {
		return body, nil
	}

	var request map[string]json.RawMessage
	if err := json.Unmarshal(body, &request); err != nil {
		return nil, &RouteSystemPromptInvalidRequestError{}
	}
	encodedMessages, ok := request["messages"]
	if !ok || !bytes.HasPrefix(bytes.TrimSpace(encodedMessages), []byte("[")) {
		return nil, &RouteSystemPromptInvalidRequestError{}
	}
	var messages []json.RawMessage
	if err := json.Unmarshal(encodedMessages, &messages); err != nil {
		return nil, &RouteSystemPromptInvalidRequestError{}
	}
	gatewayMessage, err := json.Marshal(map[string]string{"role": "system", "content": content})
	if err != nil {
		return nil, err
	}
	messages = append([]json.RawMessage{gatewayMessage}, messages...)
	request["messages"], err = json.Marshal(messages)
	if err != nil {
		return nil, err
	}
	return json.Marshal(request)
}
