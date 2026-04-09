# 2026-04-09 cooldown classification update design

## Goal
Align the fault-cooldown implementation with the confirmed runtime rules for upstream error handling while preserving the existing cooldown manager structure.

## Confirmed rules
1. All 4xx responses default to token-level cooldown.
2. `model_not_found` and `permission_denied` are exceptions across all 4xx and should mark `(provider_token_id, model)` as unsupported instead of cooling down the whole token.
3. 5xx responses and transport failures (timeout, EOF, TLS, dial/connect failures) should trigger route-level cooldown.
4. Client-canceled requests should not trigger cooldown.
5. Upstream `Retry-After` should participate in local cooldown duration selection.
6. Non-stream success semantics remain unchanged: once a successful upstream response is obtained, it counts as success.

## Recommended approach
Keep the current architecture:
- `service/proxy.go` continues to classify upstream failures.
- `common/route_cooldown.go` continues to own cooldown state and duration calculations.

This minimizes risk and keeps the change scoped to the rule update rather than mixing in a broader refactor.

## Design changes

### 1. Error classification updates in `service/proxy.go`
Update the current helper functions so behavior becomes:
- unsupported-model detection runs first and recognizes `model_not_found` and `permission_denied` across all 4xx statuses, not only 400/404.
- if unsupported-model detection matches, mark unsupported-model cache and do not apply token cooldown for that response.
- otherwise, any 4xx response triggers token cooldown.
- 5xx/502/503/504/408 and transport failures continue to trigger route cooldown.

The existing `Retryable` response behavior can remain as-is unless required by the cooldown update; this change is focused on cooldown side effects, not on retry semantics redesign.

### 2. Retry-After participation in cooldown duration
Extend cooldown recording APIs so callers may provide an optional `retryAfterSeconds` hint.

Cooldown duration rule:
- compute the existing exponential-backoff duration
- if upstream supplied `Retry-After > 0`, the final cooldown duration becomes `max(localBackoffSeconds, retryAfterSeconds)`

Apply this rule to:
- token cooldown for all applicable 4xx responses, including 429
- route cooldown for 5xx/408/502/503/504 when `Retry-After` is present

Transport-layer request failures from `http.Client.Do` continue using local backoff only because they have no upstream response headers.

### 3. Cooldown manager API changes
Adjust `common/route_cooldown.go` to expose explicit failure-recording methods that accept an optional minimum cooldown duration, for example:
- route failure with minimum seconds
- token failure with minimum seconds

The cooldown manager remains responsible for:
- decay
- half-open logic
- unsupported-model TTL
- jitter and exponential backoff

Only the final cooldown duration selection changes.

### 4. Tests
Add or update tests for:
- `403 permission_denied` -> unsupported model, not token cooldown
- generic 4xx -> token cooldown
- `Retry-After` increases local token cooldown duration when larger than local backoff
- `Retry-After` increases local route cooldown duration when larger than local backoff

## Error handling and compatibility
- Existing success handling remains unchanged.
- Existing fallback-model logic remains unchanged.
- No database schema changes are required.
- No config shape changes are required.

## Scope boundaries
This change does not include:
- redesigning `Retryable` semantics for all HTTP statuses
- adding provider-level cooldown
- adding fallback capability guards or new response headers
- moving all classification logic into a new module

## Acceptance criteria
1. `403 permission_denied` causes unsupported-model filtering for the current `(token, model)` pair.
2. Other 4xx responses place the token into cooldown.
3. 5xx and transport failures still isolate only the route.
4. Local cooldown duration honors upstream `Retry-After` as a lower bound when present.
5. Existing route cooldown features (decay, half-open, unsupported TTL) continue to work.
