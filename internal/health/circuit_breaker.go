package health

import (
	"sync/atomic"
	"time"
)

// State represents the three-state circuit breaker model.
type State int32

const (
	StateClosed   State = iota // Normal operation, requests flow through
	StateOpen                  // Circuit tripped, requests are rejected
	StateHalfOpen              // Probe state, allows a single trial request
)

// Config holds circuit breaker parameters.
type Config struct {
	Threshold int
	Recovery  time.Duration
}

// CircuitBreaker implements a three-state circuit breaker with EWMA latency tracking.
// All methods are lock-free via atomic operations, safe for concurrent use.
type CircuitBreaker struct {
	state         atomic.Int32
	failCount     atomic.Int32
	lastFailNanos atomic.Int64
	threshold     int32
	recoveryNanos int64
	latencyEWMA   atomic.Int64 // scaled by 1000 for CAS-based EWMA
}

// New creates a new CircuitBreaker in the Closed state.
func New(cfg Config) *CircuitBreaker {
	return &CircuitBreaker{
		threshold:     int32(cfg.Threshold),
		recoveryNanos: cfg.Recovery.Nanoseconds(),
	}
}

// IsOpen returns true if the circuit breaker is currently blocking requests.
// Automatically transitions from Open to HalfOpen when the recovery timeout expires.
func (cb *CircuitBreaker) IsOpen() bool {
	state := State(cb.state.Load())
	if state == StateClosed || state == StateHalfOpen {
		return false
	}
	lastFailNanos := cb.lastFailNanos.Load()
	if lastFailNanos == 0 {
		return true
	}
	lastFail := time.Unix(0, lastFailNanos)
	if time.Since(lastFail) > time.Duration(cb.recoveryNanos) {
		cb.state.Store(int32(StateHalfOpen))
		return false
	}
	return true
}

// MarkSuccess resets the circuit breaker to the Closed state.
func (cb *CircuitBreaker) MarkSuccess() {
	cb.failCount.Store(0)
	cb.state.Store(int32(StateClosed))
	cb.lastFailNanos.Store(0)
}

// MarkFailure records a failure and returns true if this triggered the circuit to open.
func (cb *CircuitBreaker) MarkFailure() bool {
	cb.lastFailNanos.Store(time.Now().UnixNano())
	count := cb.failCount.Add(1)
	if count >= cb.threshold {
		cb.state.Store(int32(StateOpen))
		return true
	}
	return false
}

// FailCount returns the current consecutive failure count.
func (cb *CircuitBreaker) FailCount() int {
	return int(cb.failCount.Load())
}

// LatencyEWMA returns the EWMA latency in milliseconds.
func (cb *CircuitBreaker) LatencyEWMA() int {
	const scale = 1000
	return int(cb.latencyEWMA.Load() / scale)
}

// UpdateLatency updates the EWMA latency with a new measurement in milliseconds.
func (cb *CircuitBreaker) UpdateLatency(latencyMs int) {
	const alpha = 0.1
	const scale = 1000
	newLatency := int64(latencyMs * scale)
	for {
		old := cb.latencyEWMA.Load()
		var updated int64
		if old == 0 {
			updated = newLatency
		} else {
			updated = int64(alpha*float64(newLatency) + (1-alpha)*float64(old))
		}
		if cb.latencyEWMA.CompareAndSwap(old, updated) {
			break
		}
	}
}
