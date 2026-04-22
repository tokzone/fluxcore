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
}

// Endpoint is the routing unit - a Key + Model combination.
type Endpoint struct {
	ID     uint      // Unique identifier
	Key    *Key      // Connection info (shared pointer)
	Model  string    // Model name. Required for Gemini (used in URL path). Empty for OpenAI/Anthropic/Cohere (model from request body).

	// Routing attributes
	InputPrice  float64 // Input token price
	OutputPrice float64 // Output token price
	LatencyMs   int     // Latency in milliseconds

	// Health state (internal)
	state *endpointState
}

// NewEndpoint creates a new endpoint with default healthy status and default circuit breaker config.
// The model parameter is required for Gemini (used in URL construction like /v1/models/{model}:generateContent).
// For OpenAI, Anthropic, and Cohere, pass empty string "" - the model is taken from the request body.
// Panics if key is nil.
func NewEndpoint(id uint, key *Key, model string, inputPrice, outputPrice float64) *Endpoint {
	if key == nil {
		panic("endpoint Key cannot be nil")
	}
	return NewEndpointWithConfig(id, key, model, inputPrice, outputPrice, CircuitBreakerConfig{})
}

// NewEndpointWithConfig creates a new endpoint with custom circuit breaker configuration.
// Panics if key is nil.
func NewEndpointWithConfig(id uint, key *Key, model string, inputPrice, outputPrice float64, cbConfig CircuitBreakerConfig) *Endpoint {
	if key == nil {
		panic("endpoint Key cannot be nil")
	}
	cfg := cbConfig.defaults()
	ep := &Endpoint{
		ID:          id,
		Key:         key,
		Model:       model,
		InputPrice:  inputPrice,
		OutputPrice: outputPrice,
		state:       &endpointState{},
	}
	ep.state.healthy.Store(true)
	ep.state.threshold.Store(int32(cfg.Threshold))
	ep.state.recoveryNanos.Store(cfg.RecoveryTimeout.Nanoseconds())
	return ep
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
	if ep.InputPrice < 0 {
		return errors.New("endpoint InputPrice must be non-negative")
	}
	if ep.OutputPrice < 0 {
		return errors.New("endpoint OutputPrice must be non-negative")
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