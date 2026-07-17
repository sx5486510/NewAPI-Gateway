# Auth Files Bulk Upload Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add multi-file and zip upload support for `/v0/management/auth-files`, skipping duplicates while continuing to import non-duplicate auth files.

**Architecture:** The Gateway management proxy will intercept multipart auth-file uploads, expand zip archives server-side, check all candidate file names against the existing CPA auth-file list, then forward only non-duplicates as individual multipart uploads to CPA. The React management UI will allow selecting multiple `.json` and `.zip` files, warn about selected duplicates already visible in the list, and continue uploading the remaining files.

**Tech Stack:** Go `net/http`, `mime/multipart`, `archive/zip`; React 18 component tests with Jest.

---

### Task 1: Backend Bulk Upload Tests

**Files:**
- Modify: `service/cpa/management_proxy_test.go`

- [ ] **Step 1: Write failing tests**

Add tests that post multipart requests containing multiple `file` parts and a zip archive. The upstream fixture should return an existing auth-file list and record forwarded POST filenames. Expected behavior: existing names are skipped, new names are forwarded, and the response reports uploaded and duplicate names.

- [ ] **Step 2: Run backend tests to verify failure**

Run: `go test ./service/cpa -run "TestManagementProxy.*AuthFile.*Upload" -count=1`

Expected: new tests fail because the proxy currently only checks the first multipart file and forwards the original request unchanged.

### Task 2: Backend Bulk Upload Implementation

**Files:**
- Modify: `service/cpa/management_proxy.go`

- [ ] **Step 1: Replace single-name duplicate guard with bulk handler**

Change `ServeHTTP` so POST `/auth-files` multipart requests are handled before the reverse proxy. Parse all `file` parts, expand `.zip` files using `archive/zip`, keep `.json` entries, fetch existing auth-file names once, skip duplicates, and forward new files one at a time with multipart field name `file`.

- [ ] **Step 2: Return structured partial-success response**

Return HTTP 200 when at least request parsing succeeds, with JSON containing `success`, `uploaded`, `duplicates`, and counts. Return 400 for malformed multipart/zip and 502 for upstream upload failures.

- [ ] **Step 3: Run backend tests to verify green**

Run: `go test ./service/cpa -run "TestManagementProxy.*AuthFile.*Upload" -count=1`

Expected: all targeted backend upload tests pass.

### Task 3: Frontend Multi-File Selection Tests

**Files:**
- Modify: `web/src/components/CPAAuthFiles.test.js`

- [ ] **Step 1: Write failing test**

Add a test that opens the upload modal, selects one duplicate visible in `authFiles` and one new file, clicks upload, and asserts that the duplicate warning is shown while `FormData` sent to `API.post` contains only the new file.

- [ ] **Step 2: Run frontend test to verify failure**

Run: `npm test -- CPAAuthFiles.test.js --watchAll=false` from `web`.

Expected: the new test fails because the component stores only one selected file and blocks upload if that file is duplicate.

### Task 4: Frontend Multi-File Implementation

**Files:**
- Modify: `web/src/components/CPAAuthFiles.js`

- [ ] **Step 1: Store selected upload files as an array**

Replace `uploadFile` state with `uploadFiles`, set it from `Array.from(e.target.files || [])`, and configure the input with `multiple` and `accept=".json,.zip,application/json,application/zip"`.

- [ ] **Step 2: Skip duplicates and upload remaining files**

In `handleUpload`, compare selected names to `authFiles`, show a duplicate warning if any selected name exists, append only non-duplicate files to `FormData`, and continue POST when at least one file remains.

- [ ] **Step 3: Update success message for partial results**

Use the backend response `uploaded` and `duplicates` fields when present to show a useful result while keeping existing error handling intact.

- [ ] **Step 4: Run frontend tests**

Run: `npm test -- CPAAuthFiles.test.js --watchAll=false` from `web`.

Expected: component tests pass.

### Task 5: Final Verification

**Files:**
- No additional files.

- [ ] **Step 1: Run targeted Go tests**

Run: `go test ./service/cpa -count=1`

Expected: pass.

- [ ] **Step 2: Run targeted frontend tests**

Run: `npm test -- CPAAuthFiles.test.js --watchAll=false` from `web`.

Expected: pass.
