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
// A provider is identified by its BaseURL and Protocol.
type Provider struct {
	ID       uint
	BaseURL  string
	Protocol Protocol             // Protocol format (OpenAI, Anthropic, Gemini, Cohere)
	state    *ProviderHealthState // Network-layer health + latency
}

// NewProvider creates a new provider with default healthy state.
func NewProvider(id uint, baseURL string, protocol Protocol) *Provider {
	return &Provider{
		ID:       id,
		BaseURL:  baseURL,
		Protocol: protocol,
		state:    newProviderHealthState(),
	}
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
