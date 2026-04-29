# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/).

## [1.0.0] - 2026-04-28

### Added — DDD Architecture

- **ServiceEndpoint aggregate**: immutable `Service` VO + network-layer `CircuitBreaker` (threshold=1, recovery=120s)
- **Route aggregate**: `RouteDesc` VO + model-layer `CircuitBreaker` (threshold=3, recovery=60s). Identity via `IdentityKey()` = `hash(ServiceName, Model, Credential)`
- **RouteTable value object**: immutable, pre-computed snapshot. `Select()` returns first available route by priority (equal-priority randomly shuffled). Target protocol determined at construction time via `ProtocolPriority()`.
- **Router domain service**: `Do()` / `Stream()` for single requests, `Execute()` / `ExecuteStream()` for retry + failover. Two-layer health feedback (network → ServiceEndpoint, model → Route).
- **RouteRepository**: caches `Route` aggregates by identity key. `FindOrCreate()` with double-check locking. TTL 300s, capacity 50000, background cleanup. Enables CB state survival across config reloads.
- **ParseProtocol**: reverse of `Protocol.String()`. Single source of truth for protocol name ↔ enum mapping.
- **ProtocolPriority**: deterministic ordering [OpenAI, Anthropic, Gemini, Cohere] for protocol fallback selection.
- **CircuitBreaker**: extracted to `internal/health/` domain primitive. Three-state: Closed → Open → HalfOpen → Closed. Configurable threshold and recovery per aggregate.

### Changed

- `Protocol` enum moved to top-level package (`fluxcore.go`)
- `Service` VO with `BaseURLs map[Protocol]string` + `BaseURLFor()` method
- Health feedback: `Router.Do()` auto-classifies errors (network vs model vs 4xx non-429) and routes to correct CB
- Backoff: exponential with jitter, configurable base/max in router

### Removed

- `provider/` package — `Provider`, `ProviderHealthState`, `Protocol` constants
- `endpoint/` package — `Endpoint`, `EndpointHealthState`, `GlobalRegistry`, `RegisterEndpoint`
- `flux/` package — `APIKey`, `UserEndpoint`, `Client`, `DoFunc`, `DoFuncGen`, `StreamDoFuncGen`
- All global singletons (no more `sync.Map` registry)

### Migration Guide (v0.9.0 → v1.0.0)

```go
// Before (v0.9.0)
openai := provider.NewProvider(1, "https://api.openai.com")
endpoint.RegisterEndpoint(1, openai, "", []provider.Protocol{provider.ProtocolOpenAI})
key, _ := flux.NewAPIKey(openai, "sk-xxx")
ue, _ := flux.NewUserEndpoint("", key, 1000)
client := flux.NewClient([]*flux.UserEndpoint{ue}, flux.WithRetryMax(3))
doFunc := flux.DoFuncGen(client, provider.ProtocolOpenAI)
resp, usage, url, err := doFunc(ctx, rawReq)

// After (v1.0.0)
openaiSE := fluxcore.NewServiceEndpoint(fluxcore.Service{
    Name: "openai",
    BaseURLs: map[fluxcore.Protocol]string{fluxcore.ProtocolOpenAI: "https://api.openai.com"},
})
repo := fluxcore.NewRouteRepository()
route := repo.FindOrCreate(
    fluxcore.RouteDesc{SvcEP: openaiSE, Model: "gpt-4", Credential: "sk-xxx", Priority: 0}.IdentityKey(),
    func() *fluxcore.Route {
        return fluxcore.NewRoute(fluxcore.RouteDesc{
            SvcEP: openaiSE, Model: "gpt-4", Credential: "sk-xxx", Priority: 0,
        })
    },
)
table := fluxcore.NewRouteTable([]*fluxcore.Route{route}, fluxcore.ProtocolOpenAI)
router := fluxcore.NewRouter(fluxcore.ProtocolOpenAI)
route, resp, usage, err := router.Execute(ctx, table, rawReq, 3)
```

## [0.9.0] - 2026-04-26

### Added
- **WithHTTPClient Option**: Allow custom HTTP client injection
  - `flux.WithHTTPClient(httpClient *http.Client)` lets users configure connection pool, timeout, etc.
  - Default: built-in `sharedClient` with sensible defaults (30s timeout, 100 max idle conns)

### Changed
- **transport/streamTransport signature**: Now accept `client *http.Client` parameter
  - Before: `transport(ctx, ue, body)`
  - After: `transport(ctx, ue, body, client)`
- **streamTransport deadline check**: Now checks for existing deadline before creating context
  - Consistent with `transport` behavior, avoids unnecessary context allocation

## [0.8.0] - 2024-04-25

### Changed
- **API simplification**: `NewUserEndpoint` signature changed
  - Before: `NewUserEndpoint(prov *provider.Provider, model string, apiKey *APIKey, priority int64)`
  - After: `NewUserEndpoint(model string, apiKey *APIKey, priority int64)`
- **SSE safety improvement**: buffer overflow now processes data before resetting
- **Code cleanup**: removed redundant WHAT comments from accessor methods
- **Performance**: SSE parsing uses package-level `sseSuffix` constant

## [0.7.0] - 2024-04-25

### Changed
- Architecture refactor for multi-tenant design
- Renamed `Router` to `Client`, `Router.Request()` to `Client.Do()`
- Renamed `endpoint.EndpointHealth` to `endpoint.Endpoint`
- Renamed `flux.ProviderKey` to `flux.APIKey`
- Renamed `flux.Endpoint` to `flux.UserEndpoint`
- Health state split into two layers: Provider (network) + Endpoint (model)
- Registry uses `sync.Map` for lock-free read

## [0.5.0] - 2024-04-24

### Changed
- Removed EstimateTokens function
- Made RetryConfig internal
- Removed unused ImageContent, AudioContent, Clone, WithModel, WithMaxTokens
- Made circuit breaker config internal
- Made SSE internal types private
- Simplified routing: one smart algorithm replaces 3 strategies
- Removed deprecated HasUsage, ShouldSkip, AsImage, AsAudio

### Added
- Stability tests for circuit breaker recovery, EWMA latency, network resilience
- IsText(), ExtractAllText() helper functions
- Package-level doc.go for all packages
- 90%+ test coverage

### Fixed
- Validate() now only requires Model for Gemini protocol
