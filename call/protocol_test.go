package call

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/tokzone/fluxcore/routing"
)

func TestRequestWithAnthropicEndpoint(t *testing.T) {
	// Anthropic-style response
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte(`{
			"id": "msg_123",
			"type": "message",
			"role": "assistant",
			"content": [{"type": "text", "text": "Hello"}],
			"usage": {"input_tokens": 10, "output_tokens": 5}
		}`))
	}))
	defer server.Close()

	ep := testEndpointWithPrice(1, server.URL, "key", routing.ProtocolAnthropic, 0.008)
	pool := routing.NewEndpointPool([]*routing.Endpoint{ep}, 2)

	ctx := context.Background()
	rawReq := []byte(`{"model":"claude-3","max_tokens":100,"messages":[{"role":"user","content":"Hi"}]}`)

	resp, usage, err := Request(ctx, pool, rawReq, routing.ProtocolAnthropic)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	if len(resp) == 0 {
		t.Error("expected non-empty response")
	}
	if usage == nil {
		t.Error("expected usage info")
	}
}

func TestRequestWithGeminiEndpoint(t *testing.T) {
	// Gemini-style response
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte(`{
			"candidates": [{
				"content": {
					"parts": [{"text": "Hello"}],
					"role": "model"
				},
				"finishReason": "STOP"
			}],
			"usageMetadata": {"promptTokenCount": 10, "candidatesTokenCount": 5}
		}`))
	}))
	defer server.Close()

	key := &routing.Key{BaseURL: server.URL, APIKey: "key", Protocol: routing.ProtocolGemini}
	ep := routing.NewEndpoint(1, key, "gemini-pro", 0.001, 0.002)
	pool := routing.NewEndpointPool([]*routing.Endpoint{ep}, 2)

	ctx := context.Background()
	rawReq := []byte(`{"model":"gemini-pro","messages":[{"role":"user","content":[{"type":"text","data":"Hi"}]}]}`)

	resp, _, err := Request(ctx, pool, rawReq, routing.ProtocolOpenAI)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	if len(resp) == 0 {
		t.Error("expected non-empty response")
	}
}

func TestRequestAnthropicResponseParseError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte(`{invalid json}`))
	}))
	defer server.Close()

	ep := testEndpoint(1, server.URL, "key", routing.ProtocolAnthropic)
	pool := routing.NewEndpointPool([]*routing.Endpoint{ep}, 2)

	ctx := context.Background()
	rawReq := []byte(`{"model":"claude-3","messages":[{"role":"user","content":"Hi"}]}`)

	_, _, err := Request(ctx, pool, rawReq, routing.ProtocolAnthropic)
	if err == nil {
		t.Error("expected error for invalid JSON response")
	}
}

func TestRequestGeminiResponseParseError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte(`{invalid json}`))
	}))
	defer server.Close()

	key := &routing.Key{BaseURL: server.URL, APIKey: "key", Protocol: routing.ProtocolGemini}
	ep := routing.NewEndpoint(1, key, "gemini-pro", 0.001, 0.001)
	pool := routing.NewEndpointPool([]*routing.Endpoint{ep}, 2)

	ctx := context.Background()
	rawReq := []byte(`{"model":"gemini-pro","messages":[{"role":"user","content":[{"type":"text","data":"Hi"}]}]}`)

	_, _, err := Request(ctx, pool, rawReq, routing.ProtocolOpenAI)
	if err == nil {
		t.Error("expected error for invalid JSON response")
	}
}

func TestRequestCohereResponseParseError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte(`{invalid json}`))
	}))
	defer server.Close()

	ep := testEndpoint(1, server.URL, "key", routing.ProtocolCohere)
	pool := routing.NewEndpointPool([]*routing.Endpoint{ep}, 2)

	ctx := context.Background()
	rawReq := []byte(`{"message":"Hi"}`)

	_, _, err := Request(ctx, pool, rawReq, routing.ProtocolCohere)
	if err == nil {
		t.Error("expected error for invalid JSON response")
	}
}

func TestRequestProtocolConversion(t *testing.T) {
	// OpenAI input → Anthropic endpoint → OpenAI output
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte(`{
			"id": "msg_123",
			"type": "message",
			"role": "assistant",
			"content": [{"type": "text", "text": "Hello"}],
			"usage": {"input_tokens": 10, "output_tokens": 5}
		}`))
	}))
	defer server.Close()

	ep := testEndpointWithPrice(1, server.URL, "key", routing.ProtocolAnthropic, 0.008)
	pool := routing.NewEndpointPool([]*routing.Endpoint{ep}, 2)

	ctx := context.Background()
	// OpenAI format input
	rawReq := []byte(`{"model":"gpt-4","messages":[{"role":"user","content":[{"type":"text","data":"Hi"}]}]}`)

	// Request with OpenAI input protocol, Anthropic endpoint
	resp, usage, err := Request(ctx, pool, rawReq, routing.ProtocolOpenAI)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	if len(resp) == 0 {
		t.Error("expected non-empty response")
	}
	if usage == nil {
		t.Error("expected usage info")
	}
}

func TestRequestOpenAIResponseParseError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte(`{invalid json}`))
	}))
	defer server.Close()

	ep := testEndpoint(1, server.URL, "key", routing.ProtocolOpenAI)
	pool := routing.NewEndpointPool([]*routing.Endpoint{ep}, 2)

	ctx := context.Background()
	rawReq := []byte(`{"model":"gpt-4","messages":[{"role":"user","content":[{"type":"text","data":"Hi"}]}]}`)

	_, _, err := Request(ctx, pool, rawReq, routing.ProtocolOpenAI)
	if err == nil {
		t.Error("expected error for invalid JSON response")
	}
}

func TestRequestWithEmptyResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte(`{"id":"test","choices":[],"usage":{"input_tokens":0,"output_tokens":0}}`))
	}))
	defer server.Close()

	ep := testEndpoint(1, server.URL, "key", routing.ProtocolOpenAI)
	pool := routing.NewEndpointPool([]*routing.Endpoint{ep}, 2)

	ctx := context.Background()
	rawReq := []byte(`{"model":"gpt-4","messages":[]}`)

	_, _, err := Request(ctx, pool, rawReq, routing.ProtocolOpenAI)
	// Empty response should be handled gracefully
	_ = err
}

func TestRequestWithMalformedRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte(`{"id":"test"}`))
	}))
	defer server.Close()

	ep := testEndpoint(1, server.URL, "key", routing.ProtocolOpenAI)
	pool := routing.NewEndpointPool([]*routing.Endpoint{ep}, 2)

	ctx := context.Background()

	_, _, err := Request(ctx, pool, []byte(`{invalid}`), routing.ProtocolOpenAI)
	if err == nil {
		t.Error("expected error for malformed request")
	}
}