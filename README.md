# fluxcore ⚡

**LLM API Client Library**

[![Go Version](https://img.shields.io/badge/Go-1.21+-00ADD8?style=flat)](https://golang.org)
[![License](https://img.shields.io/badge/License-MIT-green?style=flat)](LICENSE)
[![Version](https://img.shields.io/badge/Version-v0.9.0-blue?style=flat)]()
[![中文](https://img.shields.io/badge/README-中文-red?style=flat)](README_CN.md)

Simple LLM API client with routing and health management.

---

## Quick Start

```go
import (
    "github.com/tokzone/fluxcore/provider"
    "github.com/tokzone/fluxcore/endpoint"
    "github.com/tokzone/fluxcore/flux"
)

// 1. Define global providers
openai := provider.NewProvider(1, "https://api.openai.com", provider.ProtocolOpenAI)
anthropic := provider.NewProvider(2, "https://api.anthropic.com", provider.ProtocolAnthropic)

// 2. Register endpoints to global registry
endpoint.RegisterEndpoint(1, openai, "")
endpoint.RegisterEndpoint(2, anthropic, "")

// 3. Create APIKeys (Provider + Secret)
key1, _ := flux.NewAPIKey(openai, "sk-xxx")
key2, _ := flux.NewAPIKey(anthropic, "sk-ant-xxx")

// 4. Create UserEndpoints (Endpoint + APIKey + Priority)
ue1, _ := flux.NewUserEndpoint("", key1, 1000)
ue2, _ := flux.NewUserEndpoint("", key2, 800)

// 5. Create client (default HTTP client)
client := flux.NewClient([]*flux.UserEndpoint{ue1, ue2}, flux.WithRetryMax(3))

// Or with custom HTTP client
customHTTP := &http.Client{Timeout: 60 * time.Second}
client := flux.NewClient([]*flux.UserEndpoint{ue1, ue2}, 
    flux.WithRetryMax(3),
    flux.WithHTTPClient(customHTTP))

// 6. Send request
resp, usage, err := client.Do(ctx, rawReq, provider.ProtocolOpenAI)

// 7. Streaming
result, err := client.DoStream(ctx, rawReq, provider.ProtocolAnthropic)
defer result.Close()
for chunk := range result.Ch {
    // process chunk
}
```

---

## Features

- **Simple API** — Provider, Endpoint, APIKey, UserEndpoint, Client. Five concepts.
- **Multi-Tenant** — Shared health state (Provider/Endpoint), private secrets (APIKey) and priorities (UserEndpoint).
- **Two-Layer Health** — Provider (network) + Endpoint (model) circuit breakers.
- **Protocol Conversion** — Anthropic in, Gemini out, transparent translation.
- **Custom HTTP Client** — Inject custom client for connection pool tuning.

---

## Options

```go
// Retry configuration
flux.WithRetryMax(5)  // Max retries (default: 3)

// Custom HTTP client
flux.WithHTTPClient(&http.Client{
    Timeout: 60 * time.Second,
    Transport: &http.Transport{
        MaxIdleConns:        200,
        MaxIdleConnsPerHost: 20,
        IdleConnTimeout:     120 * time.Second,
    },
})
```

---

## Module Architecture

```
flux (user entry)
  │
  └── Client.Do() / DoStream()
  │
flux (user data)
  │
  ├── APIKey: Provider + Secret (user private)
  └── UserEndpoint: Endpoint + APIKey + Priority (user private)
  │
endpoint (global state)
  │
  └── Endpoint: Provider + Model + Health (global singleton)
  │
provider (global state)
  │
  └── Provider: URL + Protocol + Health (global singleton)
```

---

## Two-Layer Circuit Breaker

```
Provider Layer (Network):
  Connection refused → Immediate circuit (threshold=1)
  Recovery: 120s

Endpoint Layer (Model):
  429 Rate Limit → Circuit (threshold=1)
  500 Server Error → Circuit (threshold=3)
  Recovery: 60s
```

---

## License

MIT