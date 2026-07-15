# Model route token quota design

## Goal

Show the quota associated with every model-route row without adding per-row upstream requests.

## Quota semantics

A model route does not own an independent quota. Each route references one local `provider_tokens` row through `model_routes.provider_token_id`, and all routes that use the same provider token share that token's quota.

The UI will therefore label the value as `令牌额度` and display the synchronized token quota:

- `unlimited_quota = true`: display `无限`.
- Otherwise: display `remain_quota / 500000` as a USD amount.
- The raw `used_quota` value remains available in the route overview response for later diagnostics, but this change does not add a second visible usage column.

Provider account balance remains a separate provider-wide value. It is not combined with token quota in this feature because both values are shared constraints and combining them into a route-specific number would imply a quota owned exclusively by that route.

## Data flow

The existing provider synchronization already retrieves `unlimited_quota`, `remain_quota`, and `used_quota` from upstream `GET /api/token/` and stores them in `provider_tokens`.

`GetModelRouteOverview` will extend its existing `LEFT JOIN provider_tokens` projection with those three fields. `ModelRouteOverviewItem` will expose them as:

- `token_unlimited_quota`
- `token_remain_quota`
- `token_used_quota`

No database migration and no new upstream endpoint are required.

## UI behavior

The model-route detail table adds one `令牌额度` column after `供应商 / 令牌`.

- Unlimited quota renders a green `无限` badge.
- Finite quota renders a USD value with two decimal places.
- Missing or orphaned token data renders `—` instead of incorrectly showing zero.

Because a token can back several model routes, repeated values across those rows are expected.

## Error and compatibility behavior

The overview query continues using a left join so orphaned routes remain visible. Nullable token quota columns from the join distinguish a missing token from a real finite quota of zero.

Existing API consumers remain compatible because the response only gains fields. Key-only providers keep their current synthetic unlimited token behavior.

## Testing

- Backend model tests verify that overview rows return finite and unlimited token quota fields and preserve missing-token distinction.
- Frontend component tests verify finite USD formatting, unlimited display, and the additional table column span.
- Run focused Go and React tests, followed by broader relevant suites.
