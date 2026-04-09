# Cooldown Classification Update Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Align cooldown classification and Retry-After handling with the approved rules: all 4xx default to token cooldown, `model_not_found`/`permission_denied` become unsupported-model exceptions, and Retry-After extends local cooldown duration.

**Architecture:** Keep `service/proxy.go` responsible for classifying upstream failures and keep `common/route_cooldown.go` responsible for cooldown state and duration calculation. Extend cooldown recording APIs to accept an optional minimum cooldown duration and update proxy-side classification helpers to apply the new 4xx-default/token-cooldown rule with unsupported-model exceptions.

**Tech Stack:** Go, Gin, standard library `net/http`, existing in-memory cooldown manager and Go test framework.

---

## File map

- Modify: `common/route_cooldown.go`
  - Extend route/token failure recording APIs to accept a minimum cooldown duration derived from Retry-After.
  - Keep decay, half-open, unsupported-model TTL, and backoff logic in this file.
- Modify: `common/route_cooldown_test.go`
  - Add unit tests for minimum cooldown duration behavior on route/token cooldowns.
- Modify: `service/proxy.go`
  - Update upstream error classification helpers so unsupported-model exceptions override the default 4xx-token-cooldown rule.
  - Feed Retry-After into route/token cooldown recording.
- Create: `service/proxy_cooldown_classify_test.go`
  - Add focused tests for the final classification rules and Retry-After parsing/classification interactions.

---

### Task 1: Extend cooldown manager APIs for Retry-After-aware durations

**Files:**
- Modify: `common/route_cooldown.go`
- Test: `common/route_cooldown_test.go`

- [ ] **Step 1: Write the failing route cooldown duration test**

Add this test near the end of `common/route_cooldown_test.go`:

```go
func TestRouteCooldownManager_RouteFailureRespectsMinimumCooldown(t *testing.T) {
	clock := time.Date(2026, 4, 9, 0, 0, 0, 0, time.UTC)
	nowFn := func() time.Time { return clock }

	cfgFn := func() RouteCooldownConfig {
		return RouteCooldownConfig{
			Enabled:               true,
			BaseSeconds:           30,
			Multiplier:            2,
			MaxSeconds:            1800,
			DecayMinutes:          10,
			JitterRatio:           0,
			HalfOpenMaxInFlight:   1,
			UnsupportedModelHours: 24,
			TokenBaseSeconds:      600,
			TokenMaxSeconds:       7200,
		}
	}

	mgr := newRouteCooldownManager(cfgFn, nowFn, func() float64 { return 0.5 })
	mgr.RecordRouteFailureWithMinimum(10, "gpt-5", 90)

	clock = clock.Add(89 * time.Second)
	if mgr.IsRouteSelectable(10, "gpt-5") {
		t.Fatalf("expected route cooldown to respect minimum duration")
	}

	clock = clock.Add(1 * time.Second)
	if !mgr.IsRouteSelectable(10, "gpt-5") {
		t.Fatalf("expected route selectable after minimum duration expires")
	}
}
```

- [ ] **Step 2: Write the failing token cooldown duration test**

Add this test below the prior one in `common/route_cooldown_test.go`:

```go
func TestRouteCooldownManager_TokenFailureRespectsMinimumCooldown(t *testing.T) {
	clock := time.Date(2026, 4, 9, 0, 0, 0, 0, time.UTC)
	nowFn := func() time.Time { return clock }

	cfgFn := func() RouteCooldownConfig {
		return RouteCooldownConfig{
			Enabled:               true,
			BaseSeconds:           30,
			Multiplier:            2,
			MaxSeconds:            1800,
			DecayMinutes:          10,
			JitterRatio:           0,
			HalfOpenMaxInFlight:   1,
			UnsupportedModelHours: 24,
			TokenBaseSeconds:      600,
			TokenMaxSeconds:       7200,
		}
	}

	mgr := newRouteCooldownManager(cfgFn, nowFn, func() float64 { return 0.5 })
	mgr.RecordTokenFailureWithMinimum(11, 1200)

	clock = clock.Add(1199 * time.Second)
	if mgr.IsRouteSelectable(11, "gpt-5") {
		t.Fatalf("expected token cooldown to respect minimum duration")
	}

	clock = clock.Add(1 * time.Second)
	if !mgr.IsRouteSelectable(11, "gpt-5") {
		t.Fatalf("expected token selectable after minimum duration expires")
	}
}
```

- [ ] **Step 3: Run the cooldown manager tests to verify they fail**

Run:

```bash
go test ./common -run 'TestRouteCooldownManager_(RouteFailureRespectsMinimumCooldown|TokenFailureRespectsMinimumCooldown)$'
```

Expected: FAIL with undefined method errors for `RecordRouteFailureWithMinimum` and `RecordTokenFailureWithMinimum`.

- [ ] **Step 4: Implement minimum-duration-aware failure recording**

Update `common/route_cooldown.go` by replacing the existing route/token failure methods with the following structure:

```go
func (m *RouteCooldownManager) RecordRouteFailure(providerTokenId int, modelName string) {
	m.RecordRouteFailureWithMinimum(providerTokenId, modelName, 0)
}

func (m *RouteCooldownManager) RecordRouteFailureWithMinimum(providerTokenId int, modelName string, minCooldownSeconds int) {
	cfg := m.configProvider()
	if !cfg.Enabled {
		return
	}
	key := routeCooldownKey{
		providerTokenId: providerTokenId,
		modelName:       normalizeCooldownModelName(modelName),
	}
	if key.modelName == "" {
		return
	}

	now := m.now()

	m.mu.Lock()
	defer m.mu.Unlock()

	state, ok := m.routeStates[key]
	if !ok {
		state = &routeCooldownState{}
		m.routeStates[key] = state
	} else {
		m.applyRouteDecayLocked(state, now, cfg)
	}

	if cfg.FailureWindowSeconds > 0 && !state.lastFailureTime.IsZero() {
		window := time.Duration(cfg.FailureWindowSeconds) * time.Second
		if window > 0 && now.Sub(state.lastFailureTime) > window {
			state.consecutiveFailures = 0
			state.halfOpenInFlight = 0
		}
	}

	state.consecutiveFailures++
	state.lastFailureTime = now
	state.halfOpenInFlight = 0

	minFailures := cfg.MinConsecutiveFailures
	if minFailures <= 0 {
		minFailures = 1
	}
	if state.consecutiveFailures < minFailures {
		state.cooldownUntil = now
		return
	}

	duration := computeCooldownDurationSeconds(
		cfg.BaseSeconds,
		cfg.Multiplier,
		cfg.MaxSeconds,
		state.consecutiveFailures,
		cfg.JitterRatio,
		m.randFloat64,
	)
	if minCooldownSeconds > duration {
		duration = minCooldownSeconds
	}
	state.cooldownUntil = now.Add(time.Duration(duration) * time.Second)
}

func (m *RouteCooldownManager) RecordTokenFailure(providerTokenId int) {
	m.RecordTokenFailureWithMinimum(providerTokenId, 0)
}

func (m *RouteCooldownManager) RecordTokenFailureWithMinimum(providerTokenId int, minCooldownSeconds int) {
	cfg := m.configProvider()
	if !cfg.Enabled {
		return
	}
	now := m.now()

	m.mu.Lock()
	defer m.mu.Unlock()

	state, ok := m.tokenStates[providerTokenId]
	if !ok {
		state = &tokenCooldownState{}
		m.tokenStates[providerTokenId] = state
	} else {
		m.applyTokenDecayLocked(state, now, cfg)
	}

	if cfg.FailureWindowSeconds > 0 && !state.lastFailureTime.IsZero() {
		window := time.Duration(cfg.FailureWindowSeconds) * time.Second
		if window > 0 && now.Sub(state.lastFailureTime) > window {
			state.consecutiveFailures = 0
		}
	}

	state.consecutiveFailures++
	state.lastFailureTime = now

	duration := computeCooldownDurationSeconds(
		cfg.TokenBaseSeconds,
		cfg.Multiplier,
		cfg.TokenMaxSeconds,
		state.consecutiveFailures,
		cfg.JitterRatio,
		m.randFloat64,
	)
	if minCooldownSeconds > duration {
		duration = minCooldownSeconds
	}
	state.cooldownUntil = now.Add(time.Duration(duration) * time.Second)
}
```

- [ ] **Step 5: Run the cooldown manager tests to verify they pass**

Run:

```bash
go test ./common -run 'TestRouteCooldownManager_(RouteFailureRespectsMinimumCooldown|TokenFailureRespectsMinimumCooldown|BasicCooldownAndHalfOpen|DecayClearsFailures|UnsupportedModelTTL|TokenCooldown)$'
```

Expected: PASS.

- [ ] **Step 6: Commit the cooldown manager API update**

```bash
git add common/route_cooldown.go common/route_cooldown_test.go
git commit -m "fix: apply minimum cooldown durations from upstream hints"
```

---

### Task 2: Update upstream error classification rules

**Files:**
- Modify: `service/proxy.go`
- Create: `service/proxy_cooldown_classify_test.go`

- [ ] **Step 1: Write the failing classification tests**

Create `service/proxy_cooldown_classify_test.go` with this content:

```go
package service

import "testing"

func TestShouldMarkUnsupportedModel_AcrossAll4xx(t *testing.T) {
	cases := []struct {
		name       string
		statusCode int
		err        upstreamErrorInfo
		want       bool
	}{
		{
			name:       "403 permission_denied code",
			statusCode: 403,
			err:        upstreamErrorInfo{Code: "permission_denied"},
			want:       true,
		},
		{
			name:       "401 model_not_found type",
			statusCode: 401,
			err:        upstreamErrorInfo{Type: "model_not_found"},
			want:       true,
		},
		{
			name:       "400 plain invalid request",
			statusCode: 400,
			err:        upstreamErrorInfo{Code: "invalid_request"},
			want:       false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := shouldMarkUnsupportedModel(tc.statusCode, tc.err)
			if got != tc.want {
				t.Fatalf("shouldMarkUnsupportedModel(%d, %+v) = %v, want %v", tc.statusCode, tc.err, got, tc.want)
			}
		})
	}
}

func TestShouldTriggerTokenCooldown_ForAll4xxExceptUnsupportedModel(t *testing.T) {
	cases := []struct {
		name       string
		statusCode int
		err        upstreamErrorInfo
		want       bool
	}{
		{
			name:       "400 invalid request still cools token",
			statusCode: 400,
			err:        upstreamErrorInfo{Code: "invalid_request"},
			want:       true,
		},
		{
			name:       "422 semantic error cools token",
			statusCode: 422,
			err:        upstreamErrorInfo{},
			want:       true,
		},
		{
			name:       "403 permission denied excluded by unsupported model",
			statusCode: 403,
			err:        upstreamErrorInfo{Code: "permission_denied"},
			want:       false,
		},
		{
			name:       "500 server error not token cooldown",
			statusCode: 500,
			err:        upstreamErrorInfo{},
			want:       false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := shouldTriggerTokenCooldown(tc.statusCode, tc.err)
			if got != tc.want {
				t.Fatalf("shouldTriggerTokenCooldown(%d, %+v) = %v, want %v", tc.statusCode, tc.err, got, tc.want)
			}
		})
	}
}
```

- [ ] **Step 2: Run the classification tests to verify they fail**

Run:

```bash
go test ./service -run 'TestShould(MarkUnsupportedModel_AcrossAll4xx|TriggerTokenCooldown_ForAll4xxExceptUnsupportedModel)$'
```

Expected: FAIL because the current helper behavior still treats unsupported-model detection as 400/404-only and does not default all 4xx to token cooldown.

- [ ] **Step 3: Update the proxy classification helpers**

In `service/proxy.go`, replace the three helper functions with the following implementations:

```go
func shouldMarkUnsupportedModel(statusCode int, upstreamErr upstreamErrorInfo) bool {
	if statusCode < 400 || statusCode >= 500 {
		return false
	}
	code := strings.ToLower(strings.TrimSpace(upstreamErr.Code))
	typ := strings.ToLower(strings.TrimSpace(upstreamErr.Type))
	msg := strings.ToLower(strings.TrimSpace(upstreamErr.Message))
	if strings.Contains(code, "model_not_found") || strings.Contains(typ, "model_not_found") {
		return true
	}
	if strings.Contains(code, "permission_denied") || strings.Contains(typ, "permission_denied") {
		return true
	}
	if strings.Contains(msg, "model not found") || strings.Contains(msg, "no such model") {
		return true
	}
	return false
}

func shouldTriggerTokenCooldown(statusCode int, upstreamErr upstreamErrorInfo) bool {
	if statusCode < 400 || statusCode >= 500 {
		return false
	}
	if shouldMarkUnsupportedModel(statusCode, upstreamErr) {
		return false
	}
	return true
}

func shouldTriggerRouteCooldown(statusCode int, upstreamErr upstreamErrorInfo) bool {
	if statusCode >= 500 {
		return true
	}
	if statusCode == 408 {
		return true
	}
	return false
}
```

Keep `isNonRetryableInvalidRequest()` unchanged for now; the approved design keeps this change focused on cooldown side effects rather than redesigning retry semantics.

- [ ] **Step 4: Run the classification tests to verify they pass**

Run:

```bash
go test ./service -run 'TestShould(MarkUnsupportedModel_AcrossAll4xx|TriggerTokenCooldown_ForAll4xxExceptUnsupportedModel)$'
```

Expected: PASS.

- [ ] **Step 5: Commit the classification update**

```bash
git add service/proxy.go service/proxy_cooldown_classify_test.go
git commit -m "fix: align cooldown classification with approved rules"
```

---

### Task 3: Feed Retry-After into route and token cooldown updates

**Files:**
- Modify: `service/proxy.go`
- Modify: `service/proxy_cooldown_classify_test.go`

- [ ] **Step 1: Write the failing Retry-After helper tests**

Append these tests to `service/proxy_cooldown_classify_test.go`:

```go
func TestParseRetryAfterSeconds(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want int
	}{
		{name: "empty", raw: "", want: 0},
		{name: "invalid", raw: "abc", want: 0},
		{name: "negative", raw: "-5", want: 0},
		{name: "seconds", raw: "120", want: 120},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := parseRetryAfterSeconds(tc.raw)
			if got != tc.want {
				t.Fatalf("parseRetryAfterSeconds(%q) = %d, want %d", tc.raw, got, tc.want)
			}
		})
	}
}
```

Append these tests to `common/route_cooldown_test.go`:

```go
func TestRouteCooldownManager_RouteFailurePrefersLargerMinimum(t *testing.T) {
	clock := time.Date(2026, 4, 9, 0, 0, 0, 0, time.UTC)
	nowFn := func() time.Time { return clock }

	cfgFn := func() RouteCooldownConfig {
		return RouteCooldownConfig{
			Enabled:               true,
			BaseSeconds:           30,
			Multiplier:            2,
			MaxSeconds:            1800,
			DecayMinutes:          10,
			JitterRatio:           0,
			HalfOpenMaxInFlight:   1,
			UnsupportedModelHours: 24,
			TokenBaseSeconds:      600,
			TokenMaxSeconds:       7200,
		}
	}

	mgr := newRouteCooldownManager(cfgFn, nowFn, func() float64 { return 0.5 })
	mgr.RecordRouteFailureWithMinimum(12, "gpt-5", 10)

	clock = clock.Add(29 * time.Second)
	if mgr.IsRouteSelectable(12, "gpt-5") {
		t.Fatalf("expected local route cooldown to win when larger than minimum")
	}
}

func TestRouteCooldownManager_TokenFailurePrefersLargerMinimum(t *testing.T) {
	clock := time.Date(2026, 4, 9, 0, 0, 0, 0, time.UTC)
	nowFn := func() time.Time { return clock }

	cfgFn := func() RouteCooldownConfig {
		return RouteCooldownConfig{
			Enabled:               true,
			BaseSeconds:           30,
			Multiplier:            2,
			MaxSeconds:            1800,
			DecayMinutes:          10,
			JitterRatio:           0,
			HalfOpenMaxInFlight:   1,
			UnsupportedModelHours: 24,
			TokenBaseSeconds:      600,
			TokenMaxSeconds:       7200,
		}
	}

	mgr := newRouteCooldownManager(cfgFn, nowFn, func() float64 { return 0.5 })
	mgr.RecordTokenFailureWithMinimum(13, 100)

	clock = clock.Add(599 * time.Second)
	if mgr.IsRouteSelectable(13, "gpt-5") {
		t.Fatalf("expected local token cooldown to win when larger than minimum")
	}
}
```

- [ ] **Step 2: Run the focused tests**

Run:

```bash
go test ./common -run 'TestRouteCooldownManager_(RouteFailurePrefersLargerMinimum|TokenFailurePrefersLargerMinimum)$' && go test ./service -run '^TestParseRetryAfterSeconds$'
```

Expected: PASS for parse helper and FAIL only if the earlier API changes were not applied correctly.

- [ ] **Step 3: Update proxy response handling to pass Retry-After into cooldown updates**

In `service/proxy.go`, update the error-response block inside `ProxyToUpstream()` to use the parsed Retry-After when recording cooldowns:

```go
		upstreamErr := extractUpstreamErrorInfo(respBody)
		retryAfterSeconds := parseRetryAfterSeconds(resp.Header.Get("Retry-After"))

		if shouldMarkUnsupportedModel(resp.StatusCode, upstreamErr) {
			common.GlobalRouteCooldown.MarkUnsupportedModel(token.Id, resolvedModel)
		}
		if shouldTriggerTokenCooldown(resp.StatusCode, upstreamErr) {
			common.GlobalRouteCooldown.RecordTokenFailureWithMinimum(token.Id, retryAfterSeconds)
		}
		if shouldTriggerRouteCooldown(resp.StatusCode, upstreamErr) {
			common.GlobalRouteCooldown.RecordRouteFailureWithMinimum(token.Id, resolvedModel, retryAfterSeconds)
		}
```

Do not change the `http.Client.Do` transport-error branch; it should keep calling `RecordRouteFailure(...)` without Retry-After because no upstream header exists there.

- [ ] **Step 4: Run all focused cooldown tests**

Run:

```bash
go test ./common -run 'TestRouteCooldownManager_(BasicCooldownAndHalfOpen|DecayClearsFailures|UnsupportedModelTTL|TokenCooldown|RouteFailureRespectsMinimumCooldown|TokenFailureRespectsMinimumCooldown|RouteFailurePrefersLargerMinimum|TokenFailurePrefersLargerMinimum)$' && go test ./service -run 'TestShould(MarkUnsupportedModel_AcrossAll4xx|TriggerTokenCooldown_ForAll4xxExceptUnsupportedModel)$|^TestParseRetryAfterSeconds$'
```

Expected: PASS.

- [ ] **Step 5: Run package tests for regression coverage**

Run:

```bash
go test ./common ./service
```

Expected: PASS.

- [ ] **Step 6: Commit the Retry-After integration**

```bash
git add common/route_cooldown.go common/route_cooldown_test.go service/proxy.go service/proxy_cooldown_classify_test.go
git commit -m "fix: honor retry-after in cooldown decisions"
```

---

## Spec coverage check

- Approved rule "all 4xx default to token cooldown" is covered by Task 2 helper updates and tests.
- Approved exception rule for `model_not_found` / `permission_denied` is covered by Task 2 tests and helper changes.
- Approved Retry-After lower-bound rule is covered by Task 1 API changes plus Task 3 integration/tests.
- Existing success semantics remain unchanged by explicit scope in Tasks 2 and 3.
- Existing route cooldown features remain covered by the regression test commands in Task 3.

## Placeholder scan

- No TODO/TBD placeholders remain.
- All file paths, code snippets, and commands are specified.
- Each task includes concrete tests and implementation snippets.

## Type consistency check

- New public helpers are consistently named `RecordRouteFailureWithMinimum` and `RecordTokenFailureWithMinimum` across all tasks.
- Existing helpers `shouldMarkUnsupportedModel`, `shouldTriggerTokenCooldown`, `shouldTriggerRouteCooldown`, and `parseRetryAfterSeconds` are referenced consistently.
