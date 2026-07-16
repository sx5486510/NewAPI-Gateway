# Grok 4.5 System Prompt Configuration Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Create the complete Grok 4.5 ENI LIME system prompt through the existing administrator interface and bind it to the current `grok-4.5` route.

**Architecture:** Treat this as authenticated runtime configuration rather than a code or migration change. Use the existing web pages and session-authenticated APIs to persist the verbatim source document and route binding, then independently verify the resulting SQLite data and run the existing focused tests without sending a request to Grok.

**Tech Stack:** React administrator UI, Gin session authentication, existing system prompt and route APIs, SQLite CLI, Go tests, PowerShell.

---

## File Map

- Read: `Jailbreak-Guide/Grok/Grok 4.5/ENI LIME.md` - immutable prompt source.
- Read: `gateway-aggregator.db` - independent post-configuration verification.
- Use: `web/src/pages/SystemPrompt/index.js` - existing prompt creation page at `/system-prompts`.
- Use: `web/src/components/ModelRoutesTable.js` - existing route binding page at `/routes`.
- Test: `model/system_prompt_test.go` - persistence and route-binding behavior.
- Test: `controller/system_prompt_test.go` and `controller/route_test.go` - authenticated API behavior.
- Test: `service/system_prompt_injection_test.go` and `service/proxy_system_prompt_test.go` - injection and proxy behavior.

No application source file is created or modified by this plan.

### Task 1: Verify The Source And Runtime Target

**Files:**
- Read: `Jailbreak-Guide/Grok/Grok 4.5/ENI LIME.md`
- Read: `gateway-aggregator.db`

- [ ] **Step 1: Verify the source content identity**

Run:

```powershell
$source = 'Jailbreak-Guide\Grok\Grok 4.5\ENI LIME.md'
$item = Get-Item -LiteralPath $source
$hash = (Get-FileHash -Algorithm SHA256 -LiteralPath $source).Hash.ToLowerInvariant()
$characters = (Get-Content -Raw -Encoding UTF8 -LiteralPath $source).Length
[pscustomobject]@{Bytes=$item.Length; Characters=$characters; SHA256=$hash}
```

Expected:

```text
Bytes      36000
Characters 35828
SHA256     799c86295a336a93dc56b8180fb9960c63db9954584d1ea0d46d052980ec2008
```

- [ ] **Step 2: Verify the gateway is healthy**

Run:

```powershell
Invoke-RestMethod -Uri 'http://localhost:3030/api/status' -Method Get -TimeoutSec 5
```

Expected: a response with `success: true`.

- [ ] **Step 3: Verify the target route and absence of a conflicting preset**

Run:

```powershell
sqlite3 -header -column gateway-aggregator.db "SELECT id, model_name, enabled, system_prompt_id FROM model_routes WHERE id = 679029; SELECT id, name, model_name FROM system_prompts WHERE model_name = 'grok-4.5' AND name = 'ENI LIME (Grok 4.5)';"
```

Expected: route `679029` has model name `grok-4.5` and no preset row with the selected name exists. If the preset already exists, stop before creation and inspect whether its content matches the expected hash.

### Task 2: Create The Prompt Through The Administrator UI

**Files:**
- Read: `Jailbreak-Guide/Grok/Grok 4.5/ENI LIME.md`
- Use: `web/src/pages/SystemPrompt/index.js`

- [ ] **Step 1: Stage the exact source content on the clipboard**

Run:

```powershell
Get-Content -Raw -Encoding UTF8 -LiteralPath 'Jailbreak-Guide\Grok\Grok 4.5\ENI LIME.md' | Set-Clipboard
```

Expected: the clipboard contains the complete 35,828-character document without added wrapper text. The browser textarea will normalize its 354 CRLF line endings to LF when saving.

- [ ] **Step 2: Open the administrator prompt page**

Run:

```powershell
Start-Process 'http://localhost:3030/system-prompts'
```

Expected: the system prompt page opens. If redirected, sign in through `/login` using the normal administrator account; do not expose credentials or session cookies outside the browser.

- [ ] **Step 3: Create the preset**

In the `新建提示词` modal, enter these exact values:

```text
名称: ENI LIME (Grok 4.5)
模型: grok-4.5
提示词内容: paste the staged clipboard content once
```

Select `创建` exactly once.

Expected: the UI reports `系统提示词已创建`, and filtering by model `grok-4.5` and name `ENI LIME (Grok 4.5)` shows one row.

### Task 3: Bind The Prompt To The Grok 4.5 Route

**Files:**
- Use: `web/src/components/ModelRoutesTable.js`

- [ ] **Step 1: Open the route page**

Run:

```powershell
Start-Process 'http://localhost:3030/routes'
```

Expected: the administrator route table opens in the same authenticated browser session.

- [ ] **Step 2: Select the exact route and prompt**

Search for `grok-4.5`, open the `grok-4.5` model entry, and locate route ID `679029`. In its `系统提示词` selector, choose `ENI LIME (Grok 4.5)`.

Expected: the row is marked as having an unsaved change and no prompt belonging to a different model appears in the selector.

- [ ] **Step 3: Persist the route binding**

Select `保存变更` once.

Expected: the UI reports that one route change was saved. Reload `/routes`, search for `grok-4.5`, and confirm the selected prompt remains `ENI LIME (Grok 4.5)`.

### Task 4: Verify Persisted Content And Binding Independently

**Files:**
- Read: `gateway-aggregator.db`
- Read: `Jailbreak-Guide/Grok/Grok 4.5/ENI LIME.md`

- [ ] **Step 1: Verify the stored record and route reference**

Run:

```powershell
sqlite3 -header -column gateway-aggregator.db "SELECT sp.id, sp.name, sp.model_name, length(sp.content) AS characters, length(CAST(sp.content AS BLOB)) AS bytes, COUNT(mr.id) AS route_count FROM system_prompts sp LEFT JOIN model_routes mr ON mr.system_prompt_id = sp.id WHERE sp.model_name = 'grok-4.5' AND sp.name = 'ENI LIME (Grok 4.5)' GROUP BY sp.id; SELECT mr.id, mr.model_name, mr.system_prompt_id, sp.name AS prompt_name FROM model_routes mr LEFT JOIN system_prompts sp ON sp.id = mr.system_prompt_id WHERE mr.id = 679029;"
```

Expected: exactly one preset row reports 35,474 characters, 35,646 bytes, and route count `1`; route `679029` references that preset ID. These values reflect the browser's CRLF-to-LF normalization.

- [ ] **Step 2: Verify the stored bytes have the source SHA-256**

Run:

```powershell
$hex = sqlite3 gateway-aggregator.db "SELECT hex(content) FROM system_prompts WHERE model_name = 'grok-4.5' AND name = 'ENI LIME (Grok 4.5)';"
if ([string]::IsNullOrWhiteSpace($hex)) { throw 'stored prompt not found' }
$bytes = New-Object byte[] ($hex.Length / 2)
for ($i = 0; $i -lt $bytes.Length; $i++) {
    $bytes[$i] = [Convert]::ToByte($hex.Substring($i * 2, 2), 16)
}
$sha = [BitConverter]::ToString(([Security.Cryptography.SHA256]::Create()).ComputeHash($bytes)).Replace('-', '').ToLowerInvariant()
if ($sha -ne '6267e3c9b6c4f71a3da90c403930d8b8ae58e1ee7efc4505b85415d395892de7') { throw "stored prompt hash mismatch: $sha" }
$sha
```

Expected:

```text
6267e3c9b6c4f71a3da90c403930d8b8ae58e1ee7efc4505b85415d395892de7
```

### Task 5: Run Focused Regression Verification

**Files:**
- Test: `model/system_prompt_test.go`
- Test: `controller/system_prompt_test.go`
- Test: `controller/route_test.go`
- Test: `service/system_prompt_injection_test.go`
- Test: `service/proxy_system_prompt_test.go`

- [ ] **Step 1: Run the focused Go tests without cache**

Run:

```powershell
go test -count=1 ./model ./controller ./service -run 'SystemPrompt|RouteSystemPrompt|ProxyRouteSystemPrompt'
```

Expected: all three packages report `ok` and the command exits with status 0.

- [ ] **Step 2: Confirm this configuration did not alter application source**

Run:

```powershell
git status --short
```

Expected: no source-code or migration change caused by this configuration. Pre-existing unrelated untracked files, including `Jailbreak-Guide/`, may remain.

### Rollback

If verification fails, return to `/routes`, clear the `系统提示词` selector for route `679029`, and select `保存变更`. Then return to `/system-prompts` and delete `ENI LIME (Grok 4.5)`. Do not use `?unbind=true` unless the UI reports that another route still references the preset.
