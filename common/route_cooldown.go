package common

import (
	"math"
	"math/rand"
	"strconv"
	"strings"
	"sync"
	"time"
)

type RouteCooldownConfig struct {
	Enabled bool

	BaseSeconds  int
	Multiplier   float64
	MaxSeconds   int
	DecayMinutes int
	JitterRatio  float64

	MinConsecutiveFailures int
	FailureWindowSeconds   int

	HalfOpenMaxInFlight int

	UnsupportedModelHours int

	TokenBaseSeconds int
	TokenMaxSeconds  int
}

type routeCooldownKey struct {
	providerTokenId int
	modelName       string
}

type routeCooldownState struct {
	consecutiveFailures int
	cooldownUntil       time.Time
	lastFailureTime     time.Time

	halfOpenInFlight int
}

type tokenCooldownState struct {
	consecutiveFailures int
	cooldownUntil       time.Time
	lastFailureTime     time.Time
}

type RouteCooldownPermit struct {
	manager *RouteCooldownManager
	key     routeCooldownKey
	halfOpen bool
	released bool
}

func (p *RouteCooldownPermit) Release() {
	if p == nil || p.manager == nil {
		return
	}
	p.manager.releasePermit(p)
}

type RouteCooldownManager struct {
	mu sync.Mutex

	routeStates map[routeCooldownKey]*routeCooldownState
	tokenStates map[int]*tokenCooldownState
	unsupported map[routeCooldownKey]time.Time

	configProvider func() RouteCooldownConfig
	now            func() time.Time
	randFloat64    func() float64
}

var GlobalRouteCooldown = NewRouteCooldownManager(LoadRouteCooldownConfig)

func NewRouteCooldownManager(configProvider func() RouteCooldownConfig) *RouteCooldownManager {
	return newRouteCooldownManager(configProvider, time.Now, rand.Float64)
}

func newRouteCooldownManager(configProvider func() RouteCooldownConfig, nowFn func() time.Time, randFn func() float64) *RouteCooldownManager {
	if configProvider == nil {
		configProvider = func() RouteCooldownConfig { return RouteCooldownConfig{Enabled: false} }
	}
	if nowFn == nil {
		nowFn = time.Now
	}
	if randFn == nil {
		randFn = rand.Float64
	}
	return &RouteCooldownManager{
		routeStates:     make(map[routeCooldownKey]*routeCooldownState),
		tokenStates:     make(map[int]*tokenCooldownState),
		unsupported:     make(map[routeCooldownKey]time.Time),
		configProvider:  configProvider,
		now:             nowFn,
		randFloat64:     randFn,
	}
}

func (m *RouteCooldownManager) IsRouteSelectable(providerTokenId int, modelName string) bool {
	cfg := m.configProvider()
	if !cfg.Enabled {
		return true
	}
	key := routeCooldownKey{
		providerTokenId: providerTokenId,
		modelName:       normalizeCooldownModelName(modelName),
	}
	if key.modelName == "" {
		return true
	}

	now := m.now()

	m.mu.Lock()
	defer m.mu.Unlock()

	if m.isUnsupportedLocked(key, now) {
		return false
	}

	if m.isTokenInCooldownLocked(providerTokenId, now, cfg) {
		return false
	}

	if m.isRouteInCooldownLocked(key, now, cfg) {
		return false
	}

	return true
}

func (m *RouteCooldownManager) TryAcquireRouteAttempt(providerTokenId int, modelName string) (*RouteCooldownPermit, time.Duration, bool) {
	cfg := m.configProvider()
	if !cfg.Enabled {
		return &RouteCooldownPermit{}, 0, true
	}
	key := routeCooldownKey{
		providerTokenId: providerTokenId,
		modelName:       normalizeCooldownModelName(modelName),
	}
	if key.modelName == "" {
		return &RouteCooldownPermit{}, 0, true
	}

	now := m.now()

	m.mu.Lock()
	defer m.mu.Unlock()

	if until, ok := m.unsupported[key]; ok {
		if !now.Before(until) {
			delete(m.unsupported, key)
		} else {
			return nil, until.Sub(now), false
		}
	}

	if state, ok := m.tokenStates[providerTokenId]; ok {
		m.applyTokenDecayLocked(state, now, cfg)
		if now.Before(state.cooldownUntil) {
			return nil, state.cooldownUntil.Sub(now), false
		}
		if state.consecutiveFailures <= 0 && !now.Before(state.cooldownUntil) {
			delete(m.tokenStates, providerTokenId)
		}
	}

	routeState, ok := m.routeStates[key]
	if ok {
		m.applyRouteDecayLocked(routeState, now, cfg)
		if now.Before(routeState.cooldownUntil) {
			return nil, routeState.cooldownUntil.Sub(now), false
		}

		// At this point, cooldown has already expired (now >= cooldownUntil).
		// Half-open gating applies only after a real cooldown was triggered:
		// when consecutiveFailures < MinConsecutiveFailures, we intentionally set cooldownUntil == lastFailureTime
		// to avoid treating the route as half-open yet.
		halfOpen := routeState.consecutiveFailures > 0 && routeState.cooldownUntil.After(routeState.lastFailureTime)

		if halfOpen {
			limit := cfg.HalfOpenMaxInFlight
			if limit <= 0 {
				limit = 1
			}
			if routeState.halfOpenInFlight >= limit {
				return nil, 0, false
			}
			routeState.halfOpenInFlight++
			return &RouteCooldownPermit{manager: m, key: key, halfOpen: true}, 0, true
		}

		if routeState.consecutiveFailures <= 0 && !now.Before(routeState.cooldownUntil) {
			delete(m.routeStates, key)
		}
	}

	return &RouteCooldownPermit{manager: m, key: key, halfOpen: false}, 0, true
}

func (m *RouteCooldownManager) releasePermit(permit *RouteCooldownPermit) {
	if permit == nil || permit.manager != m {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	if permit.released {
		return
	}
	permit.released = true

	if !permit.halfOpen {
		return
	}
	state, ok := m.routeStates[permit.key]
	if !ok {
		return
	}
	if state.halfOpenInFlight > 0 {
		state.halfOpenInFlight--
	}
}

func (m *RouteCooldownManager) RecordRouteSuccess(providerTokenId int, modelName string) {
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
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.routeStates, key)
	delete(m.unsupported, key)
}

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

func (m *RouteCooldownManager) MarkUnsupportedModel(providerTokenId int, modelName string) {
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
	ttl := time.Duration(cfg.UnsupportedModelHours) * time.Hour
	if ttl <= 0 {
		return
	}
	now := m.now()
	m.mu.Lock()
	defer m.mu.Unlock()
	m.unsupported[key] = now.Add(ttl)
}

func (m *RouteCooldownManager) isUnsupportedLocked(key routeCooldownKey, now time.Time) bool {
	until, ok := m.unsupported[key]
	if !ok {
		return false
	}
	if !now.Before(until) {
		delete(m.unsupported, key)
		return false
	}
	return true
}

func (m *RouteCooldownManager) isTokenInCooldownLocked(providerTokenId int, now time.Time, cfg RouteCooldownConfig) bool {
	state, ok := m.tokenStates[providerTokenId]
	if !ok {
		return false
	}
	m.applyTokenDecayLocked(state, now, cfg)
	if now.Before(state.cooldownUntil) {
		return true
	}
	if state.consecutiveFailures <= 0 && !now.Before(state.cooldownUntil) {
		delete(m.tokenStates, providerTokenId)
	}
	return false
}

func (m *RouteCooldownManager) isRouteInCooldownLocked(key routeCooldownKey, now time.Time, cfg RouteCooldownConfig) bool {
	state, ok := m.routeStates[key]
	if !ok {
		return false
	}
	m.applyRouteDecayLocked(state, now, cfg)
	if now.Before(state.cooldownUntil) {
		return true
	}
	if state.consecutiveFailures <= 0 && !now.Before(state.cooldownUntil) {
		delete(m.routeStates, key)
	}
	return false
}

func (m *RouteCooldownManager) applyRouteDecayLocked(state *routeCooldownState, now time.Time, cfg RouteCooldownConfig) {
	if state == nil {
		return
	}
	if cfg.DecayMinutes <= 0 || state.consecutiveFailures <= 0 {
		return
	}
	if state.lastFailureTime.IsZero() || !now.After(state.lastFailureTime) {
		return
	}
	interval := time.Duration(cfg.DecayMinutes) * time.Minute
	if interval <= 0 {
		return
	}
	periods := int(now.Sub(state.lastFailureTime) / interval)
	if periods <= 0 {
		return
	}
	state.consecutiveFailures -= periods
	if state.consecutiveFailures < 0 {
		state.consecutiveFailures = 0
	}
	state.lastFailureTime = state.lastFailureTime.Add(time.Duration(periods) * interval)
}

func (m *RouteCooldownManager) applyTokenDecayLocked(state *tokenCooldownState, now time.Time, cfg RouteCooldownConfig) {
	if state == nil {
		return
	}
	if cfg.DecayMinutes <= 0 || state.consecutiveFailures <= 0 {
		return
	}
	if state.lastFailureTime.IsZero() || !now.After(state.lastFailureTime) {
		return
	}
	interval := time.Duration(cfg.DecayMinutes) * time.Minute
	if interval <= 0 {
		return
	}
	periods := int(now.Sub(state.lastFailureTime) / interval)
	if periods <= 0 {
		return
	}
	state.consecutiveFailures -= periods
	if state.consecutiveFailures < 0 {
		state.consecutiveFailures = 0
	}
	state.lastFailureTime = state.lastFailureTime.Add(time.Duration(periods) * interval)
}

func normalizeCooldownModelName(modelName string) string {
	return strings.TrimSpace(strings.ToLower(modelName))
}

func computeCooldownDurationSeconds(baseSeconds int, multiplier float64, maxSeconds int, consecutiveFailures int, jitterRatio float64, randFloat func() float64) int {
	if baseSeconds <= 0 {
		baseSeconds = 30
	}
	if multiplier <= 1 {
		multiplier = 2
	}
	if maxSeconds <= 0 {
		maxSeconds = baseSeconds
	}
	if consecutiveFailures <= 0 {
		return 0
	}

	exponent := consecutiveFailures - 1
	seconds := float64(baseSeconds) * math.Pow(multiplier, float64(exponent))
	if seconds > float64(maxSeconds) {
		seconds = float64(maxSeconds)
	}
	if seconds < 0 {
		seconds = 0
	}

	jitter := jitterRatio
	if jitter < 0 {
		jitter = 0
	}
	if jitter > 0 && randFloat != nil {
		delta := (randFloat()*2 - 1) * jitter
		seconds = seconds * (1 + delta)
		if seconds < 0 {
			seconds = 0
		}
	}

	if seconds > float64(maxSeconds) {
		seconds = float64(maxSeconds)
	}

	return int(math.Round(seconds))
}

const (
	cooldownEnabledOptionKey            = "CooldownEnabled"
	cooldownBaseSecondsOptionKey        = "CooldownBaseSeconds"
	cooldownMultiplierOptionKey         = "CooldownMultiplier"
	cooldownMaxSecondsOptionKey         = "CooldownMaxSeconds"
	cooldownDecayMinutesOptionKey       = "CooldownDecayMinutes"
	minConsecutiveFailuresOptionKey     = "MinConsecutiveFailures"
	failureWindowSecondsOptionKey       = "FailureWindowSeconds"
	cooldownJitterRatioOptionKey        = "CooldownJitterRatio"
	halfOpenMaxInFlightOptionKey        = "HalfOpenMaxInFlight"
	unsupportedModelTTLHoursOptionKey   = "UnsupportedModelTTLHours"
	tokenCooldownBaseSecondsOptionKey   = "TokenCooldownBaseSeconds"
	tokenCooldownMaxSecondsOptionKey    = "TokenCooldownMaxSeconds"
)

func LoadRouteCooldownConfig() RouteCooldownConfig {
	defaultCfg := RouteCooldownConfig{
		Enabled:                true,
		BaseSeconds:            30,
		Multiplier:             2,
		MaxSeconds:             1800,
		DecayMinutes:           10,
		JitterRatio:            0.1,
		MinConsecutiveFailures: 1,
		FailureWindowSeconds:   0,
		HalfOpenMaxInFlight:    1,
		UnsupportedModelHours:  24,
		TokenBaseSeconds:       600,
		TokenMaxSeconds:        7200,
	}

	OptionMapRWMutex.RLock()
	defer OptionMapRWMutex.RUnlock()
	if OptionMap == nil {
		return defaultCfg
	}

	out := defaultCfg
	out.Enabled = parseOptionBool(OptionMap[cooldownEnabledOptionKey], defaultCfg.Enabled)
	out.BaseSeconds = parseOptionIntInRange(OptionMap[cooldownBaseSecondsOptionKey], defaultCfg.BaseSeconds, 1, 86400)
	out.Multiplier = parseOptionFloatInRange(OptionMap[cooldownMultiplierOptionKey], defaultCfg.Multiplier, 1, 10)
	out.MaxSeconds = parseOptionIntInRange(OptionMap[cooldownMaxSecondsOptionKey], defaultCfg.MaxSeconds, out.BaseSeconds, 86400)
	out.DecayMinutes = parseOptionIntInRange(OptionMap[cooldownDecayMinutesOptionKey], defaultCfg.DecayMinutes, 0, 7*24*60)
	out.MinConsecutiveFailures = parseOptionIntInRange(OptionMap[minConsecutiveFailuresOptionKey], defaultCfg.MinConsecutiveFailures, 1, 100)
	out.FailureWindowSeconds = parseOptionIntInRange(OptionMap[failureWindowSecondsOptionKey], defaultCfg.FailureWindowSeconds, 0, 7*24*3600)
	out.JitterRatio = parseOptionFloatInRange(OptionMap[cooldownJitterRatioOptionKey], defaultCfg.JitterRatio, 0, 1)
	out.HalfOpenMaxInFlight = parseOptionIntInRange(OptionMap[halfOpenMaxInFlightOptionKey], defaultCfg.HalfOpenMaxInFlight, 1, 1000)
	out.UnsupportedModelHours = parseOptionIntInRange(OptionMap[unsupportedModelTTLHoursOptionKey], defaultCfg.UnsupportedModelHours, 0, 365*24)
	out.TokenBaseSeconds = parseOptionIntInRange(OptionMap[tokenCooldownBaseSecondsOptionKey], defaultCfg.TokenBaseSeconds, 1, 7*24*3600)
	out.TokenMaxSeconds = parseOptionIntInRange(OptionMap[tokenCooldownMaxSecondsOptionKey], defaultCfg.TokenMaxSeconds, out.TokenBaseSeconds, 30*24*3600)

	return out
}

func parseOptionBool(raw string, fallback bool) bool {
	normalized := strings.ToLower(strings.TrimSpace(raw))
	if normalized == "" {
		return fallback
	}
	if normalized == "true" || normalized == "1" || normalized == "yes" || normalized == "y" {
		return true
	}
	if normalized == "false" || normalized == "0" || normalized == "no" || normalized == "n" {
		return false
	}
	return fallback
}

func parseOptionIntInRange(raw string, fallback int, min int, max int) int {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}

func parseOptionFloatInRange(raw string, fallback float64, min float64, max float64) float64 {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fallback
	}
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return fallback
	}
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}

// RouteCooldownStatus represents the cooldown status of a route for API response
type RouteCooldownStatus struct {
	InCooldown     bool   `json:"in_cooldown"`      // Whether the route is currently in cooldown
	Reason         string `json:"reason"`          // Reason for cooldown: "", "route", "token", "unsupported"
	RemainingSecs  int    `json:"remaining_secs"`  // Remaining cooldown time in seconds
	HalfOpen       bool   `json:"half_open"`       // Whether in half-open state
	HalfOpenInflight int  `json:"half_open_inflight"` // Number of in-flight requests in half-open state
}

// GetRouteCooldownStatus returns the current cooldown status for a route
func (m *RouteCooldownManager) GetRouteCooldownStatus(providerTokenId int, modelName string) *RouteCooldownStatus {
	cfg := m.configProvider()
	result := &RouteCooldownStatus{}
	if !cfg.Enabled {
		return result
	}

	normalizedModel := normalizeCooldownModelName(modelName)
	now := m.now()

	m.mu.Lock()
	defer m.mu.Unlock()

	// Check unsupported model first (highest priority, blocks everything)
	if until, ok := m.unsupported[routeCooldownKey{providerTokenId: providerTokenId, modelName: normalizedModel}]; ok {
		if !now.Before(until) {
			delete(m.unsupported, routeCooldownKey{providerTokenId: providerTokenId, modelName: normalizedModel})
		} else {
			result.InCooldown = true
			result.Reason = "unsupported"
			result.RemainingSecs = int(until.Sub(now).Seconds())
			if result.RemainingSecs < 0 {
				result.RemainingSecs = 0
			}
			return result
		}
	}

	// Check token-level cooldown
	if state, ok := m.tokenStates[providerTokenId]; ok {
		m.applyTokenDecayLocked(state, now, cfg)
		if now.Before(state.cooldownUntil) {
			result.InCooldown = true
			result.Reason = "token"
			result.RemainingSecs = int(state.cooldownUntil.Sub(now).Seconds())
			if result.RemainingSecs < 0 {
				result.RemainingSecs = 0
			}
			return result
		}
		if state.consecutiveFailures <= 0 && !now.Before(state.cooldownUntil) {
			delete(m.tokenStates, providerTokenId)
		}
	}

	// Check route-level cooldown
	key := routeCooldownKey{providerTokenId: providerTokenId, modelName: normalizedModel}
	if state, ok := m.routeStates[key]; ok {
		m.applyRouteDecayLocked(state, now, cfg)
		if now.Before(state.cooldownUntil) {
			result.InCooldown = true
			result.Reason = "route"
			result.RemainingSecs = int(state.cooldownUntil.Sub(now).Seconds())
			if result.RemainingSecs < 0 {
				result.RemainingSecs = 0
			}
			result.HalfOpenInflight = state.halfOpenInFlight
			return result
		}

		// Check half-open state (cooldown expired but still has failures)
		halfOpen := state.consecutiveFailures > 0 && state.cooldownUntil.After(state.lastFailureTime)
		if halfOpen {
			result.HalfOpen = true
			result.HalfOpenInflight = state.halfOpenInFlight
			result.RemainingSecs = 0
		}

		if state.consecutiveFailures <= 0 && !now.Before(state.cooldownUntil) {
			delete(m.routeStates, key)
		}
	}

	return result
}

// GetTokenCooldownStatus returns the current cooldown status for a token (all routes)
func (m *RouteCooldownManager) GetTokenCooldownStatus(providerTokenId int) (inCooldown bool, reason string, remainingSecs int) {
	cfg := m.configProvider()
	now := m.now()

	m.mu.Lock()
	defer m.mu.Unlock()

	state, ok := m.tokenStates[providerTokenId]
	if !ok {
		return false, "", 0
	}

	m.applyTokenDecayLocked(state, now, cfg)
	if now.Before(state.cooldownUntil) {
		remainingSecs = int(state.cooldownUntil.Sub(now).Seconds())
		if remainingSecs < 0 {
			remainingSecs = 0
		}
		return true, "token", remainingSecs
	}

	if state.consecutiveFailures <= 0 && !now.Before(state.cooldownUntil) {
		delete(m.tokenStates, providerTokenId)
	}

	return false, "", 0
}
