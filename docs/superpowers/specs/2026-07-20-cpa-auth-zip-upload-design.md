# CPA Authentication ZIP Upload Design

## Goal

Allow the Gateway CPA authentication-file page to upload JSON files directly or import JSON authentication files from ZIP archives. ZIP traversal is recursive and non-JSON entries are ignored.

## Existing Behavior

The React upload input currently accepts only `.json`, although the Gateway management proxy already contains partial ZIP expansion support. The server-side implementation walks ZIP entries and filters JSON files, but an archive containing no JSON files falls through to the transparent CPA proxy. It also reads archive entries without decompression limits.

## User Experience

- The upload input accepts multiple `.json` and `.zip` files in one selection.
- The help text states that ZIP subdirectories are scanned recursively and only JSON files are imported.
- Selected ZIP files are displayed in the existing selected-file list.
- The existing upload summary reports imported files and duplicate names.
- An empty ZIP, a ZIP with no JSON files, a damaged ZIP, or a ZIP exceeding a safety limit returns a clear upload error and is never forwarded directly to CPA.

## Server Processing

The Gateway remains the ZIP-processing boundary. CPA continues receiving its existing official `POST /v0/management/auth-files` multipart requests, one JSON file at a time.

For every multipart `file` part:

1. A `.json` file is queued unchanged.
2. A `.zip` file is opened and every archive entry is visited, including entries in nested directories.
3. Directory entries and non-JSON files are ignored.
4. JSON entry paths are flattened to their base file names because CPA authentication files use a flat namespace.
5. Existing CPA file names and names already encountered in the same request are treated as duplicates. The first occurrence is imported and later occurrences are reported as duplicates.
6. Other top-level file types are rejected rather than forwarded to CPA.

If parsing succeeds but produces no JSON candidates, Gateway returns HTTP 400 with a stable error code and descriptive message.

## Safety Limits

- At most 10,000 JSON candidates may be imported in one request.
- Each extracted JSON file may contain at most 8 MiB.
- The combined uncompressed JSON content from ZIP archives may contain at most 64 MiB.
- Reads stop as soon as a limit is exceeded, so oversized content is not fully allocated before rejection.

These limits apply to ZIP expansion. Existing direct JSON upload compatibility remains unchanged unless the request contains unsupported file types.

## Error Handling

- Malformed multipart data, damaged ZIP files, unsupported top-level file types, empty/no-JSON ZIP files, and limit violations return HTTP 400.
- Existing-name conflicts continue using the current partial-success behavior.
- CPA list or upload failures retain the current Gateway 502 behavior.
- No archive content or credential values are written to logs or response messages. Errors may identify archive and entry names only.

## Testing

Backend tests cover:

- Recursive JSON discovery in nested ZIP directories.
- Ignoring non-JSON ZIP entries.
- Flattened-name duplicate handling across directories.
- Empty/no-JSON and damaged ZIP rejection without CPA upload forwarding.
- The 10,000-file count limit.
- Per-file and total uncompressed-size limits.
- Rejection of unsupported top-level file types.

Frontend tests cover:

- The file input accepts JSON and ZIP selections.
- The upload help text describes recursive JSON-only ZIP import.
- ZIP files are included in the existing multipart request.

## Scope

This change does not add a new Gateway or CPA API. It extends the existing Gateway handling of CPA's official authentication-file upload endpoint and preserves the current duplicate and persistence flow.
