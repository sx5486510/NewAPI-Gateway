package common

import (
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

func TestRouteCooldownManager_BasicCooldownAndHalfOpen(t *testing.T) {
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

	tokenID := 1
	modelName := "gpt-5.2"

	if !mgr.IsRouteSelectable(tokenID, modelName) {
		t.Fatalf("expected route selectable initially")
	}

	mgr.RecordRouteFailure(tokenID, modelName)

	if mgr.IsRouteSelectable(tokenID, modelName) {
		t.Fatalf("expected route NOT selectable during cooldown")
	}

	clock = clock.Add(29 * time.Second)
	if mgr.IsRouteSelectable(tokenID, modelName) {
		t.Fatalf("expected route NOT selectable before cooldown expiry")
	}

	clock = clock.Add(1 * time.Second)
	if !mgr.IsRouteSelectable(tokenID, modelName) {
		t.Fatalf("expected route selectable after cooldown expiry")
	}

	permit1, _, ok := mgr.TryAcquireRouteAttempt(tokenID, modelName)
	if !ok || permit1 == nil {
		t.Fatalf("expected first half-open acquire ok")
	}
	defer permit1.Release()

	_, _, ok = mgr.TryAcquireRouteAttempt(tokenID, modelName)
	if ok {
		t.Fatalf("expected second half-open acquire denied")
	}

	permit1.Release()

	permit2, _, ok := mgr.TryAcquireRouteAttempt(tokenID, modelName)
	if !ok || permit2 == nil {
		t.Fatalf("expected acquire ok after release")
	}
	permit2.Release()

	mgr.RecordRouteSuccess(tokenID, modelName)

	permit3, _, ok := mgr.TryAcquireRouteAttempt(tokenID, modelName)
	if !ok || permit3 == nil {
		t.Fatalf("expected acquire ok after success reset")
	}
	permit3.Release()
}

func TestRouteCooldownManager_DecayClearsFailures(t *testing.T) {
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

	tokenID := 2
	modelName := "gpt-5.2"

	mgr.RecordRouteFailure(tokenID, modelName) // failures=1, cooldown 30s
	clock = clock.Add(31 * time.Second)

	// No more requests for > 20 minutes: failures should decay to 0 and state should clear.
	clock = clock.Add(25 * time.Minute)

	if !mgr.IsRouteSelectable(tokenID, modelName) {
		t.Fatalf("expected route selectable after decay")
	}

	permit, _, ok := mgr.TryAcquireRouteAttempt(tokenID, modelName)
	if !ok {
		t.Fatalf("expected acquire ok after decay")
	}
	permit.Release()
}

func TestRouteCooldownManager_UnsupportedModelTTL(t *testing.T) {
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

	tokenID := 3
	modelName := "gpt-5.2"

	mgr.MarkUnsupportedModel(tokenID, modelName)

	if mgr.IsRouteSelectable(tokenID, modelName) {
		t.Fatalf("expected route NOT selectable when model is marked unsupported")
	}

	clock = clock.Add(24*time.Hour + time.Second)
	if !mgr.IsRouteSelectable(tokenID, modelName) {
		t.Fatalf("expected route selectable after unsupported TTL expiry")
	}
}

func TestRouteCooldownManager_TokenCooldown(t *testing.T) {
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

	tokenID := 4
	modelName := "gpt-5.2"

	mgr.RecordTokenFailure(tokenID)
	if mgr.IsRouteSelectable(tokenID, modelName) {
		t.Fatalf("expected route NOT selectable while token in cooldown")
	}

	clock = clock.Add(10 * time.Minute)
	if !mgr.IsRouteSelectable(tokenID, modelName) {
		t.Fatalf("expected route selectable after token cooldown expiry")
	}
}

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

func TestRouteCooldownManager_GetRouteCooldownStatusReturnsEmptyWhenDisabled(t *testing.T) {
	clock := time.Date(2026, 4, 9, 0, 0, 0, 0, time.UTC)
	nowFn := func() time.Time { return clock }
	enabled := true

	cfgFn := func() RouteCooldownConfig {
		return RouteCooldownConfig{
			Enabled:               enabled,
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
	mgr.RecordRouteFailure(21, "gpt-5")
	enabled = false

	status := mgr.GetRouteCooldownStatus(21, "gpt-5")
	if status == nil {
		t.Fatalf("expected status object")
	}
	if status.InCooldown || status.Reason != "" || status.RemainingSecs != 0 || status.HalfOpen || status.HalfOpenInflight != 0 {
		t.Fatalf("expected empty cooldown status when disabled, got %+v", *status)
	}
}

func TestRouteCooldownManager_RecordRouteFailureWithCausePopulatesFields(t *testing.T) {
	clock := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	mgr := newRouteCooldownManager(func() RouteCooldownConfig {
		return RouteCooldownConfig{Enabled: true, BaseSeconds: 30, Multiplier: 2, MaxSeconds: 1800, MinConsecutiveFailures: 1, UnsupportedModelHours: 24, TokenBaseSeconds: 600, TokenMaxSeconds: 7200}
	}, func() time.Time { return clock }, func() float64 { return 0.5 })
	mgr.RecordRouteFailureWithMinimumAndCause(42, "gpt-5", 0, CooldownCause{HTTPStatus: 503, ReasonCode: CooldownReasonUpstream5xx, ReasonMessage: "upstream temporarily unavailable", TriggeredAt: clock})
	status := mgr.GetRouteCooldownStatus(42, "gpt-5")
	if !status.InCooldown || status.CauseHTTPStatus != 503 || status.CauseReasonCode != CooldownReasonUpstream5xx || status.CauseMessage != "upstream temporarily unavailable" || status.TriggerCount != 1 {
		t.Fatalf("unexpected status: %+v", status)
	}
}

func TestRouteCooldownManager_LegacyFailureKeepsCauseFieldsEmpty(t *testing.T) {
	clock := time.Now()
	mgr := newRouteCooldownManager(func() RouteCooldownConfig {
		return RouteCooldownConfig{Enabled: true, BaseSeconds: 1, UnsupportedModelHours: 1}
	}, func() time.Time { return clock }, nil)
	mgr.RecordRouteFailure(1, "m")
	status := mgr.GetRouteCooldownStatus(1, "m")
	if status.CauseHTTPStatus != 0 || status.CauseReasonCode != "" || status.CauseMessage != "" || status.CauseTriggeredAt != 0 || status.TriggerCount != 0 {
		t.Fatalf("legacy call unexpectedly populated cause: %+v", status)
	}
}

func TestRouteCooldownManager_CauseOnlyAfterThreshold(t *testing.T) {
	clock := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	mgr := newRouteCooldownManager(func() RouteCooldownConfig {
		return RouteCooldownConfig{Enabled: true, BaseSeconds: 30, Multiplier: 2, MaxSeconds: 1800, MinConsecutiveFailures: 3, UnsupportedModelHours: 24, TokenBaseSeconds: 600, TokenMaxSeconds: 7200}
	}, func() time.Time { return clock }, func() float64 { return 0.5 })
	cause := CooldownCause{HTTPStatus: 500, ReasonCode: CooldownReasonUpstream5xx, ReasonMessage: "boom", TriggeredAt: clock}
	mgr.RecordRouteFailureWithMinimumAndCause(1, "m", 0, cause)
	mgr.RecordRouteFailureWithMinimumAndCause(1, "m", 0, cause)
	status := mgr.GetRouteCooldownStatus(1, "m")
	if status.InCooldown || status.CauseReasonCode != "" {
		t.Fatalf("cause should not be visible before threshold: %+v", status)
	}
	mgr.RecordRouteFailureWithMinimumAndCause(1, "m", 0, cause)
	status = mgr.GetRouteCooldownStatus(1, "m")
	if !status.InCooldown || status.CauseReasonCode != CooldownReasonUpstream5xx || status.TriggerCount != 3 {
		t.Fatalf("unexpected threshold status: %+v", status)
	}
}

func TestRouteCooldownManager_CauseMessageIsUTF8SafeAndBounded(t *testing.T) {
	clock := time.Now()
	mgr := newRouteCooldownManager(func() RouteCooldownConfig {
		return RouteCooldownConfig{Enabled: true, BaseSeconds: 1, UnsupportedModelHours: 1}
	}, func() time.Time { return clock }, nil)
	mgr.RecordRouteFailureWithMinimumAndCause(1, "m", 0, CooldownCause{ReasonMessage: strings.Repeat("界", 300)})
	status := mgr.GetRouteCooldownStatus(1, "m")
	if !utf8.ValidString(status.CauseMessage) || len([]rune(status.CauseMessage)) != CoolDownCauseMessageLimit()+1 {
		t.Fatalf("invalid or unexpected message: %q", status.CauseMessage)
	}
}
