package call

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/tokzone/fluxcore/message"
	"github.com/tokzone/fluxcore/routing"
)

func BenchmarkParseRequest(b *testing.B) {
	rawReq := []byte(`{"model":"gpt-4","messages":[{"role":"user","content":[{"type":"text","data":"Hello"}]}]}`)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = parseRequest(rawReq, routing.ProtocolOpenAI)
	}
}

func BenchmarkParseRequestAnthropic(b *testing.B) {
	rawReq := []byte(`{"model":"claude-3","max_tokens":100,"messages":[{"role":"user","content":"Hello"}]}`)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = parseRequest(rawReq, routing.ProtocolAnthropic)
	}
}

func BenchmarkBuildURL(b *testing.B) {
	ep := &routing.Endpoint{
		Key:   &routing.Key{BaseURL: "https://api.openai.com", Protocol: routing.ProtocolOpenAI},
		Model: "gpt-4",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = buildURL(ep, false)
	}
}

func BenchmarkBuildURLGemini(b *testing.B) {
	ep := &routing.Endpoint{
		Key:   &routing.Key{BaseURL: "https://generativelanguage.googleapis.com", Protocol: routing.ProtocolGemini},
		Model: "gemini-pro",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = buildURL(ep, true)
	}
}

func BenchmarkSetHeaders(b *testing.B) {
	ep := &routing.Endpoint{
		Key: &routing.Key{APIKey: "test-key", Protocol: routing.ProtocolOpenAI},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest("POST", "http://example.com", nil)
		setHeaders(req, ep, false)
	}
}

func BenchmarkSetHeadersAnthropic(b *testing.B) {
	ep := &routing.Endpoint{
		Key: &routing.Key{APIKey: "test-key", Protocol: routing.ProtocolAnthropic},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest("POST", "http://example.com", nil)
		setHeaders(req, ep, true)
	}
}

func BenchmarkParseMessageRequest(b *testing.B) {
	rawReq := []byte(`{"model":"gpt-4","messages":[{"role":"user","content":[{"type":"text","data":"Hello"}]}],"max_tokens":100}`)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = message.ParseRequest(rawReq)
	}
}

func BenchmarkParseMessageResponse(b *testing.B) {
	rawResp := []byte(`{"id":"test","model":"gpt-4","choices":[{"index":0,"message":{"role":"assistant","content":[{"type":"text","data":"Hi"}]},"finish_reason":"stop"}],"usage":{"input_tokens":10,"output_tokens":5}}`)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = message.ParseResponse(rawResp)
	}
}

func BenchmarkMarshalMessageRequest(b *testing.B) {
	req := &message.MessageRequest{
		Model:    "gpt-4",
		MaxTokens: 100,
		Messages: []message.Message{
			{Role: "system", Content: []message.Content{message.TextContent("You are helpful")}},
			{Role: "user", Content: []message.Content{message.TextContent("Hello")}},
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = json.Marshal(req)
	}
}

func BenchmarkMarshalMessageResponse(b *testing.B) {
	resp := &message.MessageResponse{
		ID:    "test",
		Model: "gpt-4",
		Choices: []message.Choice{
			{
				Index: 0,
				Message: message.Message{
					Role:    "assistant",
					Content: []message.Content{message.TextContent("Hello there")},
				},
				FinishReason: "stop",
			},
		},
		Usage: &message.Usage{
			InputTokens:  10,
			OutputTokens: 5,
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = json.Marshal(resp)
	}
}

func BenchmarkTransport(b *testing.B) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte(`{"id":"test","choices":[],"usage":{"input_tokens":0,"output_tokens":0}}`))
	}))
	defer server.Close()

	ep, _ := routing.NewEndpoint(1, &routing.Key{BaseURL: server.URL, APIKey: "key", Protocol: routing.ProtocolOpenAI}, "", 0)

	body := []byte(`{"model":"gpt-4","messages":[]}`)
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = transport(ctx, ep, body)
	}
}

func BenchmarkCallWithParsedRequest(b *testing.B) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte(`{"id":"test","choices":[{"message":{"role":"assistant","content":[{"type":"text","data":"Hi"}]}}],"usage":{"input_tokens":5,"output_tokens":2}}`))
	}))
	defer server.Close()

	ep, _ := routing.NewEndpoint(1, &routing.Key{BaseURL: server.URL, APIKey: "key", Protocol: routing.ProtocolOpenAI}, "", 0)

	req := &message.MessageRequest{
		Model:    "gpt-4",
		Messages: []message.Message{{Role: "user", Content: []message.Content{message.TextContent("Hello")}}},
	}
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, _ = callWithParsedRequest(ctx, ep, req, routing.ProtocolOpenAI)
	}
}

func BenchmarkRequestNoRetry(b *testing.B) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte(`{"id":"test","choices":[{"message":{"role":"assistant","content":[{"type":"text","data":"Hi"}]}}],"usage":{"input_tokens":5,"output_tokens":2}}`))
	}))
	defer server.Close()

	ep, _ := routing.NewEndpoint(1, &routing.Key{BaseURL: server.URL, APIKey: "key", Protocol: routing.ProtocolOpenAI}, "", 0)

	pool := routing.NewEndpointPool([]*routing.Endpoint{ep}, 0)

	rawReq := []byte(`{"model":"gpt-4","messages":[{"role":"user","content":[{"type":"text","data":"Hello"}]}]}`)
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, _ = Request(ctx, pool, rawReq, routing.ProtocolOpenAI)
	}
}

// Benchmark HTTP request building
func BenchmarkBuildHTTPRequest(b *testing.B) {
	ep := &routing.Endpoint{
		Key: &routing.Key{BaseURL: "https://api.openai.com", APIKey: "test-key", Protocol: routing.ProtocolOpenAI},
	}
	body := []byte(`{"model":"gpt-4","messages":[]}`)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req, _ := http.NewRequest("POST", buildURL(ep, false), bytes.NewReader(body))
		setHeaders(req, ep, false)
	}
}