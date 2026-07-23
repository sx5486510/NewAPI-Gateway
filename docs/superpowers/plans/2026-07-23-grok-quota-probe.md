# Grok Quota Probe Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a read-only Grok quota probe script that reads one local Grok CLI credential and reports plan, quota, usage, remaining capacity, reset times, and source statuses.

**Architecture:** Keep parsing and formatting in `scripts/probe-grok-quota-lib.mjs` so it can be tested offline. Keep network and CLI orchestration in `scripts/probe-grok-quota.mjs`, which imports the library, reads one credential JSON, performs bounded GET requests, and prints a safe report without tokens.

**Tech Stack:** Node.js ESM, built-in `node:test`, built-in `fetch`.

---

### Task 1: Offline parser and formatter tests

**Files:**
- Create: `scripts/probe-grok-quota-lib.test.mjs`
- Later create: `scripts/probe-grok-quota-lib.mjs`

- [ ] **Step 1: Write failing tests**

Create `scripts/probe-grok-quota-lib.test.mjs` with tests that import `extractAccessToken`, `extractUserId`, `jwtTierName`, `buildBillingSummary`, and `formatQuotaReport`.

The tests must cover:
- extracting `access_token` and direct user id;
- extracting user id from JWT `sub`;
- mapping JWT tier `5` to `supergrok_heavy`;
- merging credits and billing payloads into weekly and monthly output;
- preserving missing fields as `n/a`.

- [ ] **Step 2: Run tests to verify they fail**

Run:

```powershell
node --test scripts/probe-grok-quota-lib.test.mjs
```

Expected: FAIL because `scripts/probe-grok-quota-lib.mjs` does not exist.

### Task 2: Parser and formatter implementation

**Files:**
- Create: `scripts/probe-grok-quota-lib.mjs`
- Test: `scripts/probe-grok-quota-lib.test.mjs`

- [ ] **Step 1: Implement minimal library**

Create `scripts/probe-grok-quota-lib.mjs` with:
- credential token/user extraction helpers;
- safe JWT payload decoding;
- tier mapping;
- response normalization and merging;
- USD and percentage formatting;
- `formatQuotaReport(summary)`.

- [ ] **Step 2: Run parser tests**

Run:

```powershell
node --test scripts/probe-grok-quota-lib.test.mjs
```

Expected: PASS.

### Task 3: CLI probe script

**Files:**
- Create: `scripts/probe-grok-quota.mjs`
- Modify: `scripts/probe-grok-quota-lib.test.mjs` only if CLI formatting needs a tested exported helper

- [ ] **Step 1: Create CLI wrapper**

Create `scripts/probe-grok-quota.mjs` that:
- accepts one credential path argument;
- reads and parses JSON;
- exits with usage when missing;
- extracts token and user id using the library;
- calls the three Grok endpoints with safe headers;
- merges results using the library;
- prints the report;
- exits nonzero when both billing endpoints fail or no quota summary is usable.

- [ ] **Step 2: Smoke syntax check**

Run:

```powershell
node --check scripts/probe-grok-quota.mjs
node --check scripts/probe-grok-quota-lib.mjs
```

Expected: both pass.

### Task 4: Verification with local credential

**Files:**
- No code changes unless verification exposes a script bug.

- [ ] **Step 1: Run all offline tests**

Run:

```powershell
node --test scripts/probe-grok-quota-lib.test.mjs
```

Expected: PASS.

- [ ] **Step 2: Run real probe**

Run:

```powershell
node scripts/probe-grok-quota.mjs "$env:USERPROFILE\.cli-proxy-api\xai-08acda2c201b41f0.json"
```

Expected: output shows source HTTP statuses and whichever plan/quota fields the upstream returns. It must not include the access token or refresh token.

- [ ] **Step 3: Final diff check**

Run:

```powershell
git diff --check -- scripts/probe-grok-quota.mjs scripts/probe-grok-quota-lib.mjs scripts/probe-grok-quota-lib.test.mjs
```

Expected: no output.

