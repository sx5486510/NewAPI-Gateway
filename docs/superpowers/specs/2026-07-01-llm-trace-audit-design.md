# LLM Trace Audit Design

## Goal

Add an optional audit trail for LLM relay traffic, similar in purpose to
`@mariozechner/claude-trace`: when enabled, the gateway records the full LLM
request and response content so an administrator can review the context sent to
and returned from upstream providers.

The scope is limited to LLM relay requests. Management APIs such as login,
settings, user management, provider configuration, and dashboard queries are not
audited by this feature.

## Requirements

- Provide a system switch to enable or disable LLM context auditing.
- Default to disabled to avoid unnecessary storage and runtime overhead.
- When enabled, record complete LLM request and response bodies for relay
  attempts.
- Never store authentication secrets such as `Authorization`, `x-api-key`,
  `x-goog-api-key`, upstream provider keys, or aggregated token values.
- Store audit entries separately from existing usage logs.
- Link audit entries to existing usage logs through `request_id`.
- Allow administrators to delete all historical audit records without deleting
  usage logs or billing statistics.
- Support both non-streaming JSON responses and streaming SSE responses.
- Record upstream error bodies when an upstream provider returns an error.

## Non-Goals

- Do not audit non-LLM management APIs.
- Do not implement retention scheduling in this change.
- Do not encrypt audit bodies at rest in this change.
- Do not redact prompt or completion content beyond authentication material; the
  purpose of this feature is full context audit.
- Do not change routing, retry, model fallback, usage accounting, or pricing
  behavior.

## Architecture

The feature should be implemented in the relay path because that is where the
gateway already has all needed data:

- `controller.Relay` extracts the requested model, performs allow-list checks,
  builds fallback attempts, and calls `service.ProxyToUpstream`.
- `service.ProxyToUpstream` reads the request body, rewrites the routed model,
  sends the upstream request, handles streaming and non-streaming responses, and
  writes `UsageLog`.
- `model.UsageLog` already stores `request_id`, user, provider, token, model,
  latency, usage, and error metadata.

Add a separate audit model and service-level helpers:

- `model.LLMTrace` stores full request and response content plus lightweight
  metadata.
- A small trace helper in `service/proxy.go` checks the audit switch and writes
  the trace asynchronously after each completed attempt.
- `UsageLog` remains the source for summary metrics. `LLMTrace` is the source
  for raw context inspection.

## Data Model

Add a new table through GORM `AutoMigrate`.

Suggested fields:

- `id`: primary key.
- `request_id`: short request id already generated in `ProxyToUpstream`; indexed.
- `user_id`
- `aggregated_token_id`
- `provider_id`
- `provider_name`
- `provider_token_id`
- `model_name`
- `method`
- `path`
- `status_code`
- `requested_stream`
- `response_is_stream`
- `request_body`: full body sent to upstream after model rewrite.
- `response_body`: full upstream response body; for SSE, newline-joined event
  stream text.
- `error_message`
- `client_ip`
- `user_agent`
- `created_at`: Unix timestamp, indexed.

The request body should reflect the actual upstream request. This matters when
model fallback or route aliasing rewrites the `model` field before forwarding.

## Configuration

Add `LLMTraceEnabled` to the existing option system:

- Default: `false`.
- Store in `OptionMap` and `options` table like other booleans.
- Add a matching common boolean such as `common.LLMTraceEnabled`.
- Expose it through the existing `/api/option/` response.
- Add a checkbox to the system settings page.

When disabled, trace capture should return before allocating response buffers
for audit-specific work. Existing response handling still reads bodies where it
already must do so.

## Capture Flow

### Non-Streaming Success

`ProxyToUpstream` already reads `resp.Body` into `respBody` before writing to the
client. When trace is enabled, save:

- request metadata and rewritten request body.
- `resp.StatusCode`.
- `respBody` as `response_body`.
- stream flags.
- empty `error_message`.

### Streaming Success

`ProxyToUpstream` currently scans SSE lines and writes them directly to the
client. When trace is enabled, append each scanned line to a bounded in-memory
builder while forwarding it unchanged.

At stream completion, save:

- request metadata and rewritten request body.
- `resp.StatusCode`.
- captured SSE text as `response_body`.
- stream flags.
- scanner error, if any.

The implementation should preserve streaming behavior and flushing. Audit
capture must not delay each chunk except for the minimal append cost while the
feature is enabled.

### Upstream Error Response

For upstream HTTP status `>= 400`, `ProxyToUpstream` already reads `respBody` to
construct retry/error behavior. When trace is enabled, save the request body,
error response body, status code, stream flags, and error message.

### Transport Or Local Errors

For errors before an upstream response exists, save the request body when it was
available, leave `response_body` empty, and include `error_message`.

Cooldown rejections that do not send an upstream request do not need an audit
entry because no LLM context was sent to an upstream model.

## Admin API

Add admin-only endpoints for audit records:

- `GET /api/llm-traces/`: list trace metadata with pagination and filters.
- `GET /api/llm-traces/:id`: return one full trace including request and
  response bodies.
- `DELETE /api/llm-traces/`: delete all trace records.

The delete endpoint only clears `llm_traces`. It must not delete `usage_logs`.

Filtering should initially support request id, provider name, model name, status
class, and keyword search across request id, model, provider, and error message.
Full-text search inside large request/response bodies can be deferred to avoid
heavy table scans.

## Admin UI

Add two UI surfaces:

- System settings: checkbox labeled for enabling LLM context audit.
- Log or audit page: list trace records, open a detail view for request and
  response JSON/SSE content, and provide a clearly marked admin action to clear
  all trace records.

The detail view should avoid loading full bodies in the list response. The list
should show metadata only and fetch the full trace when the user opens a record.

## Privacy and Security

This feature intentionally stores prompt and completion content. Treat the data
as sensitive:

- Keep endpoints admin-only.
- Do not store inbound aggregated token values, upstream provider keys, or auth
  headers.
- Do not include request or response bodies in normal server logs.
- Do not expose trace bodies through the existing usage-log list endpoint.
- Make the switch default off.

## Testing

Add focused backend tests around the capture helper and model/query behavior:

- Switch disabled: relay processing produces no `LLMTrace`.
- Switch enabled with non-streaming success: request and response body are saved.
- Switch enabled with upstream error: request and error body are saved.
- Switch enabled with stream: SSE lines are captured while preserving usage
  extraction behavior.
- Delete history removes only trace records and leaves usage logs intact.

Manual verification should include enabling the switch in settings, sending a
sample chat completion, viewing the trace detail, disabling the switch, sending
another request, and confirming no new trace appears.

## Rollout Notes

Because audit bodies can grow quickly, the default must remain disabled. The
first implementation includes manual clearing of history. Retention windows or
size limits can be added later if operators need automated cleanup.
