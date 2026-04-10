# 2026-04-10 low balance route disable design

## Goal
Disable a provider's rebuilt routes when the provider balance is below 1 USD, while keeping the existing balance sync behavior unchanged and preserving manual route disables.

## Confirmed rules
1. Balance synchronization behavior must remain unchanged.
2. The balance threshold is checked only during route rebuild.
3. When provider balance is below `1`, rebuilt routes for that provider are disabled.
4. When provider balance later returns to `>= 1`, rebuilt routes are enabled again on the next rebuild.
5. Routes that were manually disabled by an admin must remain disabled even after balance recovery.

## Recommended approach
Keep the change inside `service.RebuildProviderRoutes(providerId int)`.

This matches the current architecture:
- `service/sync.go` owns sync-time business rules and route generation.
- `model.RebuildRoutesForProvider(...)` owns persistence and state carry-forward for existing routes.

This keeps the feature scoped to rebuild-time enablement logic instead of changing balance sync, while allowing one minimal internal route-state field so automatic disable and manual disable can be distinguished.

## Design changes

### 1. Route rebuild reads provider balance
Before generating route rows, load the provider record and parse `providers.balance` with the existing `parseBalanceUSD(...)` helper from `model/model_route.go`.

Derived runtime flag:
- `providerBalanceLow = parsedBalance < 1`

If balance cannot be parsed, the existing helper returns `0`, which should be treated as low balance.

### 2. Internal route state marker
Add an internal boolean field on `model_routes`, for example `auto_disabled_by_balance`, with default `false`.

Purpose:
- `false`: route enablement is normal or manually controlled
- `true`: route is currently disabled by the low-balance rebuild rule and may be auto-restored on a later rebuild

This field is internal only and does not need to be exposed in the API response or frontend.

### 3. Rebuild-time route enablement rule
When `service.RebuildProviderRoutes(...)` constructs each `model.ModelRoute`, determine the initial `Enabled` state using the low-balance flag:
- if `providerBalanceLow == true`, generated routes default to `Enabled = false`
- if `providerBalanceLow == false`, generated routes default to `Enabled = true`

At the same time, set the internal marker:
- if `providerBalanceLow == true`, generated routes default to `auto_disabled_by_balance = true`
- if `providerBalanceLow == false`, generated routes default to `auto_disabled_by_balance = false`

This does not modify sync balance behavior. It only changes the state written during rebuild.

### 4. Preserve manual disables during persistence
`model.RebuildRoutesForProvider(...)` already loads previous routes and carries forward `Enabled`, `Priority`, and `Weight` for matching `(model_name, provider_token_id)` pairs.

To support automatic recovery without overriding manual disables, the carry-forward rule needs to become:
- previous route `enabled = false` and `auto_disabled_by_balance = false`: treat as manual disable and keep `enabled = false`
- previous route `enabled = false` and `auto_disabled_by_balance = true`: treat as prior low-balance disable and allow the newly generated route state to replace it
- previous route `enabled = true`: allow the newly generated route state to replace it

Effect:
- system can auto-disable a previously enabled route when low balance is detected
- system can auto-re-enable that same route after balance recovery on the next rebuild
- a manually disabled route remains disabled across rebuilds regardless of balance

### 5. Manual route edits clear automatic state
When an admin explicitly updates route `enabled` through `UpdateRoute` or `BatchUpdateRoutes`, the persistence layer should also set `auto_disabled_by_balance = false`.

This ensures a human action becomes the source of truth for later rebuilds:
- manual disable stays manual
- manual enable is not mistaken for an old low-balance disable

### 6. Rebuild entry points
The updated behavior applies everywhere that calls `service.RebuildProviderRoutes(...)`, including:
- normal provider sync after pricing, token, and balance sync
- manual route rebuild across all providers via `service.RebuildAllRoutes()`
- token update flows that rebuild provider routes

This is intentional because the requirement is tied to route rebuild, not only to the sync button.

## Tests
Add focused tests around rebuild behavior:
- provider balance `< 1` disables rebuilt routes that were previously enabled
- provider balance `>= 1` re-enables rebuilt routes that were previously auto-disabled by low balance
- previously manual-disabled routes remain disabled after balance recovery
- explicit manual route updates clear the automatic low-balance marker
- routes for providers with parseable balance strings like `$0.99` and `$1.00` follow the threshold exactly

Preferred coverage split:
- unit test around the persistence carry-forward rule in `model.RebuildRoutesForProvider(...)`
- unit test around route update helpers clearing the automatic marker when `enabled` is patched
- service-level test around `service.RebuildProviderRoutes(...)` for balance threshold handling

## Error handling and compatibility
- A minimal database schema change is required through GORM auto-migration for the new internal route-state field.
- No API contract change is required.
- No frontend change is required for the initial implementation.
- Existing `syncBalance(...)` behavior remains unchanged.
- Existing route generation logic for groups, model limits, priority, and weight remains unchanged.

## Scope boundaries
This change does not include:
- changing the balance sync API call or storage format
- adding a new provider status field for low balance
- changing admin manual enable/disable endpoints
- changing runtime route selection logic outside rebuild results

## Acceptance criteria
1. Triggering provider sync still syncs pricing, tokens, and balance exactly as before.
2. If a provider balance is below `1`, rebuilt routes for that provider are stored as disabled and marked as automatically disabled by balance.
3. If a provider balance later becomes `>= 1`, the next rebuild re-enables routes that were only disabled by prior low balance.
4. Routes manually disabled by an admin remain disabled after any rebuild, including after balance recovery.
5. Explicit route enable/disable edits clear the automatic low-balance marker.
6. Manual global rebuild uses the same low-balance rule for each enabled provider.
