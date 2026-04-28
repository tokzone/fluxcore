package flux

import (
	"context"
	stderrors "errors"
	"log"
	"math/rand"
	"net/http"
	"slices"
	"sync/atomic"
	"time"

	"github.com/tokzone/fluxcore/endpoint"
	"github.com/tokzone/fluxcore/errors"
	"github.com/tokzone/fluxcore/message"
	"github.com/tokzone/fluxcore/provider"
)

// DoFunc is a pre-prepared execution function. It encapsulates the full request lifecycle:
// endpoint selection, protocol translation, HTTP transport, and health feedback.
// Input protocol is baked into the closure at generation time via DoFuncGen.
// Returns: response body, usage, provider URL (for logging), and error.
type DoFunc func(ctx context.Context, body []byte) ([]byte, *message.Usage, string, error)

// StreamDoFunc is the streaming counterpart of DoFunc.
// Returns: stream result, model name, provider URL, and error.
type StreamDoFunc func(ctx context.Context, body []byte) (*StreamResult, string, string, error)

// Client manages user endpoint selection with health awareness.
type Client struct {
	userEndpoints []*UserEndpoint // Pre-sorted by priority
	retryMax      int
	cached        atomic.Pointer[UserEndpoint]
	httpClient    *http.Client
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

// WithHTTPClient sets a custom HTTP client for requests.
// If not set, the default sharedClient is used.
func WithHTTPClient(httpClient *http.Client) Option {
	return func(c *Client) {
		if httpClient != nil {
			c.httpClient = httpClient
		}
	}
}

// NewClient creates a new client with the given user endpoints and options.
// User endpoints are sorted by priority (lower = preferred) for optimal selection.
func NewClient(userEndpoints []*UserEndpoint, opts ...Option) *Client {
	c := &Client{
		userEndpoints: userEndpoints,
		retryMax:      3,
		httpClient:    sharedClient, // Default to shared client
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

// buildProtoSelector pre-computes the target protocol for each endpoint and returns
// a protoMap plus a next() wrapper that bundles endpoint selection with protocol lookup.
func (c *Client) buildProtoSelector(inputProtocol provider.Protocol) (map[*UserEndpoint]provider.Protocol, func() (*UserEndpoint, provider.Protocol)) {
	protoMap := make(map[*UserEndpoint]provider.Protocol, len(c.userEndpoints))
	for _, ue := range c.userEndpoints {
		protoMap[ue] = ue.SelectProtocol(inputProtocol)
	}
	next := func() (*UserEndpoint, provider.Protocol) {
		ue := c.Next()
		if ue == nil {
			return nil, 0
		}
		return ue, protoMap[ue]
	}
	return protoMap, next
}

// DoFuncGen generates a pre-prepared DoFunc with the given input protocol baked in.
// For each endpoint in the client, the target (output) protocol is pre-computed via
// SelectProtocol, and stored in a protoMap. The returned DoFunc closure has zero
// protocol decision overhead on the hot path.
func DoFuncGen(client *Client, inputProtocol provider.Protocol) DoFunc {
	_, next := client.buildProtoSelector(inputProtocol)

	return func(ctx context.Context, body []byte) ([]byte, *message.Usage, string, error) {
		req, err := parseRequest(body, inputProtocol)
		if err != nil {
			return nil, nil, "", err
		}

		var lastErr error
		retryMax := client.RetryMax()

		for retry := 0; retry <= retryMax; retry++ {
			ue, targetProtocol := next()
			if ue == nil {
				break
			}

			if retry > 0 {
				backoff := backoffWithJitter(retry)
				select {
				case <-time.After(backoff):
				case <-ctx.Done():
					return nil, nil, "", ctx.Err()
				}
			}

			start := time.Now()
			resp, usage, err := doWithParsedRequest(ctx, ue, req, targetProtocol, inputProtocol, client.httpClient)
			latencyMs := int(time.Since(start).Milliseconds())

			if err == nil {
				client.Feedback(ue, nil, latencyMs)
				return resp, usage, ue.BaseURL(targetProtocol), nil
			}

			lastErr = err
			client.Feedback(ue, err, 0)

			if !errors.IsRetryable(err) {
				break
			}
		}

		if lastErr != nil {
			return nil, nil, "", lastErr
		}
		return nil, nil, "", errNoEndpoints
	}
}

// StreamDoFuncGen generates a pre-prepared streaming DoFunc with the given input protocol baked in.
func StreamDoFuncGen(client *Client, inputProtocol provider.Protocol) StreamDoFunc {
	_, next := client.buildProtoSelector(inputProtocol)

	return func(ctx context.Context, body []byte) (*StreamResult, string, string, error) {
		req, err := parseRequest(body, inputProtocol)
		if err != nil {
			return nil, "", "", err
		}
		req = req.WithStream(true)

		var lastErr error
		retryMax := client.RetryMax()

		for retry := 0; retry <= retryMax; retry++ {
			ue, targetProtocol := next()
			if ue == nil {
				break
			}

			if retry > 0 {
				backoff := backoffWithJitter(retry)
				select {
				case <-time.After(backoff):
				case <-ctx.Done():
					return nil, "", "", ctx.Err()
				}
			}

			start := time.Now()
			result, err := doStreamWithParsedRequest(ctx, ue, req, targetProtocol, inputProtocol, client.httpClient)

			if err == nil {
				// Wrap the result channel to track success on completion
				wrappedCh := make(chan []byte, defaultWrappedChannelBuffer)
				wrappedResult := &StreamResult{
					Ch:     wrappedCh,
					Usage:  result.Usage,
					Error:  result.Error,
					cancel: result.cancel,
				}

				go func() {
					defer func() {
						if r := recover(); r != nil {
							log.Printf("[fluxcore] stream wrapper panic recovered: %v", r)
						}
					}()
					defer close(wrappedCh)
					defer result.Close()

					for {
						select {
						case chunk, ok := <-result.Ch:
							if !ok {
								latencyMs := int(time.Since(start).Milliseconds())
								if result.Error() == nil {
									client.Feedback(ue, nil, latencyMs)
								} else {
									client.Feedback(ue, result.Error(), 0)
								}
								return
							}
							select {
							case wrappedCh <- chunk:
							case <-ctx.Done():
								client.Feedback(ue, ctx.Err(), 0)
								return
							}
						case <-ctx.Done():
							client.Feedback(ue, ctx.Err(), 0)
							return
						}
					}
				}()

				return wrappedResult, ue.Model(), ue.BaseURL(targetProtocol), nil
			}

			lastErr = err
			client.Feedback(ue, err, 0)

			if !errors.IsRetryable(err) {
				break
			}
		}

		if lastErr != nil {
			return nil, "", "", lastErr
		}
		return nil, "", "", errNoEndpoints
	}
}

var errNoEndpoints = errors.Wrap(errors.CodeNoEndpoint, "no available endpoints", nil)
