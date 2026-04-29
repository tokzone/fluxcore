package fluxcore

import (
	"time"

	"github.com/tokzone/fluxcore/internal/health"
)

// ServiceEndpoint is an aggregate root representing an external AI service.
// It holds an immutable Service value object and a network-layer circuit breaker.
// Multiple Routes can share a reference to the same ServiceEndpoint.
type ServiceEndpoint struct {
	svc Service
	cb  *health.CircuitBreaker
}

const (
	serviceEndpointCBThreshold = 1
	serviceEndpointCBRecovery  = 120 * time.Second
)

// NewServiceEndpoint creates a new ServiceEndpoint with a healthy circuit breaker.
func NewServiceEndpoint(svc Service) *ServiceEndpoint {
	return &ServiceEndpoint{
		svc: svc,
		cb: health.New(health.Config{
			Threshold: serviceEndpointCBThreshold,
			Recovery:  serviceEndpointCBRecovery,
		}),
	}
}

// IsAvailable returns true if the service endpoint is available (circuit breaker closed).
func (se *ServiceEndpoint) IsAvailable() bool {
	return !se.cb.IsOpen()
}

// MarkSuccess resets the network circuit breaker to healthy.
func (se *ServiceEndpoint) MarkSuccess() {
	se.cb.MarkSuccess()
}

// MarkNetworkFailure records a network-level failure.
// Triggered only for DNS/Timeout/Connection errors.
func (se *ServiceEndpoint) MarkNetworkFailure() {
	se.cb.MarkFailure()
}

// Service returns the immutable Service value object.
func (se *ServiceEndpoint) Service() Service {
	return se.svc
}

// FailCount returns the current network failure count for this service.
func (se *ServiceEndpoint) FailCount() int {
	return se.cb.FailCount()
}

// LatencyEWMA returns the EWMA latency in milliseconds.
func (se *ServiceEndpoint) LatencyEWMA() int {
	return se.cb.LatencyEWMA()
}

// UpdateLatency updates the EWMA latency with a new measurement in milliseconds.
func (se *ServiceEndpoint) UpdateLatency(ms int) {
	se.cb.UpdateLatency(ms)
}
