# fluxcore ⚡

**LLM API Router Library**

[![Go Version](https://img.shields.io/badge/Go-1.21+-00ADD8?style=flat)](https://golang.org)
[![License](https://img.shields.io/badge/License-MIT-green?style=flat)](LICENSE)
[![Files](https://img.shields.io/badge/Files-17-blue?style=flat)]()

30 lines to route LLM APIs.

---

## Highlights

- **Zero Abstraction** — 17 files, no interface layers. Read the code, understand the flow.
- **Price-First Routing** — Auto-select cheapest available endpoint.
- **Circuit Breaker + Retry** — 3 failures trigger circuit, auto-recovery in 60s.
- **Protocol Conversion** — Anthropic in, Gemini out, transparent translation.

---

## 4 Lines

```go
pool := routing.NewEndpointPool(endpoints, 3)
resp, usage, _ := call.Request(ctx, pool, rawReq, routing.ProtocolOpenAI)
// Done.
```

---

## Quick Start

```go
import (
    "github.com/tokflux/fluxcore/routing"
    "github.com/tokflux/fluxcore/call"
)

// 1. Define keys (connection credentials)
keys := []*routing.Key{
    {BaseURL: "https://api.openai.com", APIKey: key1, Protocol: routing.ProtocolOpenAI},
    {BaseURL: "https://api.anthropic.com", APIKey: key2, Protocol: routing.ProtocolAnthropic},
    {BaseURL: "https://generativelanguage.googleapis.com", APIKey: key3, Protocol: routing.ProtocolGemini},
}

// 2. Create endpoints (key + model + pricing)
// Note: Model is required for Gemini (used in URL), empty for OpenAI/Anthropic/Cohere
endpoints := []*routing.Endpoint{
    routing.NewEndpoint(1, keys[0], "", 0.01, 0.03),            // OpenAI (model from request)
    routing.NewEndpoint(2, keys[1], "", 0.008, 0.024),          // Anthropic (model from request)
    routing.NewEndpoint(3, keys[2], "gemini-pro", 0.001, 0.002), // Gemini (model in URL)
}

// 3. Create pool (auto-select cheapest, auto circuit breaker)
pool := routing.NewEndpointPool(endpoints, 3)

// 4. Non-streaming request
resp, usage, _ := call.Request(ctx, pool, rawReq, routing.ProtocolOpenAI)

// 5. Streaming request (auto protocol conversion)
result, _ := call.RequestStream(ctx, pool, rawReq, routing.ProtocolAnthropic)
for chunk := range result.Ch { c.Write(chunk) }
```

---

## Protocol Conversion

```go
// Frontend: Anthropic SDK format
// Backend: Gemini provider (cheaper)
// fluxcore: auto-converts

anthropicReq := `{"model": "claude-3", "messages": [...]}`
resp, _, _ := call.Request(ctx, pool, anthropicReq, routing.ProtocolAnthropic)
// Output is Anthropic format, even if pool chose Gemini endpoint
```

---

## Price-First Routing

```go
// Create endpoints with pricing
ep1 := routing.NewEndpoint(1, key1, "", 0.01, 0.03)   // OpenAI: $0.01/$0.03
ep2 := routing.NewEndpoint(2, key2, "", 0.001, 0.002) // Gemini: $0.001/$0.002

pool := routing.NewEndpointPool([]*routing.Endpoint{ep1, ep2}, 3)

// Auto-select cheapest available endpoint
// Gemini fails? Auto-switch to OpenAI
```

---

## Circuit Breaker

```
Healthy → Fail → Fail → Fail → 🔴 Circuit Open
                              ↓
                        60s auto-recovery probe
```

3 failures trigger circuit, auto-switch to other endpoints. 60s auto-recovery.

### Default Config

```go
// Default: 3 failures trigger circuit, 60s recovery timeout
ep := routing.NewEndpoint(id, key, model, inputPrice, outputPrice)

// Check circuit breaker status
if ep.IsCircuitBreakerOpen() {
    // Endpoint unhealthy, skip
}
```

### Custom Config

```go
// Custom: 5 failures trigger circuit, 30s recovery timeout
ep := routing.NewEndpointWithConfig(id, key, model, inputPrice, outputPrice, routing.CircuitBreakerConfig{
    Threshold:       5,                // Failures before circuit opens
    RecoveryTimeout: 30 * time.Second, // Time before retrying unhealthy endpoint
})
```

### API Reference

| Method | Description |
|--------|-------------|
| `IsCircuitBreakerOpen()` | Returns true if circuit breaker is open (should skip) |
| `MarkSuccess()` | Mark endpoint as healthy, reset failure count |
| `MarkFail()` | Mark endpoint as failed, increment failure count |

---

## Error Classification

| Error Type | Handling |
|------------|----------|
| Network timeout | Auto retry |
| Service unavailable (503) | Auto retry |
| Auth failed (401) | No retry, return error |
| Rate limited (429) | Wait and retry |

---

## Chinese-Aware Token Estimation

```go
// English: ~4 chars/token (standard)
message.EstimateTokens("Hello world!")  // → 4

// Chinese: ~1.5 chars/token (accurate!)
message.EstimateTokens("你好世界")       // → 3 (not 4)

// Mixed: auto-detect
message.EstimateTokens("Hello 你好")    // → 4
```

Useful for Chinese applications where billing depends on accurate estimation.

---

## Usage Statistics

```go
resp, usage, err := call.Request(ctx, pool, rawReq, routing.ProtocolOpenAI)
if usage != nil {
    fmt.Printf("Input tokens: %d\n", usage.InputTokens)
    fmt.Printf("Output tokens: %d\n", usage.OutputTokens)
    fmt.Printf("Latency: %dms\n", usage.LatencyMs)
    fmt.Printf("Total tokens: %d\n", usage.TotalTokens())

    // Check if usage is accurate (provider reported, not estimated)
    if usage.IsAccurate {
        fmt.Println("Usage is accurate (provider reported)")
    }
}
```

### Usage Fields

| Field | Type | Description |
|-------|------|-------------|
| `InputTokens` | `int` | Input/prompt tokens |
| `OutputTokens` | `int` | Output/completion tokens |
| `LatencyMs` | `int` | Request latency in milliseconds |
| `IsAccurate` | `bool` | True if provider reported accurate usage (not estimated) |

---

## Architecture

```
┌─────────────────────────────────────────────┐
│                 fluxcore                     │
│        LLM API Router Library               │
├─────────────┬───────────────────────────────┤
│  message/   │          routing/             │
│  Types      │     Selection + Circuit Breaker│
│  (IR Layer) │          (Lock-free)          │
├─────────────┴───────────────────────────────┤
│                   call/                      │
│      HTTP Transport + Retry + Streaming     │
│            + Protocol Conversion             │
└─────────────────────────────────────────────┘

4 packages. 17 files. 0 interfaces. 0 dependencies.
```

**Package naming with business semantics:**

| Package | Purpose | Files |
|---------|---------|-------|
| `message` | LLM message data structures | 6 |
| `routing` | Endpoint routing + selection + circuit breaker | 8 |
| `call` | HTTP transport + retry + streaming | 6 |
| `errors` | Error classification + retryability | 2 |

---

## Performance

| Operation | Time | Note |
|-----------|------|------|
| `CurrentEp()` | ~10ns | Lock-free atomic read |
| `MarkFail()` | ~50ns | CAS + O(1) map |
| Concurrent test | 1000 QPS | No deadlock |

---

## Security (SSRF Protection)

fluxcore 提供 SSRF 防护工具，策略你来定。

```go
ep := &routing.Endpoint{
    BaseURL:  userProvidedURL,  // 用户输入
    APIKey:   "your-key",
    Protocol: routing.ProtocolOpenAI,
}

// Step 1: 验证格式（scheme, APIKey, Format）
if err := ep.Validate(); err != nil {
    return err
}

// Step 2: SSRF 防护（可选，你的策略）
u, _ := url.Parse(ep.BaseURL)
if routing.IsPrivateIP(u.Hostname()) {
    return errors.New("private IPs not allowed")  // 你的策略
}
```

---

## Who Uses Fluxcore

| User | Use Case |
|------|----------|
| **SaaS Teams** | Multi-tenant LLM features with endpoint isolation |
| **AI Startups** | Cost-optimized routing (cheapest provider first) |
| **Platform Teams** | Unified LLM API for internal services |
| **Indie Developers** | Prototype to production in hours, not weeks |

---

## Protocol Support

| Format | Constant | Endpoint |
|--------|----------|----------|
| **OpenAI** | `ProtocolOpenAI` | `/v1/chat/completions` |
| **Anthropic** | `ProtocolAnthropic` | `/v1/messages` |
| **Gemini** | `ProtocolGemini` | `/v1/models/{model}:generateContent` |
| **Cohere** | `ProtocolCohere` | `/v1/chat` |

### OpenAI-Compatible Providers

| Provider | Base URL |
|----------|----------|
| **Azure OpenAI** | Your Azure endpoint |
| **Mistral AI** | `https://api.mistral.ai` |
| **Groq** | `https://api.groq.com` |
| **DeepSeek** | `https://api.deepseek.com` |
| **Zhipu GLM-4** | `https://open.bigmodel.cn/api/paas/v4/` |

---

## Integration Example

```go
package main

import (
    "io"
    "github.com/tokflux/fluxcore/routing"
    "github.com/tokflux/fluxcore/call"
    "github.com/gin-gonic/gin"
)

func main() {
    keys := []*routing.Key{
        {BaseURL: "https://api.openai.com", APIKey: "sk-xxx", Protocol: routing.ProtocolOpenAI},
        {BaseURL: "https://api.anthropic.com", APIKey: "sk-yyy", Protocol: routing.ProtocolAnthropic},
    }

    endpoints := []*routing.Endpoint{
        routing.NewEndpoint(1, keys[0], "", 0.01, 0.03),
        routing.NewEndpoint(2, keys[1], "", 0.008, 0.024),
    }

    pool := routing.NewEndpointPool(endpoints, 3)

    r := gin.Default()
    r.POST("/v1/chat/completions", func(c *gin.Context) {
        rawReq, _ := io.ReadAll(c.Request.Body)
        resp, usage, err := call.Request(c.Request.Context(), pool, rawReq, routing.ProtocolOpenAI)
        if err != nil {
            c.JSON(500, gin.H{"error": err.Error()})
            return
        }
        c.Data(200, "application/json", resp)
    })

    r.Run(":8080")
}
```

---

## Get Started

```bash
go get github.com/tokflux/fluxcore
```

**Next steps:**
1. Try the Quick Start above
2. See [Integration Example](#integration-example)
3. ⭐ Star if helpful

---

## 中文说明

### fluxcore ⚡

**LLM API 路由库**

30行代码，LLM API 路由搞定。

---

### 特性

- **零抽象** — 17个文件，无接口层。读代码，懂流程。
- **价格优先路由** — 自动选择最便宜可用端点。
- **熔断器 + 重试** — 3次失败触发熔断，60秒自动恢复。
- **协议转换** — Anthropic 输入，Gemini 输出，透明翻译。

---

### 4行代码起步

```go
pool := routing.NewEndpointPool(endpoints, 3)
resp, usage, _ := call.Request(ctx, pool, rawReq, routing.ProtocolOpenAI)
// 完成。
```

---

### 快速开始

```go
import (
    "github.com/tokflux/fluxcore/routing"
    "github.com/tokflux/fluxcore/call"
)

// 1. 定义密钥（连接凭证）
keys := []*routing.Key{
    {BaseURL: "https://api.openai.com", APIKey: key1, Protocol: routing.ProtocolOpenAI},
    {BaseURL: "https://api.anthropic.com", APIKey: key2, Protocol: routing.ProtocolAnthropic},
}

// 2. 创建端点（密钥 + 模型 + 定价）
// 注意：Model 参数对 Gemini 必填（用于 URL），OpenAI/Anthropic/Cohere 用空字符串 ""
endpoints := []*routing.Endpoint{
    routing.NewEndpoint(1, keys[0], "", 0.01, 0.03),            // OpenAI（模型从请求获取）
    routing.NewEndpoint(2, keys[1], "", 0.008, 0.024),          // Anthropic（模型从请求获取）
    routing.NewEndpoint(3, keys[2], "gemini-pro", 0.001, 0.002), // Gemini（模型在 URL 中）
}

// 3. 创建池（自动选择最便宜，自动熔断）
pool := routing.NewEndpointPool(endpoints, 3)

// 4. 非流式请求
resp, usage, _ := call.Request(ctx, pool, rawReq, routing.ProtocolOpenAI)

// 5. 流式请求（协议自动转换）
result, _ := call.RequestStream(ctx, pool, rawReq, routing.ProtocolAnthropic)
for chunk := range result.Ch { c.Write(chunk) }
```

---

### 协议透明转换

```go
// 前端: Anthropic SDK 格式
// 后端: Gemini provider（更便宜）
// fluxcore: 自动转换

anthropicReq := `{"model": "claude-3", "messages": [...]}`
resp, _, _ := call.Request(ctx, pool, anthropicReq, routing.ProtocolAnthropic)
// 输出是 Anthropic 格式，即使 pool 选择了 Gemini endpoint
```

---

### 成本优先路由

```go
// 创建端点，标注价格
ep1 := routing.NewEndpoint(1, key1, "", 0.01, 0.03)   // OpenAI: $0.01/$0.03
ep2 := routing.NewEndpoint(2, key2, "", 0.001, 0.002) // Gemini: $0.001/$0.002

pool := routing.NewEndpointPool([]*routing.Endpoint{ep1, ep2}, 3)

// 自动选择最便宜可用端点
// Gemini 失败？自动切换到 OpenAI
```

---

### 熔断器自愈

```
健康 → 失败 → 失败 → 失败 → 🔴 熔断
                              ↓
                        60秒自动恢复探测
```

3次失败触发熔断，自动切换到其他端点。60秒后自动恢复探测。

**默认配置：**
```go
// 默认：3次失败触发熔断，60秒恢复超时
ep := routing.NewEndpoint(id, key, model, inputPrice, outputPrice)

// 检查熔断器状态
if ep.IsCircuitBreakerOpen() {
    // 端点不健康，跳过
}
```

**自定义配置：**
```go
// 自定义：5次失败触发熔断，30秒恢复超时
ep := routing.NewEndpointWithConfig(id, key, model, inputPrice, outputPrice, routing.CircuitBreakerConfig{
    Threshold:       5,                // 触发熔断的失败次数
    RecoveryTimeout: 30 * time.Second, // 重试不健康端点前的等待时间
})
```

---

### 智能错误分类

| 错误类型 | 处理方式 |
|----------|----------|
| 网络超时 | 自动重试 |
| 服务不可用 (503) | 自动重试 |
| 认证失败 (401) | 不重试，直接报错 |
| 配额超限 (429) | 等待后重试 |

---

### 中文智能 Token 估算

```go
// 英文: ~4 字符/token（标准）
message.EstimateTokens("Hello world!")  // → 4

// 中文: ~1.5 字符/token（精确！）
message.EstimateTokens("你好世界")       // → 3（不是 4）

// 混合: 自动检测
message.EstimateTokens("Hello 你好")    // → 4
```

对于需要精确计费的中文应用至关重要。

---

### 使用统计

```go
resp, usage, err := call.Request(ctx, pool, rawReq, routing.ProtocolOpenAI)
if usage != nil {
    fmt.Printf("输入 tokens: %d\n", usage.InputTokens)
    fmt.Printf("输出 tokens: %d\n", usage.OutputTokens)
    fmt.Printf("延迟: %dms\n", usage.LatencyMs)
    fmt.Printf("总 tokens: %d\n", usage.TotalTokens())

    // 检查使用量是否精确（Provider 报告，非估算）
    if usage.IsAccurate {
        fmt.Println("使用量精确（Provider 报告）")
    }
}
```

---

### 架构

```
┌─────────────────────────────────────────────┐
│                 fluxcore                     │
│        LLM API 路由库                        │
├─────────────┬───────────────────────────────┤
│  message/   │          routing/             │
│  数据类型    │     路由选择 + 熔断器         │
│  (IR层)     │          (无锁)               │
├─────────────┴───────────────────────────────┤
│                   call/                      │
│      HTTP传输 + 重试 + 流式                  │
│            + 协议转换                        │
└─────────────────────────────────────────────┘

4个包。17个文件。0个接口。0个依赖。
```

---

### 性能

| 操作 | 时间 | 说明 |
|------|------|------|
| `CurrentEp()` | ~10ns | 无锁原子读取 |
| `MarkFail()` | ~50ns | CAS + O(1) 映射 |
| 并发测试 | 1000 QPS | 无死锁 |

---

### 安全（SSRF 防护）

fluxcore 提供 SSRF 防护工具，策略你来定。

```go
ep := &routing.Endpoint{
    BaseURL:  userProvidedURL,  // 用户输入
    APIKey:   "your-key",
    Protocol: routing.ProtocolOpenAI,
}

// Step 1: 验证格式
if err := ep.Validate(); err != nil {
    return err
}

// Step 2: SSRF 防护（可选，你的策略）
u, _ := url.Parse(ep.BaseURL)
if routing.IsPrivateIP(u.Hostname()) {
    return errors.New("私有 IP 不允许")  // 你的策略
}
```

---

### 谁在使用

| 用户 | 用途 |
|------|------|
| **SaaS 团队** | 多租户 AI 功能，端点隔离 |
| **AI 创业** | 成本控制，自动选最便宜 |
| **平台团队** | 统一 LLM API，运维友好 |
| **独立开发者** | 快速上线，零学习成本 |

---

### 协议支持

| 格式 | 常量 | 端点 |
|------|------|------|
| **OpenAI** | `ProtocolOpenAI` | `/v1/chat/completions` |
| **Anthropic** | `ProtocolAnthropic` | `/v1/messages` |
| **Gemini** | `ProtocolGemini` | `/v1/models/{model}:generateContent` |
| **Cohere** | `ProtocolCohere` | `/v1/chat` |

**OpenAI 兼容提供商：**

| 提供商 | Base URL |
|--------|----------|
| **Azure OpenAI** | 你的 Azure 端点 |
| **Mistral AI** | `https://api.mistral.ai` |
| **Groq** | `https://api.groq.com` |
| **DeepSeek** | `https://api.deepseek.com` |
| **智谱 GLM-4** | `https://open.bigmodel.cn/api/paas/v4/` |

---

### 开始使用

```bash
go get github.com/tokflux/fluxcore
```

**下一步：**
1. 试试上面的快速开始
2. 查看集成示例
3. ⭐ Star 如果有帮助

---

## License

MIT. Free forever.

---

**fluxcore - LLM API Router Library. 30行代码，路由搞定。**