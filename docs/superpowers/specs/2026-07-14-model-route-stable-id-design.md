# Stable Model Route IDs Design

## Problem

Provider synchronization rebuilds a provider's model routes every five minutes. The current rebuild deletes every route and inserts replacement rows. Although it preserves selected route settings, it assigns new route IDs.

An already-open route page continues to submit the old IDs. GORM treats an update that matches no rows as a successful SQL statement, so the API reports success even though the status or client restriction was not saved. After the page reloads with the replacement IDs, the same operation succeeds, which appears as a required second click.

## Goal

Keep the database ID of every unchanged logical route stable across provider synchronization and reject updates that reference a route that no longer exists.

This change covers route status and the Codex, CC, and block-all client restriction fields because they use the same route update functions. It does not change the frontend interaction model: status edits remain part of the existing save flow, while client restriction edits remain immediate.

## Route Identity

A route is logically identified within a provider by:

- `provider_id`
- `provider_token_id`
- `model_name`

`RebuildRoutesForProvider` already uses the token/model portion of this key after filtering existing rows by provider. The new implementation keeps that identity rule and preserves the matched row's primary key.

## Rebuild Algorithm

`RebuildRoutesForProvider` will continue to run in one database transaction:

1. Load all existing routes for the provider and index them by token ID and model name.
2. For every generated route that matches an existing logical route, update the generated fields on the existing row instead of deleting it. Preserve the existing `id`, `enabled`, `allow_codex`, `allow_cc`, and `block_clients` values.
3. Insert generated routes that have no existing logical match.
4. Delete existing routes whose logical keys are absent from the generated set.
5. Commit only after all updates, inserts, and deletes succeed; otherwise roll back the complete rebuild.

The generated `priority` and `weight` fields continue to follow the incoming route data. A matched route's provider ID, provider token ID, and model name are its identity and therefore do not need to change.

No schema migration or new unique index is included. This keeps the fix compatible with the current schema and limits it to the synchronization behavior that creates the stale IDs.

## Stale Update Handling

`UpdateModelRouteFields` will inspect the GORM result after updating by primary key. If the result reports zero affected rows, it will query for that ID: an existing row means the requested values were already stored and the update remains an idempotent success; an absent row returns a descriptive route-not-found error.

`BatchUpdateModelRoutes` will apply the same check to every non-empty patch using the active transaction. If any requested route ID is absent, it rolls back the entire batch and returns an error. This prevents an earlier valid patch in the same request from being committed when a later patch is stale.

The existence check after a zero-row update is required for database portability. MySQL can report zero affected rows when an update assigns values that are already stored, while PostgreSQL and SQLite may report matched rows differently.

Existing validation and client restriction normalization remain unchanged.

## Error Behavior

Database errors are returned as before. Missing route IDs become application errors from the model layer and flow through the existing controller error response path. The frontend therefore receives failure rather than being told that an unapplied change succeeded.

## Testing

Add focused regression tests to `model/model_route_test.go`:

- Rebuilding a provider with an unchanged logical route preserves that route's ID and manual status/client restriction settings.
- Rebuilding inserts a genuinely new route and removes a route that is no longer generated.
- Updating a nonexistent route ID with `UpdateModelRouteFields` returns an error.
- A batch containing one valid route ID and one nonexistent route ID returns an error and leaves the valid route unchanged.

The tests use the existing in-memory SQLite setup and call the model functions directly. Each regression test must fail against the current delete/reinsert or zero-row-success behavior before the implementation is changed.

After implementation, run the focused model route tests and then the full Go test suite. No frontend change or frontend test is required unless backend verification exposes a separate UI defect.

## Non-Goals

- Changing route page controls or save behavior.
- Adding retries for stale IDs.
- Changing provider synchronization frequency.
- Refactoring unrelated routing selection logic.
- Cleaning up existing high auto-increment values.
