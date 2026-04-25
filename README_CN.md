# fluxcore ⚡

**LLM API 客户端库**

[![Go Version](https://img.shields.io/badge/Go-1.21+-00ADD8?style=flat)](https://golang.org)
[![License](https://img.shields.io/badge/License-MIT-green?style=flat)](LICENSE)
[![Version](https://img.shields.io/badge/Version-v0.8.0-blue?style=flat)]()
[![English](https://img.shields.io/badge/README-English-blue?style=flat)](README.md)

简洁的 LLM API 客户端，带路由和健康管理。

---

## 快速开始

```go
import (
    "github.com/tokzone/fluxcore/provider"
    "github.com/tokzone/fluxcore/endpoint"
    "github.com/tokzone/fluxcore/flux"
)

// 1. 定义全局 Provider
openai := provider.NewProvider(1, "https://api.openai.com", provider.ProtocolOpenAI)
anthropic := provider.NewProvider(2, "https://api.anthropic.com", provider.ProtocolAnthropic)

// 2. 注册 Endpoint 到全局 Registry
endpoint.RegisterEndpoint(1, openai, "")
endpoint.RegisterEndpoint(2, anthropic, "")

// 3. 创建 APIKey（Provider + Secret）
key1, _ := flux.NewAPIKey(openai, "sk-xxx")
key2, _ := flux.NewAPIKey(anthropic, "sk-ant-xxx")

// 4. 创建 UserEndpoint（Endpoint + APIKey + Priority）
ue1, _ := flux.NewUserEndpoint("", key1, 1000)
ue2, _ := flux.NewUserEndpoint("", key2, 800)

// 5. 创建 Client
client := flux.NewClient([]*flux.UserEndpoint{ue1, ue2}, flux.WithRetryMax(3))

// 6. 发送请求
resp, usage, err := client.Do(ctx, rawReq, provider.ProtocolOpenAI)

// 7. 流式请求
result, err := client.DoStream(ctx, rawReq, provider.ProtocolAnthropic)
defer result.Close()
for chunk := range result.Ch {
    // 处理 chunk
}
```

---

## 特性

- **简洁 API** — Provider、Endpoint、APIKey、UserEndpoint、Client。五个概念。
- **多租户** — 共享健康状态（Provider/Endpoint），私有密钥（APIKey）和优先级（UserEndpoint）。
- **双层健康** — Provider（网络）+ Endpoint（模型）熔断器。
- **协议转换** — Anthropic 输入，Gemini 输出，透明转换。

---

## 模块架构

```
flux（用户入口）
  │
  └── Client.Do() / DoStream()
  │
flux（用户数据）
  │
  ├── APIKey: Provider + Secret（用户私有）
  └── UserEndpoint: Endpoint + APIKey + Priority（用户私有）
  │
endpoint（全局状态）
  │
  └── Endpoint: Provider + Model + Health（全局单例）
  │
provider（全局状态）
  │
  └── Provider: URL + Protocol + Health（全局单例）
```

---

## 双层熔断器

```
Provider 层（网络）：
  连接拒绝 → 立即熔断（阈值=1）
  恢复：120s

Endpoint 层（模型）：
  429 Rate Limit → 熔断（阈值=1）
  500 Server Error → 熔断（阈值=3）
  恢复：60s
```

---

## 许可证

MIT