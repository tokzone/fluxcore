// Package endpoint provides endpoint definition and registry.
//
// An Endpoint represents a Provider + Model combination (global singleton).
// Endpoint health state is shared across all users using this endpoint.
//
// Core types:
//   - Endpoint: Provider + Model + HealthState (global singleton)
//   - EndpointHealthState: Model-layer health and inference latency
//   - Registry: Global endpoint registry for lookup
//
// Health layers:
//   - Provider layer (network): connection refused, DNS failure
//   - Endpoint layer (model): 429 rate limit, 500 server error
//
// # Registry
//
// Global Registry is a singleton using sync.Map for lock-free read operations:
//
//	endpoint.RegisterEndpoint(1, prov, "")  // Register to global registry
//	ep := endpoint.GlobalRegistry().GetByProviderModel(prov, "")  // Lookup
//
// Important: Endpoint MUST be registered BEFORE creating UserEndpoint:
//
//	// Correct order:
//	endpoint.RegisterEndpoint(1, openai, "")       // 1. Register first
//	key, _ := flux.NewAPIKey(openai, "sk-xxx")     // 2. Create APIKey
//	ue, _ := flux.NewUserEndpoint(openai, "", key, 1000)  // 3. Create UserEndpoint
//
//	// Wrong order will fail:
//	key, _ := flux.NewAPIKey(openai, "sk-xxx")
//	ue, err := flux.NewUserEndpoint(openai, "", key, 1000)  // Error: endpoint not registered
//	// RegisterEndpoint must come first
//
// # Example
//
//	prov := provider.NewProvider(1, "https://api.openai.com", provider.ProtocolOpenAI)
//	ep := endpoint.RegisterEndpoint(1, prov, "")  // Register to global registry
//	// Or use custom registry:
//	reg := endpoint.NewRegistry()
//	reg.Register(ep)
//	// Lookup:
//	ep := reg.GetByProviderModel(prov, "")
package endpoint
