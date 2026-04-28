package endpoint

import (
	"errors"
	"fmt"
	"net/url"
	"slices"

	"github.com/tokzone/fluxcore/provider"
)

// Endpoint is a global singleton - Provider + Model combination.
// Endpoint health state is shared across all users using this endpoint.
type Endpoint struct {
	ID        uint                 // Unique identifier
	provider  *provider.Provider   // Reference to shared Provider
	Model     string               // Model name. Required for Gemini. Empty for OpenAI/Anthropic/Cohere.
	Protocols []provider.Protocol  // Supported protocol capabilities (at least one required)
	state     *EndpointHealthState // Model-layer health + inference latency
}

// errNilProvider is returned when attempting to create an endpoint with nil provider.
var errNilProvider = errors.New("endpoint provider cannot be nil")

// NewEndpoint creates a new endpoint with default healthy state.
// The model parameter is required for Gemini (used in URL construction like /v1/models/{model}:generateContent).
// For OpenAI, Anthropic, and Cohere, pass empty string "" - the model is taken from the request body.
// protocols is the list of supported protocol formats (at least one required).
// Returns errNilProvider if provider is nil.
func NewEndpoint(id uint, prov *provider.Provider, model string, protocols []provider.Protocol) (*Endpoint, error) {
	if prov == nil {
		return nil, errNilProvider
	}
	return &Endpoint{
		ID:        id,
		provider:  prov,
		Model:     model,
		Protocols: protocols,
		state:     newEndpointHealthState(),
	}, nil
}

func (ep *Endpoint) Provider() *provider.Provider {
	return ep.provider
}

// Protocol returns the default protocol (first in Protocols list).
func (ep *Endpoint) Protocol() provider.Protocol {
	if len(ep.Protocols) > 0 {
		return ep.Protocols[0]
	}
	return provider.ProtocolOpenAI // fallback, should not happen after validation
}

// HasProtocol returns true if the endpoint supports the given protocol.
func (ep *Endpoint) HasProtocol(p provider.Protocol) bool {
	return slices.Contains(ep.Protocols, p)
}

// SelectProtocol returns the matching protocol if endpoint supports it, otherwise the default (Protocols[0]).
func (ep *Endpoint) SelectProtocol(input provider.Protocol) provider.Protocol {
	if ep.HasProtocol(input) {
		return input
	}
	return ep.Protocol()
}

func (ep *Endpoint) BaseURL(proto provider.Protocol) string {
	return ep.provider.BaseURLFor(proto)
}

// IsCircuitBreakerOpen returns true if the endpoint or its provider has circuit breaker open.
// Checks both Provider layer (network) and Endpoint layer (model).
func (ep *Endpoint) IsCircuitBreakerOpen() bool {
	// Provider layer check (network)
	if ep.provider.IsCircuitBreakerOpen() {
		return true
	}
	// Endpoint layer check (model)
	return ep.state.IsCircuitBreakerOpen()
}

// IsAvailable returns true if the endpoint is available for routing.
func (ep *Endpoint) IsAvailable() bool {
	return !ep.IsCircuitBreakerOpen()
}

// MarkEndpointFail marks an endpoint-level failure (model error: 429, 500).
func (ep *Endpoint) MarkEndpointFail() {
	ep.state.MarkFail()
}

// MarkEndpointSuccess marks an endpoint-level success.
func (ep *Endpoint) MarkEndpointSuccess() {
	ep.state.MarkSuccess()
}

func (ep *Endpoint) EndpointLatencyEWMA() int {
	return ep.state.LatencyEWMA()
}

func (ep *Endpoint) UpdateEndpointLatency(latencyMs int) {
	ep.state.UpdateLatency(latencyMs)
}

// Validate checks endpoint configuration for errors.
func (ep *Endpoint) Validate() error {
	// Provider validation
	if ep.provider == nil {
		return errors.New("endpoint provider is required")
	}
	if len(ep.provider.BaseURLs) == 0 {
		return errors.New("endpoint provider.BaseURLs is required (at least one protocol URL)")
	}
	for proto, rawURL := range ep.provider.BaseURLs {
		u, err := url.Parse(rawURL)
		if err != nil {
			return fmt.Errorf("invalid BaseURL for %s: %w", proto, err)
		}
		if u.Scheme != "https" && u.Scheme != "http" {
			return fmt.Errorf("BaseURL for %s must use http or https scheme", proto)
		}
	}

	// Protocols validation
	if len(ep.Protocols) == 0 {
		return errors.New("endpoint Protocols is required (at least one protocol)")
	}
	for _, p := range ep.Protocols {
		if p < provider.ProtocolOpenAI || p > provider.ProtocolCohere {
			return errors.New("invalid Protocol in Protocols list")
		}
	}

	// Model is required for Gemini (used in URL path), optional for other protocols
	if ep.Model == "" && ep.HasProtocol(provider.ProtocolGemini) {
		return errors.New("endpoint Model is required for Gemini protocol")
	}

	return nil
}
