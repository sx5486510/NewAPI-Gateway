# Route System Prompt Injection Design

## Objective

Add administrator-managed system prompt presets to NewAPI Gateway. Each preset belongs to exactly one model name. Each model route may select at most one compatible preset, with no preset selected by default. For OpenAI `POST /v1/chat/completions` requests, the gateway inserts the selected preset as the first system message while preserving all client messages.

The first version does not import or synchronize content from external repositories. Administrators selectively copy trusted prompt content into the gateway so untrusted hidden Unicode or upstream repository changes cannot enter production automatically.

## Scope

### Included

- An administrator-only system prompt management page.
- Multiple named prompt presets for each exact `model_name`.
- Create, list, update, filter, and delete operations for presets.
- One optional preset binding per model route.
- A prompt selector in each row of the model routes table.
- Injection into OpenAI `POST /v1/chat/completions` only.
- Preservation of client-provided system messages after the gateway preset.
- Explicit handling of deletion when a preset is referenced by routes.

### Excluded

- OpenAI Responses, Anthropic Messages, Gemini, completions, embeddings, and other protocols.
- Automatic GitHub synchronization or bundled jailbreak content.
- Prompt safety review, moderation, or runtime risk blocking.
- Multiple simultaneous presets on one route.
- Prompt templating or variable substitution.

## Data Model

Create a `system_prompts` table with:

| Field | Type | Rules |
| --- | --- | --- |
| `id` | integer | Primary key |
| `name` | string | Trimmed, non-empty |
| `model_name` | string | Trimmed, non-empty, indexed |
| `content` | text | Non-empty |
| `created_at` | integer | Creation timestamp |
| `updated_at` | integer | Last update timestamp |

Add a unique constraint on `(model_name, name)`. Names may repeat across different models.

Add nullable `system_prompt_id` to `model_routes`. A null value means no prompt and preserves current behavior. Application-level validation must ensure that a selected preset exists and its `model_name` exactly equals the route's `model_name`.

The prompt list response includes a computed `route_count` for deletion warnings and administration display. Prompt content is resolved for a route attempt before proxying; a corrupted reference or model mismatch must not silently become an unprompted request.

## Administrator API

Add an administrator-only route group protected by the existing `AdminAuth` and `NoTokenAuth` middleware:

```text
GET    /api/system-prompt/
POST   /api/system-prompt/
PUT    /api/system-prompt/:id
DELETE /api/system-prompt/:id
```

The list endpoint supports exact model filtering and name search. Create and update validate trimmed names, exact model association, non-empty content, and per-model name uniqueness.

Deletion behaves as follows:

- An unreferenced preset is deleted normally.
- A referenced preset is rejected by default and returns its route reference count.
- `DELETE /api/system-prompt/:id?unbind=true` performs route unbinding and preset deletion in one database transaction.
- The UI presents cancellation and explicit automatic unbinding; it never sends `unbind=true` without administrator confirmation.

Extend the existing single-route and batch-route update payloads with nullable `system_prompt_id`. The backend validates the preset rather than relying on filtered frontend options.

## User Interface

Add a system prompt item to the administrator navigation and a dedicated management page.

The page provides:

- Model filter and prompt-name search.
- A table containing name, model name, content summary, referenced route count, update time, and actions.
- Create and edit dialogs with name, model name, and multiline content fields.
- A delete confirmation that, when references exist, offers cancellation or automatic route unbinding and deletion.

Add a `System Prompt` column to the detailed model routes table. Each row contains a selector whose first option is `No system prompt`. Remaining options contain only presets whose `model_name` exactly matches the row. Changes participate in the table's existing batch-save flow.

Editing a preset immediately affects every bound route on subsequent requests because routes store a preset reference, not copied prompt content.

## Proxy Data Flow

The selected `ModelRoute` must be available to the proxy attempt. Extend the proxy call boundary to carry the route, or an equivalent immutable attempt object, alongside its provider and token.

For every route attempt:

1. Read the cached original client request body.
2. Rewrite the upstream model using the existing resolved-model behavior.
3. If the request is not `POST /v1/chat/completions`, do not inject a prompt.
4. If the route has no preset binding, do not parse or modify messages.
5. Resolve and validate the bound preset for the selected route.
6. Parse the JSON body and require `messages` to be an array.
7. Insert `{ "role": "system", "content": <preset content> }` at index zero.
8. Encode and send the attempt-specific body upstream.

Every retry starts from the cached original body. This prevents duplicate injection and ensures that retrying onto a different route selects that route's preset instead of retaining the previous attempt's content.

Client-provided system messages remain in their original relative order after the gateway-injected message. Streaming and non-streaming requests share the same request transformation.

## Error Handling and Observability

- Missing or non-array `messages` on a route requiring injection returns HTTP 400 using the OpenAI-compatible `invalid_request_error` shape.
- Invalid JSON continues to use the gateway's request validation/error conventions.
- A missing preset or model mismatch on a configured route makes that route attempt unavailable and records a server-side diagnostic. The request may continue through normal retry selection, but it must never use that route without its configured prompt.
- Prompt content is encoded with the JSON encoder so quotes, newlines, and Unicode remain valid JSON.
- LLM trace capture stores the actual attempt body sent upstream, including the injected message, subject to the gateway's existing trace redaction and enablement settings.
- Prompt CRUD and binding endpoints never expose data to non-administrators.

## Migration and Compatibility

Use the project's existing database migration pattern to create `system_prompts` and add nullable `model_routes.system_prompt_id`. Existing rows remain null, so deployments retain current forwarding behavior until an administrator explicitly binds a preset.

Route rebuilds must preserve prompt bindings for stable route identities. If a rebuild removes a route, no prompt preset is deleted. If route identity matching cannot preserve a binding, the rebuild must leave the new route unbound rather than guessing based only on provider or token data.

## Testing

Backend tests cover:

- Prompt CRUD, trimming, empty values, per-model uniqueness, filtering, and admin authorization.
- Binding a matching preset, rejecting a cross-model preset, clearing a binding, and batch updates.
- Rejecting deletion of referenced presets and transactional automatic unbinding.
- No-binding requests retaining current behavior.
- Injection into an empty messages array.
- Gateway prompt ordering before existing client system messages.
- Correct JSON encoding of multiline and Unicode prompt content.
- Different retry routes using their own prompts without duplicate injection.
- Non-chat-completions paths remaining unchanged.
- Malformed JSON and invalid `messages` errors.
- Trace capture containing the actual injected upstream request body.
- Route rebuild preservation for stable identities.

Frontend tests cover:

- Prompt page listing, filters, validation, create/edit operations, and referenced-delete confirmation.
- Route selectors showing only exact-model presets and defaulting to no prompt.
- Selection changes participating in existing batch save and clear operations.

## Acceptance Criteria

- Administrators can manage multiple prompt presets for an exact model name on a dedicated page.
- Every model route defaults to no preset and may select one compatible preset.
- A bound preset becomes the first system message for that route's OpenAI chat completions requests.
- Client messages, including existing system messages, are preserved.
- Retry attempts reconstruct their bodies and use the selected route's own preset.
- Unsupported protocols and unbound routes behave as before.
- Referenced prompt deletion requires an explicit administrator choice and can atomically unbind routes.
- Existing routes remain unbound after migration, introducing no forwarding behavior change by default.
