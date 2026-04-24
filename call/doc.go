// Package call provides HTTP request execution with retry, failover, and streaming support.
//
// The call package is the orchestration layer that coordinates:
//   - Endpoint selection via routing.EndpointPool
//   - HTTP transport with automatic retry and exponential backoff
//   - Protocol translation between different LLM providers
//   - Streaming response handling with SSE parsing
//   - Circuit breaker integration for health management
//
// Main entry points:
//   - Request: Non-streaming chat completion request
//   - RequestStream: Streaming chat completion request
//
// Example usage:
//
//	pool := routing.NewEndpointPool(endpoints, 3)
//	resp, usage, err := call.Request(ctx, pool, rawReq, routing.ProtocolOpenAI)
//
// Streaming:
//
//	result, err := call.RequestStream(ctx, pool, rawReq, routing.ProtocolOpenAI)
//	if err != nil { /* handle error */ }
//	defer result.Close()
//	for chunk := range result.Ch {
//	    // process chunk
//	}
//	usage := result.Usage()
package call