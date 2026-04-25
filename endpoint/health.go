package endpoint

import (
	"sync/atomic"
	"time"
)

// Default endpoint-level circuit breaker settings
const (
	defaultEndpointCircuitBreakerThreshold = 3                // Model failure: 3 failures trigger circuit
	defaultEndpointRecoveryTimeout         = 60 * time.Second // Endpoint recovery
)

// EndpointHealthState holds the health state for an Endpoint (model layer).
type EndpointHealthState struct {
	healthy       atomic.Bool  // Endpoint available
	failCount     atomic.Int32 // Consecutive failure count
	lastFailNanos atomic.Int64 // Last failure timestamp
	threshold     int          // Circuit breaker threshold
	recoveryNanos int64        // Recovery timeout (nanoseconds)
	latencyEWMA   atomic.Int64 // Total request latency EWMA (scaled by 1000)
}

// newEndpointHealthState creates a new endpoint health state with defaults.
func newEndpointHealthState() *EndpointHealthState {
	s := &EndpointHealthState{
		threshold:     defaultEndpointCircuitBreakerThreshold,
		recoveryNanos: defaultEndpointRecoveryTimeout.Nanoseconds(),
	}
	s.healthy.Store(true)
	return s
}

// IsCircuitBreakerOpen returns true if the endpoint circuit breaker is open.
func (s *EndpointHealthState) IsCircuitBreakerOpen() bool {
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

// MarkSuccess marks the endpoint as healthy and resets failure count.
func (s *EndpointHealthState) MarkSuccess() {
	s.failCount.Store(0)
	s.healthy.Store(true)
	s.lastFailNanos.Store(0)
}

// MarkFail marks the endpoint as failed.
// For endpoint-level failures, threshold is 3 (failures trigger circuit).
func (s *EndpointHealthState) MarkFail() {
	failCount := s.failCount.Add(1)
	if failCount >= int32(s.threshold) {
		s.healthy.Store(false)
	}
	s.lastFailNanos.Store(time.Now().UnixNano())
}

// UpdateLatency updates the EWMA latency with a new measurement.
// Uses CAS loop to avoid race condition with concurrent updates.
func (s *EndpointHealthState) UpdateLatency(latencyMs int) {
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
func (s *EndpointHealthState) LatencyEWMA() int {
	const scale = 1000
	return int(s.latencyEWMA.Load() / scale)
}
