package routing

import (
	"errors"
	"slices"
)

// ErrNoEndpoints is returned when no endpoints are available
var ErrNoEndpoints = errors.New("no endpoints available")

// selectBest returns the best endpoint by price (lower is better), then latency
func selectBest(endpoints []*Endpoint) *Endpoint {
	if len(endpoints) == 0 {
		return nil
	}
	return slices.MinFunc(endpoints, func(a, b *Endpoint) int {
		aPrice := a.InputPrice + a.OutputPrice
		bPrice := b.InputPrice + b.OutputPrice
		if aPrice != bPrice {
			// Compare by price (lower is better)
			if aPrice < bPrice {
				return -1
			}
			return 1
		}
		// Same price: compare by latency (lower is better)
		return a.LatencyMs - b.LatencyMs
	})
}


