# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/).

## [0.8.0] - 2024-04-25

### Changed
- **API simplification**: `NewUserEndpoint` signature changed
  - Before: `NewUserEndpoint(prov *provider.Provider, model string, apiKey *APIKey, priority int64)`
  - After: `NewUserEndpoint(model string, apiKey *APIKey, priority int64)`
  - Provider is now inferred from `apiKey.Provider()`, eliminating redundant parameter
- **SSE safety improvement**: buffer overflow now processes data before resetting (prevents data loss)
- **Code cleanup**: removed redundant WHAT comments from accessor methods
- **Performance**: SSE parsing uses package-level `sseSuffix` constant instead of allocating per call

### Migration Guide (v0.7.0 → v0.8.0)

```go
// Before (v0.7.0)
ue, _ := flux.NewUserEndpoint(openai, "", key, 1000)

// After (v0.8.0)
ue, _ := flux.NewUserEndpoint("", key, 1000)  // Provider inferred from key
```

## [0.7.0] - 2024-04-25

### Changed
- **Architecture refactor for multi-tenant design**
- Renamed `Router` to `Client` (user perspective: entry point)
- Renamed `Request()` to `Client.Do()`, `RequestStream()` to `Client.DoStream()`
- Renamed `endpoint.EndpointHealth` to `endpoint.Endpoint` (core business entity)
- Renamed `flux.ProviderKey` to `flux.APIKey` (semantic completeness)
- Renamed `flux.Endpoint` to `flux.UserEndpoint` (avoid naming conflict)
- Health state split into two layers: Provider (network) + Endpoint (model)
- Registry uses `sync.Map` for lock-free read operations
- SSE processing uses bytes operations to reduce GC pressure

### Added
- **Endpoint Registry**: global singleton for Provider + Model → Endpoint lookup
- **flux package**: new user entry point with Client, APIKey, UserEndpoint
- Provider validation in NewUserEndpoint: ensures Endpoint.Provider == APIKey.Provider
- Provider-level circuit breaker: threshold=1, recovery=120s (network errors)
- Endpoint-level circuit breaker: threshold=3, recovery=60s (model errors)
- Error classification: connection refused → Provider, 429/500 → Endpoint

### Removed
- `call` package (functionality moved to flux.Client)
- `router` package (functionality moved to flux.Client)
- `pool` package (replaced by flux.APIKey + flux.UserEndpoint)
- `endpoint.Key` type (replaced by flux.APIKey)
- `endpoint.Protocol` (moved to provider package)
- `endpoint.NewPool` (Pool no longer needed)
- `router.WithPriority` option (priority now in UserEndpoint)

### Migration Guide (v0.6.0 → v0.7.0)

```go
// Before (v0.6.0)
keys := []*endpoint.Key{
    {BaseURL: "https://api.openai.com", APIKey: "sk-xxx", Protocol: endpoint.ProtocolOpenAI},
}
ep, _ := endpoint.NewEndpoint(1, keys[0], "")
pool := endpoint.NewPool([]*endpoint.Endpoint{ep})
r := router.NewRouter(pool, router.WithRetryMax(3), router.WithPriority(1, 1000))
call.Request(ctx, r, rawReq, endpoint.ProtocolOpenAI)

// After (v0.7.0)
openai := provider.NewProvider(1, "https://api.openai.com", provider.ProtocolOpenAI)
endpoint.RegisterEndpoint(1, openai, "")
key, _ := flux.NewAPIKey(openai, "sk-xxx")
ue, _ := flux.NewUserEndpoint("", key, 1000)
client := flux.NewClient([]*flux.UserEndpoint{ue}, flux.WithRetryMax(3))
client.Do(ctx, rawReq, provider.ProtocolOpenAI)
```

## [0.5.0] - 2024-04-24

### Changed
- Removed EstimateTokens function (token estimation is application-layer concern)
- Made RetryConfig internal: retryConfig, setRetryConfig, getRetryConfig
- Removed unused ImageContent, AudioContent, Clone, WithModel, WithMaxTokens
- Made circuit breaker config internal (circuitBreakerConfig, newEndpointWithConfig)
- Made SSE internal types private: sseEvent, sseParseResult, chunkParser, registerChunkParser
- Simplified routing: one smart algorithm replaces 3 strategies
- Merged ImageData/AudioData into MediaData with type aliases
- Removed deprecated HasUsage field (use IsAccurate)
- Removed deprecated ShouldSkip method (use IsCircuitBreakerOpen)
- Removed deprecated AsImage/AsAudio methods (use AsMedia)

### Added
- Stability tests for circuit breaker recovery, EWMA latency, network resilience
- IsText(), ExtractAllText() helper functions
- Package-level doc.go for all packages
- Key nil check in NewEndpoint constructors
- 90%+ test coverage (Routing 94.2%, Call 90.8%)

### Fixed
- Validate() now only requires Model for Gemini protocol
- doc.go examples updated to match actual API