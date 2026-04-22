// Package routing provides endpoint management, selection, and health tracking.
//
// The routing package handles:
//   - Endpoint definition (Key + Model + pricing attributes)
//   - Endpoint selection with cost-based optimization
//   - Circuit breaker pattern for health management
//   - Thread-safe endpoint pool with atomic updates
//
// Core types:
//   - Key: Connection credentials (BaseURL, APIKey, Protocol)
//   - Endpoint: Routing unit with health state
//   - EndpointPool: Collection of endpoints with selection logic
//   - Protocol: LLM provider protocol (OpenAI, Anthropic, Gemini, Cohere)
//
// Example usage:
//
//	key := &routing.Key{
//	    BaseURL:  "https://api.openai.com/v1",
//	    APIKey:   "sk-xxx",
//	    Protocol: routing.ProtocolOpenAI,
//	}
//	ep := routing.NewEndpoint(1, key, "", 0.01, 0.03)
//	pool := routing.NewEndpointPool([]*routing.Endpoint{ep}, 3)
//	selected := pool.Select()
//
// Health management:
//
//	ep.MarkSuccess()  // Reset failure count
//	ep.MarkFail()     // Increment failure count
//	if ep.IsCircuitBreakerOpen() { /* skip endpoint */ }
package routing