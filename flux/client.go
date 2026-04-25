package flux

import (
	stderrors "errors"
	"math/rand"
	"slices"
	"sync/atomic"
	"time"

	"github.com/tokzone/fluxcore/endpoint"
	"github.com/tokzone/fluxcore/errors"
	"github.com/tokzone/fluxcore/provider"
)

// Client manages user endpoint selection with health awareness.
// Client provides a simple API: Do() and DoStream(), hiding all internal complexity.
type Client struct {
	userEndpoints []*UserEndpoint // Pre-sorted by priority
	retryMax      int
	cached        atomic.Pointer[UserEndpoint]
}

// Option configures Client during creation.
type Option func(*Client)

// WithRetryMax sets the maximum retry count (default: 3).
func WithRetryMax(n int) Option {
	return func(c *Client) {
		if n > 0 {
			c.retryMax = n
		}
	}
}

// NewClient creates a new client with the given user endpoints and options.
// User endpoints are sorted by priority (lower = preferred) for optimal selection.
func NewClient(userEndpoints []*UserEndpoint, opts ...Option) *Client {
	c := &Client{
		userEndpoints: userEndpoints,
		retryMax:      3,
	}

	// Apply options
	for _, opt := range opts {
		opt(c)
	}

	// Sort user endpoints by priority (lower = preferred)
	slices.SortFunc(c.userEndpoints, func(a, b *UserEndpoint) int {
		if a.Priority() < b.Priority() {
			return -1
		}
		if a.Priority() > b.Priority() {
			return 1
		}
		// Same priority: randomize to avoid always selecting same endpoint
		if rand.Float64() < 0.5 {
			return -1
		}
		return 1
	})

	// Set initial cached endpoint
	c.refreshCache()

	return c
}

// Next returns the next available user endpoint.
// Returns nil if no user endpoints are available.
// User endpoint is cached for high-frequency access (lock-free read).
func (c *Client) Next() *UserEndpoint {
	ue := c.cached.Load()
	if ue != nil && ue.Endpoint().IsAvailable() {
		return ue // Cached user endpoint available, return directly
	}

	// Cached user endpoint unavailable, select first available
	newUE := c.selectFirstAvailable(ue)
	if newUE != nil {
		c.cached.Store(newUE)
	}
	return newUE
}

// Feedback reports the result of a call to a user endpoint.
// Client internally handles:
// - Provider health update (network errors)
// - Endpoint health update (model errors: 429, 500)
// - Latency update (EWMA)
// - Cache refresh on failure
func (c *Client) Feedback(ue *UserEndpoint, err error, latencyMs int) {
	if ue == nil {
		return
	}

	ep := ue.Endpoint()
	prov := ep.Provider()

	if err == nil {
		// Success: reset health state + update latency
		ep.MarkEndpointSuccess()
		prov.MarkProviderSuccess()
		ep.UpdateEndpointLatency(latencyMs)
		prov.UpdateProviderLatency(latencyMs)
	} else {
		// Classify error and update appropriate layer
		if isProviderError(err) {
			// Provider layer failure (network)
			prov.MarkProviderFail()
		} else {
			// Endpoint layer failure (model)
			ep.MarkEndpointFail()
		}

		// If this is the cached user endpoint, try to switch
		for {
			current := c.cached.Load()
			if current != ue {
				break // Already switched by another goroutine
			}
			newUE := c.selectFirstAvailable(ue)
			if newUE == nil {
				break // No alternative user endpoint
			}
			if c.cached.CompareAndSwap(current, newUE) {
				break
			}
		}
	}
}

// RetryMax returns the maximum retry count.
func (c *Client) RetryMax() int {
	return c.retryMax
}

// refreshCache selects the first available endpoint and caches it.
func (c *Client) refreshCache() {
	first := c.selectFirstAvailable(nil)
	c.cached.Store(first)
}

// selectFirstAvailable selects the first available user endpoint, excluding the specified one.
// User endpoints are pre-sorted by priority, so first available is optimal.
func (c *Client) selectFirstAvailable(exclude *UserEndpoint) *UserEndpoint {
	for _, ue := range c.userEndpoints {
		if ue == exclude {
			continue
		}
		if ue.Endpoint().IsAvailable() {
			return ue
		}
	}
	return nil
}

func isProviderError(err error) bool {
	var classified *errors.ClassifiedError
	if stderrors.As(err, &classified) {
		switch classified.Code {
		case errors.CodeNetworkError, errors.CodeDNSError, errors.CodeTimeout:
			return true
		}
	}
	return false
}

// IsHealthy returns whether an endpoint health is healthy (for external monitoring).
// Checks both Provider and Endpoint health layers.
func (c *Client) IsHealthy(prov *provider.Provider, model string) bool {
	health := endpoint.GlobalRegistry().GetByProviderModel(prov, model)
	if health == nil {
		return false
	}
	return health.IsAvailable()
}

// EndpointLatency returns the EWMA latency for an endpoint (for external monitoring).
func (c *Client) EndpointLatency(prov *provider.Provider, model string) int {
	health := endpoint.GlobalRegistry().GetByProviderModel(prov, model)
	if health == nil {
		return 0
	}
	return health.EndpointLatencyEWMA()
}

// ProviderLatency returns the EWMA latency for a provider (for external monitoring).
func (c *Client) ProviderLatency(prov *provider.Provider) int {
	return prov.ProviderLatencyEWMA()
}

const (
	defaultBaseBackoff = 100 * time.Millisecond
	defaultMaxBackoff  = 5 * time.Second
)

// backoffWithJitter calculates exponential backoff with full jitter.
// Go 1.20+ rand.Float64 is concurrent-safe.
func backoffWithJitter(attempt int) time.Duration {
	if attempt <= 0 {
		return 0
	}

	// Exponential backoff: base * 2^(attempt-1)
	backoff := defaultBaseBackoff << uint(attempt-1)
	if backoff > defaultMaxBackoff {
		backoff = defaultMaxBackoff
	}
	// Full jitter: random value between 0 and backoff
	return time.Duration(rand.Float64() * float64(backoff))
}
