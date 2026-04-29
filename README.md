# fluxcore

**LLM API Routing Engine**

[![Go Version](https://img.shields.io/badge/Go-1.21+-00ADD8?style=flat)](https://golang.org)
[![License](https://img.shields.io/badge/License-MIT-green?style=flat)](LICENSE)
[![Version](https://img.shields.io/badge/Version-v1.0.0-blue?style=flat)]()
[![中文](https://img.shields.io/badge/README-中文-red?style=flat)](README_CN.md)

Domain-driven routing engine for LLM API requests with two-layer circuit breaking and protocol translation.

---

## Quick Start

```go
import "github.com/tokzone/fluxcore"

// 1. Create ServiceEndpoints (one per external AI service)
openaiSE := fluxcore.NewServiceEndpoint(fluxcore.Service{
    Name:     "openai",
    BaseURLs: map[fluxcore.Protocol]string{fluxcore.ProtocolOpenAI: "https://api.openai.com"},
})
anthropicSE := fluxcore.NewServiceEndpoint(fluxcore.Service{
    Name:     "anthropic",
    BaseURLs: map[fluxcore.Protocol]string{fluxcore.ProtocolAnthropic: "https://api.anthropic.com"},
})

// 2. Create Routes (one per model+credential combination)
routes := []*fluxcore.Route{
    fluxcore.NewRoute(fluxcore.RouteDesc{
        SvcEP: openaiSE, Model: "gpt-4", Credential: "sk-xxx", Priority: 0,
    }),
    fluxcore.NewRoute(fluxcore.RouteDesc{
        SvcEP: anthropicSE, Model: "claude-3", Credential: "sk-ant-xxx", Priority: 10,
    }),
}

// 3. Build a RouteTable (pre-computed, immutable)
table := fluxcore.NewRouteTable(routes, fluxcore.ProtocolOpenAI)

// 4. Execute with retry and failover
router := fluxcore.NewRouter(fluxcore.ProtocolOpenAI)
route, resp, usage, err := router.Execute(ctx, table, rawReq, 3)

// 5. Streaming
route, result, err := router.ExecuteStream(ctx, table, rawReq, 3)
defer result.Close()
for chunk := range result.Ch {
    // process chunk
}
```

---

## Core Concepts

### ServiceEndpoint (Aggregate Root)

Represents an external AI service. Holds an immutable `Service` value object and a network-layer circuit breaker (threshold=1, recovery=120s). Multiple `Route` instances can share a reference to the same `ServiceEndpoint`.

```go
se := fluxcore.NewServiceEndpoint(fluxcore.Service{
    Name:     "deepseek",
    BaseURLs: map[fluxcore.Protocol]string{
        fluxcore.ProtocolOpenAI:    "https://api.deepseek.com",
        fluxcore.ProtocolAnthropic: "https://api.deepseek.com/anthropic",
    },
})
se.IsAvailable()     // CB state
se.Service().Name    // "deepseek"
```

### Route (Aggregate Root)

Represents a specific model route through a service. Identified by `IdentityKey()` = `hash(ServiceName, Model, Credential)`. Holds a model-layer circuit breaker (threshold=3, recovery=60s).

```go
route := fluxcore.NewRoute(fluxcore.RouteDesc{
    SvcEP:      se,
    Model:      "gpt-4",
    Credential: "sk-xxx",
    Priority:   0,  // lower = higher priority
})
route.IdentityKey()        // "deepseek/gpt-4/sk-xxx"
route.IsAvailable()        // SvcEP.IsAvailable() && route CB closed
```

### RouteTable (Value Object)

An immutable, pre-computed snapshot of routes. Constructed once, then `Select()` is O(n) over available routes. Routes are sorted by priority; equal-priority routes are randomly shuffled.

```go
table := fluxcore.NewRouteTable(routes, fluxcore.ProtocolOpenAI)
route, targetProto := table.Select()  // first available route
```

### Router (Domain Service)

Stateless service that executes requests through a `RouteTable`. Handles protocol translation, HTTP transport, retry with backoff, and two-layer health feedback.

```go
router := fluxcore.NewRouter(fluxcore.ProtocolOpenAI, fluxcore.WithHTTPClient(customClient))
route, resp, usage, err := router.Execute(ctx, table, body, maxRetry)
```

### RouteRepository

Caches `Route` aggregates by identity key, enabling circuit breaker state to survive config reloads and request cycles.

```go
repo := fluxcore.NewRouteRepository()
defer repo.Close()

// On reload: existing routes are reused, CB state preserved
route := repo.FindOrCreate(desc.IdentityKey(), func() *fluxcore.Route {
    return fluxcore.NewRoute(desc)
})
```

---

## Two-Layer Circuit Breaker

```
ServiceEndpoint layer (network):
  DNS / Connection refused / Timeout → Immediate trip (threshold=1)
  Recovery: 120s

Route layer (model):
  429 Rate Limit → Trip (threshold=3 cumulative)
  500 Server Error → Trip (threshold=3 cumulative)
  Recovery: 60s
  Note: 4xx non-429 errors do NOT trip any circuit breaker
```

Health feedback happens automatically in `Router.Do()`:

```
Success: route.MarkSuccess() + route.SvcEP().MarkSuccess()
Network error: route.SvcEP().MarkNetworkFailure()
429/5xx: route.MarkModelFailure()
4xx non-429: no CB change
```

---

## Protocol Translation

When the input protocol doesn't match what the service supports, `RouteTable` pre-computes the target protocol at construction time using `ProtocolPriority()` (OpenAI > Anthropic > Gemini > Cohere). `Router` handles the translation transparently.

```go
// Input: Anthropic request, Service only supports OpenAI
// RouteTable.Select() returns targetProto = ProtocolOpenAI
// Router.Do() translates Anthropic → OpenAI → response back to Anthropic
```

---

## Options

```go
// Custom HTTP client
fluxcore.WithHTTPClient(&http.Client{
    Timeout: 60 * time.Second,
    Transport: &http.Transport{
        MaxIdleConns:        200,
        MaxIdleConnsPerHost: 20,
        IdleConnTimeout:     120 * time.Second,
    },
})
```

---

## Package Structure

```
fluxcore/
├── fluxcore.go           # Protocol, Model, Service, ParseProtocol, ProtocolPriority
├── service_endpoint.go   # ServiceEndpoint aggregate (network CB)
├── route.go              # RouteDesc, Route aggregate (model CB)
├── table.go              # RouteTable value object (pre-computed, immutable)
├── router.go             # Router domain service (Do, Stream, Execute, ExecuteStream)
├── route_repo.go         # RouteRepository (FindOrCreate, TTL 300s, max 50000)
├── errors/               # Error classification (IsRetryable, IsNetworkError, IsModelError)
├── message/              # Intermediate representation types (MessageRequest, MessageResponse, Usage)
└── internal/
    ├── health/           # CircuitBreaker (three-state: Closed → Open → HalfOpen → Closed)
    ├── translate/        # Protocol translators (OpenAI, Anthropic, Gemini, Cohere) + SSE parsing
    └── httpclient/       # Shared HTTP client
```

---

## Integration Patterns

### Single-tenant (tokrouter CLI proxy)

```go
// Startup
svcEPs := map[string]*fluxcore.ServiceEndpoint{...}
repo := fluxcore.NewRouteRepository()
oaRouter := fluxcore.NewRouter(fluxcore.ProtocolOpenAI)

// Build tables from config
routes := configToRoutes(cfg, svcEPs, repo)
tables := make(map[fluxcore.Model]*fluxcore.RouteTable)
for model, routes := range groupByModel(routes) {
    tables[model] = fluxcore.NewRouteTable(routes, fluxcore.ProtocolOpenAI)
}

// Hot path
table := tables[fluxcore.Model(model)]
route, resp, usage, err := oaRouter.Execute(ctx, table, body, maxRetry)

// Reload: svcEPs and repo survive → CB state preserved
```

### Multi-tenant (tokhub SaaS gateway)

```go
// Cache strategy: RouteTable (10s TTL) + RouteRepository (300s TTL)
table := routeTableCache.Get(cacheKey)
if table == nil {
    records := endpointRepo.GetActiveByUser(ctx, userID, model)
    routes := builder.BuildRoutes(records, svcEPs, routeRepo)
    // BuildRoutes: decrypt credential → RouteDesc → repo.FindOrCreate
    table = fluxcore.NewRouteTable(routes, inputProto)
    routeTableCache.Set(cacheKey, table, 10*time.Second)
}
route, resp, usage, err := router.Execute(ctx, table, body, maxRetry)
```

---

## License

MIT
