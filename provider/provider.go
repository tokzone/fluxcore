package provider

// Protocol represents the communication protocol format for LLM requests.
type Protocol int

// Protocol constants define the supported communication formats.
const (
	ProtocolOpenAI Protocol = iota
	ProtocolAnthropic
	ProtocolGemini
	ProtocolCohere
)

// String returns the string representation of the protocol.
func (p Protocol) String() string {
	switch p {
	case ProtocolOpenAI:
		return "openai"
	case ProtocolAnthropic:
		return "anthropic"
	case ProtocolGemini:
		return "gemini"
	case ProtocolCohere:
		return "cohere"
	default:
		return "unknown"
	}
}

// Provider represents a LLM API provider (global singleton).
// A provider has per-protocol base URLs.
type Provider struct {
	ID       uint
	BaseURLs map[Protocol]string
	state    *ProviderHealthState // Network-layer health + latency
}

// NewProvider creates a new provider with default healthy state.
func NewProvider(id uint, baseURLs map[Protocol]string) *Provider {
	return &Provider{
		ID:       id,
		BaseURLs: baseURLs,
		state:    newProviderHealthState(),
	}
}

// BaseURLFor returns the base URL for the given protocol.
// If no URL is configured for the protocol, falls back to the OpenAI URL.
func (p *Provider) BaseURLFor(proto Protocol) string {
	if url, ok := p.BaseURLs[proto]; ok && url != "" {
		return url
	}
	return p.BaseURLs[ProtocolOpenAI]
}

var priorityOrder = [...]Protocol{ProtocolOpenAI, ProtocolAnthropic, ProtocolGemini, ProtocolCohere}

// PrimaryBaseURL returns the primary base URL (first non-empty in map).
func (p *Provider) PrimaryBaseURL() string {
	for _, proto := range priorityOrder {
		if url, ok := p.BaseURLs[proto]; ok && url != "" {
			return url
		}
	}
	return ""
}

// SingleBaseURL creates a BaseURLs map with a single OpenAI protocol entry.
// Convenience for providers that only support the OpenAI protocol.
func SingleBaseURL(url string) map[Protocol]string {
	return map[Protocol]string{ProtocolOpenAI: url}
}

// IsCircuitBreakerOpen returns true if the provider-level circuit breaker is open.
func (p *Provider) IsCircuitBreakerOpen() bool {
	return p.state.IsCircuitBreakerOpen()
}

// MarkProviderFail marks a provider-level failure (network error).
func (p *Provider) MarkProviderFail() {
	p.state.MarkFail()
}

// MarkProviderSuccess marks a provider-level success.
func (p *Provider) MarkProviderSuccess() {
	p.state.MarkSuccess()
}

// ProviderLatencyEWMA returns the EWMA latency for the provider.
func (p *Provider) ProviderLatencyEWMA() int {
	return p.state.LatencyEWMA()
}

// UpdateProviderLatency updates the EWMA latency for the provider.
func (p *Provider) UpdateProviderLatency(latencyMs int) {
	p.state.UpdateLatency(latencyMs)
}
