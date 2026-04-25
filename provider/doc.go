// Package provider provides LLM API provider definitions.
//
// A Provider represents an API endpoint (BaseURL + Protocol).
// Provider-level health state is shared across all endpoints using this provider.
//
// Core types:
//   - Provider: LLM API provider (BaseURL + Protocol)
//   - Protocol: Communication format (OpenAI, Anthropic, Gemini, Cohere)
//   - ProviderHealthState: Network-layer health and latency
//
// Example:
//
//	p := provider.NewProvider(1, "https://api.openai.com", provider.ProtocolOpenAI)
//	if p.IsCircuitBreakerOpen() {
//	    // Provider network is unhealthy
//	}
package provider
