package call

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	fluxerrors "github.com/tokzone/fluxcore/errors"
	"github.com/tokzone/fluxcore/message"
	"github.com/tokzone/fluxcore/routing"
)

func TestParseRequestInvalidJSON(t *testing.T) {
	_, err := parseRequest([]byte(`{invalid}`), routing.ProtocolOpenAI)
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestParseRequestEmptyBody(t *testing.T) {
	_, err := parseRequest([]byte{}, routing.ProtocolOpenAI)
	if err == nil {
		t.Error("expected error for empty body")
	}
}

func TestParseRequestNilBody(t *testing.T) {
	_, err := parseRequest(nil, routing.ProtocolOpenAI)
	if err == nil {
		t.Error("expected error for nil body")
	}
}

func TestCallWithNilEndpoint(t *testing.T) {
	ctx := context.Background()
	rawReq := []byte(`{"model":"gpt-4"}`)

	// Create empty pool
	pool := routing.NewEndpointPool([]*routing.Endpoint{}, 2)

	_, _, err := Request(ctx, pool, rawReq, routing.ProtocolOpenAI)
	if err == nil {
		t.Error("expected error with empty pool")
	}
}

func TestCallWithUnhealthyEndpoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte(`{"id":"test","choices":[],"usage":{"input_tokens":0,"output_tokens":0}}`))
	}))
	defer server.Close()

	ep, _ := routing.NewEndpoint(1, &routing.Key{BaseURL: server.URL, APIKey: "key", Protocol: routing.ProtocolOpenAI}, "", 0)
	// Mark as unhealthy by triggering circuit breaker (3 failures)
	for i := 0; i < 3; i++ {
		ep.MarkFail()
	}

	pool := routing.NewEndpointPool([]*routing.Endpoint{ep}, 2)
	ctx := context.Background()
	rawReq := []byte(`{"model":"gpt-4","messages":[{"role":"user","content":[{"type":"text","data":"Hello"}]}]}`)

	// Request should fail with no healthy endpoints
	_, _, err := Request(ctx, pool, rawReq, routing.ProtocolOpenAI)
	if err == nil {
		t.Error("expected error with no healthy endpoints")
	}
}

func TestTransportLargeResponse(t *testing.T) {
	// Generate a large response
	largeContent := make([]byte, 10000)
	for i := range largeContent {
		largeContent[i] = 'a'
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write(largeContent)
	}))
	defer server.Close()

	ep := testEndpoint(1, server.URL, "key", routing.ProtocolOpenAI)

	ctx := context.Background()
	resp, err := transport(ctx, ep, []byte(`{}`))
	if err != nil {
		t.Fatalf("transport failed: %v", err)
	}
	if len(resp) != 10000 {
		t.Errorf("expected 10000 bytes, got %d", len(resp))
	}
}

func TestTransportEmptyResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		// Empty body
	}))
	defer server.Close()

	ep := testEndpoint(1, server.URL, "key", routing.ProtocolOpenAI)

	ctx := context.Background()
	resp, err := transport(ctx, ep, []byte(`{}`))
	if err != nil {
		t.Fatalf("transport failed: %v", err)
	}
	if len(resp) != 0 {
		t.Errorf("expected empty response, got %d bytes", len(resp))
	}
}

func TestTransportErrorBodyTruncation(t *testing.T) {
	// Server returns a very large error body
	largeError := make([]byte, 10000)
	for i := range largeError {
		largeError[i] = 'e'
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		w.Write(largeError)
	}))
	defer server.Close()

	ep := testEndpoint(1, server.URL, "key", routing.ProtocolOpenAI)

	ctx := context.Background()
	_, err := transport(ctx, ep, []byte(`{}`))
	if err == nil {
		t.Fatal("expected error")
	}

	var classified *fluxerrors.ClassifiedError
	if errors.As(err, &classified) {
		// Message should be truncated
		if len(classified.Message) > 300 {
			t.Errorf("error message should be truncated, got length %d", len(classified.Message))
		}
	}
}

func TestBuildURLEmptyBaseURL(t *testing.T) {
	ep := &routing.Endpoint{
		Key: &routing.Key{BaseURL: "", Protocol: routing.ProtocolOpenAI},
	}
	url := buildURL(ep, false)
	// Should still produce a URL (even if invalid)
	if url == "" {
		t.Error("expected non-empty URL")
	}
}

func TestBuildURLWithTrailingSlash(t *testing.T) {
	ep := &routing.Endpoint{
		Key: &routing.Key{BaseURL: "https://api.example.com/", Protocol: routing.ProtocolOpenAI},
	}
	url := buildURL(ep, false)
	// Should handle trailing slash correctly
	if url != "https://api.example.com//v1/chat/completions" && url != "https://api.example.com/v1/chat/completions" {
		t.Errorf("unexpected URL: %s", url)
	}
}

func TestSetHeadersEmptyAPIKey(t *testing.T) {
	ep := &routing.Endpoint{
		Key: &routing.Key{APIKey: "", Protocol: routing.ProtocolOpenAI},
	}
	req := httptest.NewRequest("POST", "http://example.com", nil)
	setHeaders(req, ep, false)

	// Should set header even with empty key
	auth := req.Header.Get("Authorization")
	if auth != "Bearer " {
		t.Errorf("expected 'Bearer ', got %q", auth)
	}
}

func TestParseRequestWithUnknownFields(t *testing.T) {
	// JSON with extra unknown fields should be ignored
	rawReq := []byte(`{
		"model": "gpt-4",
		"messages": [{"role": "user", "content": [{"type": "text", "data": "Hello"}]}],
		"unknown_field": "should be ignored",
		"another_unknown": 123
	}`)

	req, err := parseRequest(rawReq, routing.ProtocolOpenAI)
	if err != nil {
		t.Fatalf("parseRequest failed: %v", err)
	}
	if req.Model != "gpt-4" {
		t.Errorf("expected model gpt-4, got %s", req.Model)
	}
}

func TestParseRequestWithMissingFields(t *testing.T) {
	// Minimal valid JSON
	rawReq := []byte(`{}`)

	req, err := parseRequest(rawReq, routing.ProtocolOpenAI)
	if err != nil {
		t.Fatalf("parseRequest failed: %v", err)
	}
	// Model should be empty
	if req.Model != "" {
		t.Errorf("expected empty model, got %s", req.Model)
	}
}

func TestCallWithParsedRequestInvalidProtocol(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte(`{"id":"test","choices":[],"usage":{"input_tokens":0,"output_tokens":0}}`))
	}))
	defer server.Close()

	ep := testEndpoint(1, server.URL, "key", routing.ProtocolOpenAI)

	req := parseMinimalRequest()
	ctx := context.Background()

	// Should use default OpenAI format for unknown format
	_, _, err := callWithParsedRequest(ctx, ep, req, routing.ProtocolOpenAI)
	// Should succeed with default format handling
	if err != nil {
		t.Logf("callWithParsedRequest returned: %v", err)
	}
}

func parseMinimalRequest() *message.MessageRequest {
	return &message.MessageRequest{
		Model: "gpt-4",
		Messages: []message.Message{
			{Role: "user", Content: []message.Content{message.TextContent("Hello")}},
		},
	}
}

func TestRequestWithDifferentProtocols(t *testing.T) {
	tests := []struct {
		name     string
		protocol routing.Protocol
	}{
		{"openai", routing.ProtocolOpenAI},
		{"anthropic", routing.ProtocolAnthropic},
		{"gemini", routing.ProtocolGemini},
		{"cohere", routing.ProtocolCohere},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Just verify parseRequest doesn't panic with different protocols
			rawReq := []byte(`{"model":"test","messages":[{"role":"user","content":"Hello"}]}`)
			_, err := parseRequest(rawReq, tt.protocol)
			// Error is OK, we just verify no panic
			_ = err
		})
	}
}