# fluxcore

**LLM API 路由引擎**

[![Go Version](https://img.shields.io/badge/Go-1.21+-00ADD8?style=flat)](https://golang.org)
[![License](https://img.shields.io/badge/License-MIT-green?style=flat)](LICENSE)
[![Version](https://img.shields.io/badge/Version-v1.0.0-blue?style=flat)]()
[![English](https://img.shields.io/badge/README-English-blue?style=flat)](README.md)

领域驱动的 LLM API 路由引擎，带双层熔断和协议转换。

---

## 快速开始

```go
import "github.com/tokzone/fluxcore"

// 1. 创建 ServiceEndpoint（每个外部 AI 服务一个）
openaiSE := fluxcore.NewServiceEndpoint(fluxcore.Service{
    Name:     "openai",
    BaseURLs: map[fluxcore.Protocol]string{fluxcore.ProtocolOpenAI: "https://api.openai.com"},
})
anthropicSE := fluxcore.NewServiceEndpoint(fluxcore.Service{
    Name:     "anthropic",
    BaseURLs: map[fluxcore.Protocol]string{fluxcore.ProtocolAnthropic: "https://api.anthropic.com"},
})

// 2. 创建 Route（每个 模型+密钥 组合一个）
routes := []*fluxcore.Route{
    fluxcore.NewRoute(fluxcore.RouteDesc{
        SvcEP: openaiSE, Model: "gpt-4", Credential: "sk-xxx", Priority: 0,
    }),
    fluxcore.NewRoute(fluxcore.RouteDesc{
        SvcEP: anthropicSE, Model: "claude-3", Credential: "sk-ant-xxx", Priority: 10,
    }),
}

// 3. 构建 RouteTable（预计算，不可变）
table := fluxcore.NewRouteTable(routes, fluxcore.ProtocolOpenAI)

// 4. 执行请求（含重试和故障转移）
router := fluxcore.NewRouter(fluxcore.ProtocolOpenAI)
route, resp, usage, err := router.Execute(ctx, table, rawReq, 3)

// 5. 流式请求
route, result, err := router.ExecuteStream(ctx, table, rawReq, 3)
defer result.Close()
for chunk := range result.Ch {
    // 处理 chunk
}
```

---

## 核心概念

### ServiceEndpoint（聚合根）

代表一个外部 AI 服务。持有不可变的 `Service` 值对象和网络层熔断器（阈值=1，恢复=120s）。多个 `Route` 实例可共享引用同一 `ServiceEndpoint`。

```go
se := fluxcore.NewServiceEndpoint(fluxcore.Service{
    Name:     "deepseek",
    BaseURLs: map[fluxcore.Protocol]string{
        fluxcore.ProtocolOpenAI:    "https://api.deepseek.com",
        fluxcore.ProtocolAnthropic: "https://api.deepseek.com/anthropic",
    },
})
se.IsAvailable()     // 熔断状态
se.Service().Name    // "deepseek"
```

### Route（聚合根）

代表通过某服务访问特定模型的路由。由 `IdentityKey()` = `hash(ServiceName, Model, Credential)` 标识。持有模型层熔断器（阈值=3，恢复=60s）。

```go
route := fluxcore.NewRoute(fluxcore.RouteDesc{
    SvcEP:      se,
    Model:      "gpt-4",
    Credential: "sk-xxx",
    Priority:   0,  // 越小优先级越高
})
route.IdentityKey()        // "deepseek/gpt-4/sk-xxx"
route.IsAvailable()        // SvcEP.IsAvailable() && route 熔断器关闭
```

### RouteTable（值对象）

不可变的路由预计算快照。构造一次，`Select()` 遍历可用路由 O(n)。按优先级排序，等优先级随机打散。

```go
table := fluxcore.NewRouteTable(routes, fluxcore.ProtocolOpenAI)
route, targetProto := table.Select()  // 第一个可用路由
```

### Router（领域服务）

无状态服务，通过 `RouteTable` 执行请求。处理协议转换、HTTP 传输、退避重试和双层健康反馈。

```go
router := fluxcore.NewRouter(fluxcore.ProtocolOpenAI, fluxcore.WithHTTPClient(customClient))
route, resp, usage, err := router.Execute(ctx, table, body, maxRetry)
```

### RouteRepository

按 identity key 缓存 `Route` 聚合，确保熔断状态在配置重载和请求周期之间保持。

```go
repo := fluxcore.NewRouteRepository()
defer repo.Close()

// 重载时：已有 Route 复用，熔断状态保留
route := repo.FindOrCreate(desc.IdentityKey(), func() *fluxcore.Route {
    return fluxcore.NewRoute(desc)
})
```

---

## 双层熔断器

```
ServiceEndpoint 层（网络）：
  DNS / 连接拒绝 / 超时 → 立即熔断（阈值=1）
  恢复：120s

Route 层（模型）：
  429 限流 → 熔断（累计阈值=3）
  500 服务错误 → 熔断（累计阈值=3）
  恢复：60s
  注意：4xx 非 429 错误不触发任何熔断
```

健康反馈在 `Router.Do()` 中自动处理：

```
成功：route.MarkSuccess() + route.SvcEP().MarkSuccess()
网络错误：route.SvcEP().MarkNetworkFailure()
429/5xx：route.MarkModelFailure()
4xx 非 429：不触发熔断
```

---

## 协议转换

当输入协议与服务支持的协议不匹配时，`RouteTable` 在构造时使用 `ProtocolPriority()`（OpenAI > Anthropic > Gemini > Cohere）预计算目标协议。`Router` 透明处理转换。

```go
// 输入：Anthropic 请求，服务仅支持 OpenAI
// RouteTable.Select() 返回 targetProto = ProtocolOpenAI
// Router.Do() 翻译 Anthropic → OpenAI → 响应转回 Anthropic
```

---

## Options 配置

```go
// 自定义 HTTP Client
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

## 包结构

```
fluxcore/
├── fluxcore.go           # Protocol, Model, Service, ParseProtocol, ProtocolPriority
├── service_endpoint.go   # ServiceEndpoint 聚合（网络熔断）
├── route.go              # RouteDesc, Route 聚合（模型熔断）
├── table.go              # RouteTable 值对象（预计算，不可变）
├── router.go             # Router 领域服务（Do, Stream, Execute, ExecuteStream）
├── route_repo.go         # RouteRepository（FindOrCreate, TTL 300s, 最大 50000）
├── errors/               # 错误分类（IsRetryable, IsNetworkError, IsModelError）
├── message/              # 中间表示类型（MessageRequest, MessageResponse, Usage）
└── internal/
    ├── health/           # CircuitBreaker（三态：Closed → Open → HalfOpen → Closed）
    ├── translate/        # 协议翻译器（OpenAI, Anthropic, Gemini, Cohere）+ SSE 解析
    └── httpclient/       # 共享 HTTP Client
```

---

## 集成模式

### 单租户（tokrouter CLI 代理）

```go
// 启动
svcEPs := map[string]*fluxcore.ServiceEndpoint{...}
repo := fluxcore.NewRouteRepository()
oaRouter := fluxcore.NewRouter(fluxcore.ProtocolOpenAI)

// 从配置构建 RouteTable
routes := configToRoutes(cfg, svcEPs, repo)
tables := make(map[fluxcore.Model]*fluxcore.RouteTable)
for model, routes := range groupByModel(routes) {
    tables[model] = fluxcore.NewRouteTable(routes, fluxcore.ProtocolOpenAI)
}

// 热路径
table := tables[fluxcore.Model(model)]
route, resp, usage, err := oaRouter.Execute(ctx, table, body, maxRetry)

// 重载：svcEPs 和 repo 保持 → 熔断状态保留
```

### 多租户（tokhub SaaS 网关）

```go
// 缓存策略：RouteTable（10s TTL）+ RouteRepository（300s TTL）
table := routeTableCache.Get(cacheKey)
if table == nil {
    records := endpointRepo.GetActiveByUserID(ctx, userID, model)
    routes := builder.BuildRoutes(records, svcEPs, routeRepo)
    // BuildRoutes: 解密密钥 → RouteDesc → repo.FindOrCreate
    table = fluxcore.NewRouteTable(routes, inputProto)
    routeTableCache.Set(cacheKey, table, 10*time.Second)
}
route, resp, usage, err := router.Execute(ctx, table, body, maxRetry)
```

---

## 许可证

MIT
