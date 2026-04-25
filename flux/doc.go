// Package flux provides the main client for LLM API routing.
//
// Flux is the user's entry point for making LLM API requests with automatic
// routing, health management, and protocol conversion.
//
// Core types:
//   - Client: Main client for making requests
//   - APIKey: User's API key for a Provider (Provider + Secret)
//   - UserEndpoint: User's request endpoint (Endpoint + APIKey + Priority)
//   - StreamResult: Result of streaming requests
//
// # Architecture
//
// Flux follows a layered architecture:
//
//	Global Layer (shared singletons):
//	  - Provider: URL + Protocol + network health
//	  - Endpoint: Provider + Model + model health
//	  - Registry: global endpoint registry (sync.Map, lock-free)
//
//	User Layer (private instances):
//	  - APIKey: Provider reference + Secret
//	  - UserEndpoint: Endpoint reference + APIKey reference + Priority
//	  - Client: []UserEndpoint + cache + retry logic
//
// # Quick Start
//
// Basic usage:
//
//	// 1. Define global providers
//	openai := provider.NewProvider(1, "https://api.openai.com", provider.ProtocolOpenAI)
//	anthropic := provider.NewProvider(2, "https://api.anthropic.com", provider.ProtocolAnthropic)
//
//	// 2. Register endpoints to global registry
//	endpoint.RegisterEndpoint(1, openai, "")
//	endpoint.RegisterEndpoint(2, anthropic, "")
//
//	// 3. Create APIKeys (Provider + Secret)
//	key1, _ := flux.NewAPIKey(openai, "sk-xxx")
//	key2, _ := flux.NewAPIKey(anthropic, "sk-ant-xxx")
//
//	// 4. Create UserEndpoints (Endpoint + APIKey + Priority)
//	ue1, _ := flux.NewUserEndpoint("", key1, 1000)
//	ue2, _ := flux.NewUserEndpoint(anthropic, "", key2, 800)
//
//	// 5. Create Client
//	client := flux.NewClient([]*flux.UserEndpoint{ue1, ue2}, flux.WithRetryMax(3))
//
//	// 6. Send request
//	resp, usage, err := client.Do(ctx, rawReq, provider.ProtocolOpenAI)
//
// # Streaming
//
// For streaming responses:
//
//	result, err := client.DoStream(ctx, rawReq, provider.ProtocolOpenAI)
//	defer result.Close()
//	for chunk := range result.Ch {
//	    // process chunk
//	}
//	usage := result.Usage()
//
// # Priority
//
// Lower priority value = higher preference:
//
//	ue1, _ := flux.NewUserEndpoint("", key1, 100)   // Most preferred
//	ue2, _ := flux.NewUserEndpoint("", key2, 500)   // Normal
//	ue3, _ := flux.NewUserEndpoint("", key3, 1000)  // Least preferred
//
// # Health Management
//
// Two-layer circuit breaker:
//
//	Provider Layer (network):
//	  - Connection refused -> immediate circuit (threshold=1)
//	  - Recovery: 120s
//
//	Endpoint Layer (model):
//	  - 429/500 errors -> circuit after 3 failures (threshold=3)
//	  - Recovery: 60s
//
// # Protocol Conversion
//
// Flux transparently converts between protocols:
//
//	// Request in Anthropic format, response in OpenAI format
//	client.Do(ctx, anthropicReq, provider.ProtocolAnthropic)
//
// Supported conversions:
//   - OpenAI <-> Anthropic
//   - OpenAI <-> Gemini
//   - OpenAI <-> Cohere
//
// # Error Handling
//
// Errors are classified with structured codes:
//
//	resp, _, err := client.Do(ctx, rawReq, provider.ProtocolOpenAI)
//	if err != nil {
//	    var classified *errors.ClassifiedError
//	    if errors.As(err, &classified) {
//	        switch classified.Code {
//	        case errors.CodeRateLimit:
//	            // rate limit - retryable
//	        case errors.CodeAuthError:
//	            // auth error - not retryable
//	        }
//	    }
//	}
//
// Retryable errors: timeout, network_error, rate_limit, server_error, model_error
// Non-retryable errors: auth_error, invalid_request
//
// # Provider Validation
//
// When creating UserEndpoint, Provider must match between Endpoint and APIKey:
//
//	key1, _ := flux.NewAPIKey(openai, "sk-xxx")
//	key2, _ := flux.NewAPIKey(anthropic, "sk-ant-xxx")
//
//	ue1, _ := flux.NewUserEndpoint("", key1, 1000)  // OK: openai == openai
//	ue2, err := flux.NewUserEndpoint("", key2, 1000) // Error: openai != anthropic
package flux
