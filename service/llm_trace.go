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
