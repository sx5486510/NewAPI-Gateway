# CPA xAI Real Quota Design

## Problem

The CPA management page can show a newly imported xAI/Grok auth file as zero quota.
Two independent gaps are involved:

1. `GET /v0/management/auth-files` exposes the runtime index and provider, but not
   the `sub` stored in the credential JSON. The browser quota adapter therefore
   omits the `x-userid` header required by the Grok billing endpoints.
2. CLIProxyAPI v7.2.80 (and the checked v7.2.94) does not refresh xAI OAuth access
   tokens in the generic management `api-call` handler. An expired access token is
   forwarded unchanged and can produce 401 responses.

The fix must keep refresh tokens out of new browser-facing responses and must not
turn an authentication or upstream failure into a successful zero-quota result.

## Recommended Architecture

Keep the existing browser quota adapters and management proxy boundary.

### Browser quota flow

`CPAAuthFiles` already downloads each visible credential for status display. Extend
that detail path to retain only the non-secret xAI subject identifier needed by the
quota adapter. Pass the detail to `fetchCPAQuota`; do not retain or render access or
refresh tokens in React state.

`fetchGrokQuota` resolves the user id in this order: explicit list metadata, cached
credential detail, then no header. When available it sends `x-userid` on both billing
requests. A production-shaped auth list entry without `sub` must therefore still
produce the same request as a complete credential file.

### Backend token flow

Extend the gateway-owned CPA management proxy for xAI API-call requests that use
`$TOKEN$`:

1. Resolve the selected auth by `auth_index` through the CPA management interface.
2. Read the credential JSON server-side and determine whether `expired` is within
   the refresh skew.
3. If it is still valid, preserve the current transparent forwarding behavior.
4. If it is expired, refresh with the stored xAI OAuth refresh token using the
   existing xAI OAuth endpoint/client contract from the pinned CPA SDK.
5. Atomically persist the returned token fields to the credential file. The CPA
   file watcher updates runtime state for later requests.
6. For the current request, replace `$TOKEN$` in the parsed API-call headers with
   the fresh access token before forwarding. This avoids racing the asynchronous
   file watcher while keeping the token on the gateway-to-loopback boundary.

Only the selected auth is touched. Refresh failures return a stable non-2xx error
(`auth_token_refresh_failed`) and are surfaced by the existing quota error UI;
they must never be converted to a quota object containing zero values.

The proxy must validate the credential path remains inside the configured auth
directory, preserve unrelated JSON fields, use restrictive file permissions, and
avoid logging token values.

## Error and Zero-Quota Semantics

- Missing `x-userid` is allowed only when the credential genuinely has no subject;
  the request remains observable through the existing error response.
- A 401/403 from either billing endpoint remains an error state and keeps its
  status code for the existing 401 filter.
- A successful billing response with an explicit zero plan limit is a valid
  zero-quota account and is displayed as such.
- An empty or structurally invalid billing response is an error, not zero quota.

## Tests

### Frontend

- A production-shaped auth list item with no `sub` loads credential detail and sends
  the parsed subject as `x-userid`.
- An explicit list subject takes precedence over downloaded detail.
- Billing 401 remains an error with status code 401.
- Explicit zero plan limits remain valid zero quota.

### Backend

- Valid xAI access tokens are forwarded without a refresh.
- Expired xAI credentials refresh once, atomically persist the returned fields, and
  forward the new token without exposing it in the response.
- Refresh endpoint failure returns `auth_token_refresh_failed` and leaves the
  original credential file unchanged.
- Auth paths outside the configured directory are rejected.
- Existing management proxy upload, API-call, and reset-quota tests remain green.

## Scope and Rollout

This change is limited to xAI quota requests and the xAI token refresh path in the
embedded CPA management proxy. Other providers keep their existing adapters and
refresh behavior. No database schema change or public API response change is
required.
