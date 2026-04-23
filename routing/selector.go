package routing

import (
	"errors"
	"slices"
)

// ErrNoEndpoints is returned when no endpoints are available
var ErrNoEndpoints = errors.New("no endpoints available")

// compareEndpoint compares two endpoints for sorting.
// Returns negative if a is better, positive if b is better, 0 if equal.
func compareEndpoint(a, b *Endpoint) int {
	// Compare by priority (lower is better)
	if a.Priority != b.Priority {
		return int(a.Priority - b.Priority)
	}
	// Same priority: compare by EWMA latency (lower is better)
	aLatency := a.LatencyEWMA()
	bLatency := b.LatencyEWMA()
	if aLatency != bLatency {
		return aLatency - bLatency
	}
	// Fallback to static latency
	return a.LatencyMs - b.LatencyMs
}

// selectBest returns the best endpoint by priority (lower is better), then latency
func selectBest(endpoints []*Endpoint) *Endpoint {
	if len(endpoints) == 0 {
		return nil
	}
	return slices.MinFunc(endpoints, compareEndpoint)
}


