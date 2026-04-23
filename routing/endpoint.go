package routing

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"sync/atomic"
	"time"
)

// Default circuit breaker settings
const (
	DefaultCircuitBreakerThreshold = 3               // Failures before circuit breaker opens
	DefaultRecoveryTimeout         = 60 * time.Second // Time before retrying unhealthy endpoint
)

// CircuitBreakerConfig holds circuit breaker configuration.
// Zero values use defaults (Threshold=3, RecoveryTimeout=60s).
type CircuitBreakerConfig struct {
	Threshold      int           // Failures before circuit breaker opens (default: 3)
	RecoveryTimeout time.Duration // Time before retrying unhealthy endpoint (default: 60s)
}

// defaults returns config with zero values replaced by defaults.
func (c CircuitBreakerConfig) defaults() CircuitBreakerConfig {
	if c.Threshold <= 0 {
		c.Threshold = DefaultCircuitBreakerThreshold
	}
	if c.RecoveryTimeout <= 0 {
		c.RecoveryTimeout = DefaultRecoveryTimeout
	}
	return c
}

// endpointState holds runtime health state for an Endpoint (internal).
type endpointState struct {
	failCount     atomic.Int32  // Consecutive failure count
	healthy       atomic.Bool   // Health status
	lastFailNanos atomic.Int64  // Last failure time as Unix nanoseconds
	threshold     atomic.Int32  // Circuit breaker threshold
	recoveryNanos atomic.Int64  // Recovery timeout in nanoseconds
	latencyEWMA   atomic.Int64  // EWMA latency in milliseconds (scaled by 1000 for precision)
}

// Endpoint is the routing unit - a Key + Model combination.
type Endpoint struct {
	ID     uint   // Unique identifier
	Key    *Key   // Connection info (shared pointer)
	Model  string // Model name. Required for Gemini (used in URL path). Empty for OpenAI/Anthropic/Cohere (model from request body).

	// Routing attributes
	Priority int64 // Generic priority for endpoint selection. Lower values are preferred.
	// The semantic meaning (price, latency, custom combination) is defined by the caller.
	LatencyMs int // Latency in milliseconds

	// Health state (internal)
	state *endpointState
}

// ErrNilKey is returned when attempting to create an endpoint with nil key.
var ErrNilKey = errors.New("endpoint Key cannot be nil")

// NewEndpoint creates a new endpoint with default healthy status and default circuit breaker config.
// The model parameter is required for Gemini (used in URL construction like /v1/models/{model}:generateContent).
// For OpenAI, Anthropic, and Cohere, pass empty string "" - the model is taken from the request body.
// Returns ErrNilKey if key is nil.
func NewEndpoint(id uint, key *Key, model string, priority int64) (*Endpoint, error) {
	if key == nil {
		return nil, ErrNilKey
	}
	return NewEndpointWithConfig(id, key, model, priority, CircuitBreakerConfig{})
}

// NewEndpointWithConfig creates a new endpoint with custom circuit breaker configuration.
// Returns ErrNilKey if key is nil.
func NewEndpointWithConfig(id uint, key *Key, model string, priority int64, cbConfig CircuitBreakerConfig) (*Endpoint, error) {
	if key == nil {
		return nil, ErrNilKey
	}
	cfg := cbConfig.defaults()
	ep := &Endpoint{
		ID:       id,
		Key:      key,
		Model:    model,
		Priority: priority,
		state:    &endpointState{},
	}
	ep.state.healthy.Store(true)
	ep.state.threshold.Store(int32(cfg.Threshold))
	ep.state.recoveryNanos.Store(cfg.RecoveryTimeout.Nanoseconds())
	return ep, nil
}

// SetPriority updates the endpoint priority.
func (ep *Endpoint) SetPriority(p int64) {
	ep.Priority = p
}

// IsCircuitBreakerOpen returns true if the circuit breaker is open (endpoint should be skipped).
// Returns true when: unhealthy AND within recovery timeout period.
func (ep *Endpoint) IsCircuitBreakerOpen() bool {
	if !ep.state.healthy.Load() {
		lastFailNanos := ep.state.lastFailNanos.Load()
		if lastFailNanos == 0 {
			return true
		}
		recoveryNanos := ep.state.recoveryNanos.Load()
		if recoveryNanos <= 0 {
			recoveryNanos = DefaultRecoveryTimeout.Nanoseconds()
		}
		lastFail := time.Unix(0, lastFailNanos)
		return time.Since(lastFail) <= time.Duration(recoveryNanos)
	}
	return false
}

// MarkSuccess marks an endpoint as healthy and resets failure count.
func (ep *Endpoint) MarkSuccess() {
	ep.state.failCount.Store(0)
	ep.state.healthy.Store(true)
	ep.state.lastFailNanos.Store(0)
}

// MarkFail marks an endpoint as failed with current time.
func (ep *Endpoint) MarkFail() {
	failCount := ep.state.failCount.Add(1)
	threshold := ep.state.threshold.Load()
	if threshold <= 0 {
		threshold = DefaultCircuitBreakerThreshold
	}
	if failCount >= threshold {
		ep.state.healthy.Store(false)
	}
	ep.state.lastFailNanos.Store(time.Now().UnixNano())
}

// setHealthy sets the health status atomically (for tests).
func (ep *Endpoint) setHealthy(healthy bool) {
	ep.state.healthy.Store(healthy)
}

// isHealthy returns the health status atomically (for tests).
func (ep *Endpoint) isHealthy() bool {
	return ep.state.healthy.Load()
}

// failCount returns the consecutive failure count atomically (for tests).
func (ep *Endpoint) failCount() int32 {
	return ep.state.failCount.Load()
}

// setLastFail sets the last failure time (for tests).
func (ep *Endpoint) setLastFail(t time.Time) {
	ep.state.lastFailNanos.Store(t.UnixNano())
}

// UpdateLatency updates the EWMA latency with a new measurement.
// EWMA formula: new = α * current + (1-α) * old, where α = 0.1
func (ep *Endpoint) UpdateLatency(latencyMs int) {
	const alpha = 0.1
	const scale = 1000 // Scale for precision

	newLatency := int64(latencyMs * scale)
	oldLatency := ep.state.latencyEWMA.Load()

	if oldLatency == 0 {
		// First measurement
		ep.state.latencyEWMA.Store(newLatency)
	} else {
		// EWMA update
		updated := int64(alpha*float64(newLatency) + (1-alpha)*float64(oldLatency))
		ep.state.latencyEWMA.Store(updated)
	}
}

// LatencyEWMA returns the EWMA latency in milliseconds.
func (ep *Endpoint) LatencyEWMA() int {
	const scale = 1000
	return int(ep.state.latencyEWMA.Load() / scale)
}

// Validate checks endpoint configuration for errors.
// Note: SSRF protection is NOT enforced here - use IsPrivateIP() in application layer.
func (ep *Endpoint) Validate() error {
	// Key validation
	if ep.Key == nil {
		return errors.New("endpoint Key is required")
	}
	if ep.Key.BaseURL == "" {
		return errors.New("endpoint Key.BaseURL is required")
	}
	u, err := url.Parse(ep.Key.BaseURL)
	if err != nil {
		return fmt.Errorf("invalid BaseURL: %w", err)
	}
	if u.Scheme != "https" && u.Scheme != "http" {
		return errors.New("BaseURL must use http or https scheme")
	}

	if ep.Key.Protocol < ProtocolOpenAI || ep.Key.Protocol > ProtocolCohere {
		return errors.New("invalid Protocol: must be ProtocolOpenAI, ProtocolAnthropic, ProtocolGemini, or ProtocolCohere")
	}

	if ep.Key.APIKey == "" {
		return errors.New("endpoint Key.APIKey is required")
	}

	// Business constraints
	// Model is required for Gemini (used in URL path), optional for other protocols
	if ep.Model == "" && ep.Key.Protocol == ProtocolGemini {
		return errors.New("endpoint Model is required for Gemini protocol")
	}
	if ep.LatencyMs < 0 {
		return errors.New("endpoint LatencyMs must be non-negative")
	}

	return nil
}

// specialHosts are hostname strings that should be treated as private
var specialHosts = []string{"localhost", "0.0.0.0"}

// IsPrivateIP checks if hostname points to a private/internal IP address.
// Application layer should call this to implement SSRF protection based on their security policy.
func IsPrivateIP(host string) bool {
	for _, h := range specialHosts {
		if host == h {
			return true
		}
	}

	// Try parsing as IP address directly
	ip := net.ParseIP(host)
	if ip != nil {
		return ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast()
	}

	// Not a valid IP address - application layer should handle DNS resolution if needed
	return false
}