# Embedded CPA Full Management Design

**Date:** 2026-07-16

## Goal

Give Gateway Root users complete control of the embedded CLIProxyAPI (CPA) instance from the Gateway UI. Reuse the official CPA management panel and support the full management API surface, including credential import, download, enable/disable, field edits, deletion, OAuth, quota inspection and reset, configuration, logs, API keys, providers, and plugins.

The integration must preserve Gateway as the only externally reachable service. The CPA listener and its real management credential must remain private to the Gateway process.

## Current State

The repository embeds `github.com/router-for-me/CLIProxyAPI/v7 v7.2.80` and starts it on `127.0.0.1`. The current Gateway API exposes only:

- `GET /api/cpa/config`
- `PUT /api/cpa/config`
- `POST /api/cpa/reload`

Only four values are persisted: enabled, proxy API keys, auth directory, and port. Startup rebuilds a minimal YAML file in `.smoke/cpa-config.yaml`. This would overwrite or reset settings changed through CPA's full management API, so it cannot remain the persistence model.

CPA v7.2.80 already provides the required management routes under `/v0/management`, including wildcard-extensible configuration endpoints, authentication files, OAuth, quota reset, logs, API key usage, providers, and plugins. Its SDK also provides `WithLocalManagementPassword`, which accepts an in-memory password only for localhost management requests.

## Chosen Approach

Use a version-pinned copy of the official CPA management panel and place it behind a same-origin Gateway bridge.

Alternative approaches were rejected:

1. Exposing the CPA port directly breaks the single-entry architecture and makes remote access and authentication harder to secure.
2. Reimplementing the panel in the Gateway React application duplicates a large and changing upstream feature set and would inevitably lag behind CPA.

The official panel is pinned to Management Center v1.18.3 for CPA v7.2.80. Its reviewed `management.html` is stored as `service/cpa/assets/management.html`, embedded in the Gateway binary, and verified against SHA-256 `941a49a619a719a59e4c7917c6888a53eb3f41a4fa2fbb5c1cc94f2d1fc9cd4b`. Panel upgrades must be reviewed and shipped with CPA dependency upgrades. Runtime download or execution of an unverified panel is not allowed.

## User Experience

Root users receive a top-level `CPA Management` navigation item and page. Admin and ordinary users do not see the item and cannot access its routes directly.

The page contains a small Gateway-native lifecycle bar with:

- current state: stopped, starting, running, stopping, or error;
- CPA version and internal endpoint status;
- start, stop, and restart commands;
- the last lifecycle error, when present.

When CPA is running, the official management panel fills the remaining viewport in a same-origin iframe. When CPA is stopped or unavailable, the iframe is not loaded and the page presents the offline state and start action. Gateway navigation remains visible around the panel.

The parent page prepares the panel's legacy local-storage connection values before loading it. It supplies the current Gateway origin and a non-secret placeholder management key. This skips the panel's independent login screen. The placeholder has no authority: the Gateway management bridge discards it and injects the real in-memory password server-side.

The Gateway-facing routes are:

- `GET /api/cpa/status` for lifecycle state, readiness, version, and the last error;
- `POST /api/cpa/start` to start a stopped instance;
- `POST /api/cpa/stop` to stop a running instance;
- `POST /api/cpa/restart` to perform a serialized restart;
- `GET /api/cpa/panel` to serve the pinned official panel asset;
- `/v0/management` and `/v0/management/*path` for the authenticated management bridge.

The existing `GET /api/cpa/config`, `PUT /api/cpa/config`, and `POST /api/cpa/reload` routes remain compatible. They are reimplemented on top of the full snapshot and lifecycle manager; `reload` is an alias for `restart`. A basic config update patches only enabled, API keys, auth directory, and port into the full document rather than recreating a minimal document.

## Architecture

```text
Root browser
  |
  +-- /cpa ---------------- Gateway lifecycle page
  |                           +-- start
  |                           +-- stop
  |                           +-- restart
  |
  +-- pinned official panel iframe
        |
        +-- /v0/management/* -- Root session and same-origin checks
                                    |
                                    +-- strip browser credentials
                                    +-- inject local management password
                                    +-- proxy to 127.0.0.1:<CPA port>
```

### Components

1. **CPA lifecycle manager**
   Owns the running instance, state transitions, base URL, runtime management password, readiness, shutdown, and the last error. Mutating lifecycle operations are serialized.

2. **CPA management reverse proxy**
   Proxies every method and subpath below `/v0/management`. It preserves query strings, request bodies, content types, response bodies, status codes, downloads, and CPA version headers while removing hop-by-hop and client-supplied authentication headers.

3. **Pinned panel asset handler**
   Serves the reviewed official single-file panel only to Root sessions. It does not fetch assets at runtime.

4. **Gateway CPA page**
   Displays lifecycle controls, prepares the placeholder panel session, and mounts or removes the panel iframe according to runtime state.

5. **Configuration snapshot store**
   Persists the complete validated CPA YAML in the Gateway database and restores the runtime file at startup.

6. **Provider synchronization coordinator**
   Debounces model synchronization after configuration, credential, or OAuth changes and keeps the auto-registered `__embedded_cpa__` provider consistent with the running CPA instance.

Each component has a narrow contract. The panel never receives the real management password, and the proxy does not own lifecycle or configuration interpretation.

## Authentication And Network Boundary

All panel asset, lifecycle, status, and management proxy routes require Gateway `RootAuth` plus `NoTokenAuth`. User API tokens, Admin sessions, and ordinary sessions are rejected.

For every proxied request, Gateway removes:

- `Authorization`;
- `X-Management-Key`;
- browser cookies before forwarding upstream;
- standard hop-by-hop proxy headers.

Gateway then sends the random runtime password as the CPA management authorization header. The password is generated with a cryptographically secure random source for each CPA start, stored only in the lifecycle manager, passed through `WithLocalManagementPassword`, and erased when the instance stops. CPA accepts it only from localhost.

State-changing methods (`POST`, `PUT`, `PATCH`, and `DELETE`) also require a same-origin `Origin` or `Referer` consistent with the Gateway host. Existing Gateway session cookie protections remain in force.

The embedded CPA host is forced to `127.0.0.1` at runtime even if the YAML requests a different host. Full YAML editing therefore cannot expose CPA on an external interface. No generic CPA root proxy is provided.

## OAuth Callbacks

OAuth support uses explicit callback relays only for provider paths supported by the pinned CPA version: `GET /anthropic/callback`, `GET /codex/callback`, and `GET /antigravity/callback`. CPA's authenticated `/v0/management/oauth-callback` route continues through the management bridge. Callback relays are not a wildcard proxy.

Provider callbacks cannot carry the Gateway login session, so they are accepted without `RootAuth` only when CPA confirms that the provider and state match a pending, unexpired, one-time OAuth session created through an authenticated management request. Invalid, mismatched, reused, or expired states are rejected. Callback request bodies and tokens are never logged.

OAuth status success and credential mutations schedule a debounced provider/model synchronization.

## Configuration Persistence And Migration

The runtime YAML file remains CPA's hot-reload source. The Gateway option `CPAConfigYAML` stores the complete YAML snapshot for durable recovery.

On first startup after upgrade:

1. If no full snapshot exists, read the existing four CPA options.
2. Produce a valid full CPA configuration using those values and CPA defaults.
3. Force the embedded host invariant.
4. Persist the full YAML snapshot.
5. Materialize it to `$CPA_RUNTIME_DIR/config.yaml`.

The `.smoke` proof-of-concept path is retired. `CPA_RUNTIME_DIR` defaults to `cpa` relative to the process working directory, which resolves to `/data/cpa` in the provided container. The directory is created with owner-only write permissions and the YAML is written atomically. The enabled lifecycle flag remains the existing Gateway `CPAEnabled` setting rather than a CPA YAML field.

After a successful management mutation that may alter configuration, Gateway validates the runtime YAML and synchronously saves its complete contents before returning success to the browser. If CPA changed runtime state but database persistence fails, Gateway returns a structured `persistence_failed` response stating that runtime application succeeded but durability failed. The on-disk YAML is retained for recovery.

At startup, Gateway validates both the database snapshot and any existing runtime YAML. If the valid disk file differs from the snapshot, it is treated as recovery data from an interrupted persistence window and is written back to the database before CPA starts. If the disk file is absent or invalid, the validated database snapshot is materialized instead. Invalid data never starts CPA.

Credential files are not duplicated into Gateway tables. Their durability follows CPA's configured `auth-dir` or CPA-managed Git, PostgreSQL, or S3 storage. Gateway must not log or expose their contents outside the authenticated download response.

## Lifecycle And Provider Availability

The lifecycle state machine is:

```text
stopped -> starting -> running -> stopping -> stopped
                \-> error
```

Start performs these steps:

1. Reject concurrent lifecycle mutations.
2. Load and validate the complete configuration.
3. Ensure the configured authentication directory exists when local file storage is used.
4. Generate the runtime management password.
5. Start CPA with loopback forced and the local management password installed.
6. Poll `/healthz` until ready or until a bounded timeout expires.
7. Publish the proxy target atomically.
8. Upsert and synchronize the embedded provider.

Stop first prevents new management requests, waits for in-flight proxy requests within a bounded drain period, then cancels CPA and waits for shutdown. The embedded provider is removed from route selection while CPA is unavailable. The provider's operator-controlled priority, weight, and desired enabled status are preserved and restored on start.

Restart is a serialized stop followed by start. Configuration changes to the internal port are applied on restart, and the management proxy reads its target from the manager rather than caching a port.

Successful non-read management mutations schedule a debounced provider sync. Successful OAuth completion also schedules a sync. Multiple operations in a short interval collapse into one sync to avoid repeated model and route rebuilds.

## Proxy Semantics

The management bridge supports the complete CPA route surface rather than enumerating individual management endpoints. It forwards:

- all HTTP methods registered by CPA;
- JSON and YAML configuration bodies;
- multipart credential and plugin uploads;
- file and log downloads;
- query parameters such as auth file names, indexes, and usage counts;
- response status and content headers;
- CPA version, build, and plugin-support headers used by the official panel.

The proxy applies bounded transport timeouts appropriate to normal calls while allowing downloads and long-running management operations to stream. It does not buffer credential uploads or downloads into logs. Configuration persistence hooks may buffer only the small management response needed to decide whether the mutation succeeded.

Future endpoints added within `/v0/management` work without new Gateway routes, subject to the pinned panel's ability to display them.

## Errors And Audit

Gateway-generated errors use JSON and stable codes:

- `401` or `403`: Gateway session or Root authorization failure;
- `409`: lifecycle transition conflict;
- `503`: CPA disabled, stopped, starting, or otherwise unavailable;
- `502`: connection failure to the embedded CPA instance;
- `504`: upstream management timeout;
- `500` with `persistence_failed`: runtime change applied but snapshot persistence failed.

Normal CPA business responses, including `4xx`, `5xx`, validation messages, and download headers, pass through unchanged.

Audit entries contain the Gateway Root user, method, normalized management path, result status, duration, and lifecycle action. They exclude request and response bodies, query values known to contain secrets, API keys, authorization headers, OAuth codes, tokens, and credential file content.

## Testing

### Backend Unit Tests

- lifecycle state transitions and concurrent start/stop/restart rejection;
- secure password generation and absence from API responses;
- Root and no-token authorization on panel, lifecycle, and proxy routes;
- stripping browser credentials and injecting the runtime password;
- query, body, content-type, status, header, upload, and download preservation;
- offline, transport, timeout, and persistence error mapping;
- full YAML migration, validation, atomic materialization, and disk recovery;
- same-origin checks for mutating methods;
- exact OAuth callback allowlist and state rejection;
- provider availability preservation and debounced synchronization.

### CPA Integration Tests

Run a real embedded CPA on a free loopback port and verify:

- management routes accept the runtime password;
- configuration mutations hot reload;
- full configuration survives stop and restart;
- authentication file upload, list, status patch, field patch, download, and delete;
- quota reset forwarding for a known runtime auth index;
- CPA stop makes the management bridge unavailable;
- restart updates the proxy target and provider connection details.

Where a real provider account would be required, use CPA's handler state or an upstream test server rather than external credentials.

### Frontend Tests

- CPA navigation is visible only to Root users;
- lifecycle state, buttons, busy states, and errors render correctly;
- the iframe is mounted only while CPA is running;
- placeholder panel connection data is prepared without a real management secret;
- the offline page can start CPA;
- desktop and mobile layouts do not overflow or overlap.

### Browser Verification

Use Playwright against a running Gateway and embedded CPA to verify that the pinned panel renders nonblank, automatically reaches its dashboard without a second login, performs representative management reads and writes, and remains usable at desktop and mobile viewport sizes. Inspect browser storage and captured requests to confirm the real management password is absent.

## Acceptance Criteria

1. A Root user opens CPA Management and reaches the official dashboard without entering another password.
2. The Root user can use every management feature supported by CPA v7.2.80, including configuration, credential import/download/edit/disable/delete, OAuth, quota inspection/reset, logs, API keys, providers, and plugins.
3. Full configuration changes survive Gateway and CPA restart.
4. Credential and OAuth changes trigger Gateway provider/model synchronization.
5. Stopped CPA instances do not receive management traffic or participate in route selection.
6. Admin users, ordinary users, user tokens, cross-origin mutations, and arbitrary CPA root paths cannot reach management functionality.
7. CPA remains loopback-only and the runtime management password never reaches browser storage, browser requests, logs, or API responses.
8. The pinned official panel works offline and changes only when Gateway intentionally upgrades it with CPA.
