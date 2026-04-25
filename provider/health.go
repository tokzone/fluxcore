package provider

import (
	"sync/atomic"
	"time"
)

// Default provider-level circuit breaker settings
const (
	defaultProviderCircuitBreakerThreshold = 1                 // Network failure: immediate circuit break
	defaultProviderRecoveryTimeout         = 120 * time.Second // Provider recovery takes longer
)

// ProviderHealthState holds the health state for a Provider (network layer).
type ProviderHealthState struct {
	healthy       atomic.Bool  // Provider network available
	failCount     atomic.Int32 // Consecutive failure count
	lastFailNanos atomic.Int64 // Last failure timestamp
	threshold     int          // Circuit breaker threshold
	recoveryNanos int64        // Recovery timeout (nanoseconds)
	latencyEWMA   atomic.Int64 // Network latency EWMA (scaled by 1000)
}

// newProviderHealthState creates a new provider health state with defaults.
func newProviderHealthState() *ProviderHealthState {
	s := &ProviderHealthState{
		threshold:     defaultProviderCircuitBreakerThreshold,
		recoveryNanos: defaultProviderRecoveryTimeout.Nanoseconds(),
	}
	s.healthy.Store(true)
	return s
}

// IsCircuitBreakerOpen returns true if the provider circuit breaker is open.
func (s *ProviderHealthState) IsCircuitBreakerOpen() bool {
	if !s.healthy.Load() {
		lastFailNanos := s.lastFailNanos.Load()
		if lastFailNanos == 0 {
			return true
		}
		lastFail := time.Unix(0, lastFailNanos)
		recovery := time.Duration(s.recoveryNanos)
		return time.Since(lastFail) <= recovery
	}
	return false
}

// MarkSuccess marks the provider as healthy and resets failure count.
func (s *ProviderHealthState) MarkSuccess() {
	s.failCount.Store(0)
	s.healthy.Store(true)
	s.lastFailNanos.Store(0)
}

// MarkFail marks the provider as failed.
// For provider-level failures, threshold is 1 (immediate circuit break).
func (s *ProviderHealthState) MarkFail() {
	failCount := s.failCount.Add(1)
	if failCount >= int32(s.threshold) {
		s.healthy.Store(false)
	}
	s.lastFailNanos.Store(time.Now().UnixNano())
}

// UpdateLatency updates the EWMA latency with a new measurement.
// Uses CAS loop to avoid race condition with concurrent updates.
func (s *ProviderHealthState) UpdateLatency(latencyMs int) {
	const alpha = 0.1
	const scale = 1000

	newLatency := int64(latencyMs * scale)

	for {
		old := s.latencyEWMA.Load()
		var updated int64
		if old == 0 {
			updated = newLatency
		} else {
			updated = int64(alpha*float64(newLatency) + (1-alpha)*float64(old))
		}
		if s.latencyEWMA.CompareAndSwap(old, updated) {
			break
		}
	}
}

// LatencyEWMA returns the EWMA latency in milliseconds.
func (s *ProviderHealthState) LatencyEWMA() int {
	const scale = 1000
	return int(s.latencyEWMA.Load() / scale)
}
