package endpoint

import (
	"errors"
	"fmt"
	"net/url"

	"github.com/tokzone/fluxcore/provider"
)

// Endpoint is a global singleton - Provider + Model combination.
// Endpoint health state is shared across all users using this endpoint.
type Endpoint struct {
	ID       uint                 // Unique identifier
	provider *provider.Provider   // Reference to shared Provider
	Model    string               // Model name. Required for Gemini. Empty for OpenAI/Anthropic/Cohere.
	state    *EndpointHealthState // Model-layer health + inference latency
}

// errNilProvider is returned when attempting to create an endpoint with nil provider.
var errNilProvider = errors.New("endpoint provider cannot be nil")

// NewEndpoint creates a new endpoint with default healthy state.
// The model parameter is required for Gemini (used in URL construction like /v1/models/{model}:generateContent).
// For OpenAI, Anthropic, and Cohere, pass empty string "" - the model is taken from the request body.
// Returns errNilProvider if provider is nil.
func NewEndpoint(id uint, prov *provider.Provider, model string) (*Endpoint, error) {
	if prov == nil {
		return nil, errNilProvider
	}
	return &Endpoint{
		ID:       id,
		provider: prov,
		Model:    model,
		state:    newEndpointHealthState(),
	}, nil
}

func (ep *Endpoint) Provider() *provider.Provider {
	return ep.provider
}

func (ep *Endpoint) Protocol() provider.Protocol {
	return ep.provider.Protocol
}

func (ep *Endpoint) BaseURL() string {
	return ep.provider.BaseURL
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
	if ep.provider.BaseURL == "" {
		return errors.New("endpoint provider.BaseURL is required")
	}
	u, err := url.Parse(ep.provider.BaseURL)
	if err != nil {
		return fmt.Errorf("invalid BaseURL: %w", err)
	}
	if u.Scheme != "https" && u.Scheme != "http" {
		return errors.New("BaseURL must use http or https scheme")
	}

	if ep.provider.Protocol < provider.ProtocolOpenAI || ep.provider.Protocol > provider.ProtocolCohere {
		return errors.New("invalid Protocol: must be ProtocolOpenAI, ProtocolAnthropic, ProtocolGemini, or ProtocolCohere")
	}

	// Model is required for Gemini (used in URL path), optional for other protocols
	if ep.Model == "" && ep.provider.Protocol == provider.ProtocolGemini {
		return errors.New("endpoint Model is required for Gemini protocol")
	}

	return nil
}
