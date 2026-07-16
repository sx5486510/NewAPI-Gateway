# Grok 4.5 System Prompt Configuration Design

## Objective

Configure the existing `grok-4.5` model route to inject the complete prompt from
`Jailbreak-Guide/Grok/Grok 4.5/ENI LIME.md` through the gateway's existing
administrator-managed system prompt feature.

This is runtime configuration, not a new bundled preset or a code change. The
gateway must store the source document verbatim and inject it only when the
selected route handles an exact `POST /v1/chat/completions` request.

## Confirmed Target

- Gateway: `http://localhost:3030`
- Model name: `grok-4.5`
- Current route ID: `679029`
- Current route binding: none
- Prompt name: `ENI LIME (Grok 4.5)`
- Source file: `Jailbreak-Guide/Grok/Grok 4.5/ENI LIME.md`
- Source size: 36,000 bytes, 35,828 UTF-8 characters
- Source SHA-256: `799c86295a336a93dc56b8180fb9960c63db9954584d1ea0d46d052980ec2008`
- Browser-normalized size: 35,646 bytes, 35,474 characters
- Browser-normalized SHA-256: `6267e3c9b6c4f71a3da90c403930d8b8ae58e1ee7efc4505b85415d395892de7`

The source content must not be summarized, sanitized, templated, or wrapped in
additional instructions. HTML textareas normalize CRLF line endings to LF; the
stored prompt is considered verbatim when it equals the source after only this
line-ending normalization.

## Selected Approach

Use the existing administrator web interface and its session-authenticated API:

1. Sign in through the normal administrator login flow.
2. Open the system prompt management page.
3. Create `ENI LIME (Grok 4.5)` with exact model name `grok-4.5` and the complete
   UTF-8 source content.
4. Open the model routes page and bind the new preset to route `679029`.
5. Reload both pages and verify that the persisted prompt and route binding are
   displayed.

The API's `NoTokenAuth` middleware intentionally rejects ordinary API tokens.
The configuration must therefore use an authenticated administrator browser
session. Credentials and session cookies must not be copied into scripts,
documentation, shell history, or chat messages.

## Data Flow

The management page sends the prompt to `POST /api/system-prompt/`. After the
record is created, the route page sends its numeric ID as `system_prompt_id` to
the existing route update API. The backend requires the preset's `model_name`
to equal the route's `model_name` exactly.

For a matching chat-completions request, the proxy reloads the bound preset and
prepends this message to the outgoing request:

```json
{"role":"system","content":"<verbatim stored prompt>"}
```

All client messages retain their original order after that message. Other
paths, methods, models, and unbound routes remain unchanged.

## Error Handling And Rollback

- Reject creation if the name/model pair already exists; inspect the existing
  record before deciding whether it should be updated.
- Reject the binding if the route model and prompt model do not match exactly.
- Do not issue an external Grok request merely to test configuration.
- Roll back operationally by clearing `system_prompt_id` from route `679029`.
- Delete the preset only after it has been unbound from every route.

## Verification

1. Verify the source file SHA-256 immediately before configuration.
2. Confirm the saved record has model name `grok-4.5` and the expected name.
3. Confirm the saved UTF-8 content has the documented browser-normalized byte
   length and SHA-256, and equals the source after CRLF-to-LF normalization.
4. Confirm route `679029` references the created preset ID.
5. Run the existing focused Go tests for system prompt persistence, route
   binding, request transformation, and proxy integration.
6. Confirm no source code or migration files changed as part of this runtime
   configuration.

## Non-Goals

- No automatic import from the external Jailbreak-Guide repository.
- No default prompt seeding for new installations.
- No wildcard model matching or automatic binding of future routes.
- No change to the injection protocol or request transformation code.
- No live request to an external provider during verification.
