package fluxcore

import (
	"fmt"
	"time"

	"github.com/tokzone/fluxcore/internal/health"
)

// RouteDesc is an immutable value object describing a Route.
type RouteDesc struct {
	SvcEP      *ServiceEndpoint
	Model      Model
	Credential string // Decrypted upstream API key (opaque to fluxcore)
	Priority   int64  // Lower = preferred
}

// IdentityKey returns a stable string key for this route description.
// Format: serviceName/model/credential
func (d RouteDesc) IdentityKey() string {
	return fmt.Sprintf("%s/%s/%s", d.SvcEP.Service().Name, d.Model, d.Credential)
}

// Route is an aggregate root representing a specific model route through a service.
// It holds an immutable RouteDesc and a model-layer circuit breaker.
type Route struct {
	desc RouteDesc
	cb   *health.CircuitBreaker
}

const (
	routeCBThreshold = 3
	routeCBRecovery  = 60 * time.Second
)

// NewRoute creates a new Route with a healthy model-layer circuit breaker.
func NewRoute(desc RouteDesc) *Route {
	return &Route{
		desc: desc,
		cb: health.New(health.Config{
			Threshold: routeCBThreshold,
			Recovery:  routeCBRecovery,
		}),
	}
}

// IdentityKey returns the stable identity key for this route.
func (r *Route) IdentityKey() string {
	return r.desc.IdentityKey()
}

// IsAvailable returns true if both the service endpoint and the route are available.
func (r *Route) IsAvailable() bool {
	return r.desc.SvcEP.IsAvailable() && !r.cb.IsOpen()
}

// MarkSuccess resets the model-level circuit breaker to healthy.
func (r *Route) MarkSuccess() {
	r.cb.MarkSuccess()
}

// MarkModelFailure records a model-level failure (429/5xx).
func (r *Route) MarkModelFailure() {
	r.cb.MarkFailure()
}

// Desc returns the immutable RouteDesc.
func (r *Route) Desc() RouteDesc {
	return r.desc
}

// SvcEP returns the shared ServiceEndpoint reference.
func (r *Route) SvcEP() *ServiceEndpoint {
	return r.desc.SvcEP
}

// FailCount returns the current model-level failure count.
func (r *Route) FailCount() int {
	return r.cb.FailCount()
}

// LatencyEWMA returns the EWMA latency in milliseconds.
func (r *Route) LatencyEWMA() int {
	return r.cb.LatencyEWMA()
}

// UpdateLatency updates the EWMA latency with a new measurement in milliseconds.
func (r *Route) UpdateLatency(ms int) {
	r.cb.UpdateLatency(ms)
}
