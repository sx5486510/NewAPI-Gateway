package service

import (
	"NewAPI-Gateway/model"
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
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
	if method != http.MethodPost || path != "/v1/chat/completions" {
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
	// The route preset replaces every client-provided system message, leaving a
	// single authoritative system message for the upstream. Non-system messages
	// keep their original relative order; non-object entries pass through.
	messages = replaceClientSystemMessages(messages, gatewayMessage)
	request["messages"], err = json.Marshal(messages)
	if err != nil {
		return nil, err
	}
	return json.Marshal(request)
}

// replaceClientSystemMessages returns the preset followed by all client
// messages whose role is not "system". A message that cannot be decoded as an
// object is kept unchanged rather than silently dropped.
func replaceClientSystemMessages(messages []json.RawMessage, preset json.RawMessage) []json.RawMessage {
	result := make([]json.RawMessage, 0, len(messages)+1)
	result = append(result, preset)
	for _, message := range messages {
		var header struct {
			Role string `json:"role"`
		}
		if err := json.Unmarshal(message, &header); err == nil && strings.EqualFold(header.Role, "system") {
			continue
		}
		result = append(result, message)
	}
	return result
}
