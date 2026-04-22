package routing

import (
	"sync"
	"sync/atomic"
)

type EndpointPool struct {
	_currentEp atomic.Pointer[Endpoint] // Cached best endpoint (lock-free read)
	mu         sync.RWMutex
	_endpoints []*Endpoint // Sequential access (routing)
	_retryMax  int
}

func NewEndpointPool(endpoints []*Endpoint, retryMax int) *EndpointPool {
	if retryMax <= 0 {
		retryMax = 2
	}

	pool := &EndpointPool{
		_endpoints: endpoints,
		_retryMax:  retryMax,
	}

	// Set initial currentEp to best endpoint
	if best := pool.SelectBest(); best != nil {
		pool._currentEp.Store(best)
	}

	return pool
}

// CurrentEp returns the cached best endpoint.
func (p *EndpointPool) CurrentEp() *Endpoint {
	return p._currentEp.Load()
}

// RetryMax returns the maximum retry count.
func (p *EndpointPool) RetryMax() int {
	return p._retryMax
}

// MarkFail marks an endpoint as failed and switches to another.
func (p *EndpointPool) MarkFail(ep *Endpoint) {
	ep.MarkFail()

	// Use CAS loop to ensure atomic switching
	for {
		current := p._currentEp.Load()
		if current != ep {
			break
		}

		newEp := p.selectBestExcluding(ep)
		if newEp == nil {
			break
		}

		if p._currentEp.CompareAndSwap(current, newEp) {
			break
		}
	}
}

// selectBestExcluding selects best endpoint, excluding the specified one.
func (p *EndpointPool) selectBestExcluding(exclude *Endpoint) *Endpoint {
	p.mu.RLock()
	defer p.mu.RUnlock()

	filtered := make([]*Endpoint, 0, len(p._endpoints))
	for _, ep := range p._endpoints {
		if ep != exclude && !ep.IsCircuitBreakerOpen() {
			filtered = append(filtered, ep)
		}
	}
	return selectBest(filtered)
}

// SelectBest selects the best endpoint (skipping unhealthy ones).
func (p *EndpointPool) SelectBest() *Endpoint {
	p.mu.RLock()
	defer p.mu.RUnlock()

	filtered := make([]*Endpoint, 0, len(p._endpoints))
	for _, ep := range p._endpoints {
		if !ep.IsCircuitBreakerOpen() {
			filtered = append(filtered, ep)
		}
	}
	return selectBest(filtered)
}