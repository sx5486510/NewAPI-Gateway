# Route Group in Logs and Audit Design

## Goal

Show the provider token group used by the actual matched request route on every usage log record and every LLM audit record.

The group name is the same value shown on the route page as `token_group_name`, sourced from `provider_tokens.group_name`.

## Approach

Persist the selected token group name at request time.

When proxy routing chooses a provider token, `logUsage` and `captureLLMTrace` already receive the selected `ProviderToken`. Both writers will copy `token.GroupName` into their records. This keeps each record tied to the group that was used when the request happened, even if the token is later edited or deleted.

## Data Model

Add `TokenGroupName` to:

- `model.UsageLog` with JSON field `token_group_name`.
- `model.LLMTrace` with JSON field `token_group_name`.

Existing AutoMigrate flow will create the new columns. Existing rows will have an empty value and should display as `-`.

## Backend Flow

Usage logs:

- `service.logUsage` sets `TokenGroupName` from `token.GroupName`.
- `UsageLog.Insert` includes `token_group_name`.
- Existing list queries return the field with the rest of the record.

LLM audit:

- `service.captureLLMTrace` sets `TokenGroupName` from `input.Token.GroupName`.
- `QueryLLMTraces` includes the field in list projections.
- `GetLLMTraceByID` returns the field through the full model.

## Frontend

Add one metadata pill to each record:

- Logs page: show label `分组` and value `log.token_group_name || '-'`.
- Audit page: show label `分组` and value `trace.token_group_name || '-'`.

No new filters or search behavior are included in this change.

## Testing

Backend tests should verify that writing usage logs and LLM traces stores `token_group_name` from the selected provider token.

Frontend tests should be updated only where existing component tests make this low risk; otherwise build verification is sufficient for the display-only JSX change.
