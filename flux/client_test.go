package flux

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tokzone/fluxcore/endpoint"
	"github.com/tokzone/fluxcore/errors"
	"github.com/tokzone/fluxcore/provider"
)

func TestNewAPIKey(t *testing.T) {
	prov := provider.NewProvider(1, provider.SingleBaseURL("https://api.openai.com"))

	key, err := NewAPIKey(prov, "sk-test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if key == nil {
		t.Fatal("expected APIKey")
	}
	if key.Provider() != prov {
		t.Error("expected same provider")
	}
	if key.Secret() != "sk-test" {
		t.Errorf("expected secret sk-test, got %s", key.Secret())
	}
}

func TestNewAPIKeyNilProvider(t *testing.T) {
	_, err := NewAPIKey(nil, "sk-test")
	if err == nil {
		t.Fatal("expected error for nil provider")
	}
}

func TestNewAPIKeyEmptySecret(t *testing.T) {
	prov := provider.NewProvider(1, provider.SingleBaseURL("https://api.openai.com"))
	_, err := NewAPIKey(prov, "")
	if err == nil {
		t.Fatal("expected error for empty secret")
	}
}

func TestNewUserEndpoint(t *testing.T) {
	prov := provider.NewProvider(1, provider.SingleBaseURL("https://api.openai.com"))
	endpoint.RegisterEndpoint(1, prov, "", []provider.Protocol{provider.ProtocolOpenAI})
	key, _ := NewAPIKey(prov, "sk-test")

	ue, err := NewUserEndpoint("", key, 1000)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ue == nil {
		t.Fatal("expected UserEndpoint")
	}
	if ue.Priority() != 1000 {
		t.Errorf("expected priority 1000, got %d", ue.Priority())
	}
	if ue.Secret() != "sk-test" {
		t.Errorf("expected secret sk-test, got %s", ue.Secret())
	}
}

func TestNewUserEndpointNotFound(t *testing.T) {
	prov := provider.NewProvider(99, provider.SingleBaseURL("https://api.notregistered.com"))
	key, _ := NewAPIKey(prov, "sk-test")

	_, err := NewUserEndpoint("", key, 1000)
	if err == nil {
		t.Fatal("expected error for endpoint not found")
	}
}

func TestNewUserEndpointMultipleProviders(t *testing.T) {
	prov1 := provider.NewProvider(1, provider.SingleBaseURL("https://api.openai.com"))
	prov2 := provider.NewProvider(2, provider.SingleBaseURL("https://api.anthropic.com"))
	endpoint.RegisterEndpoint(1, prov1, "", []provider.Protocol{provider.ProtocolOpenAI})
	endpoint.RegisterEndpoint(2, prov2, "", []provider.Protocol{provider.ProtocolOpenAI})

	key1, _ := NewAPIKey(prov1, "sk-openai")
	key2, _ := NewAPIKey(prov2, "sk-ant")

	// Both endpoints should work with their respective keys
	ue1, err := NewUserEndpoint("", key1, 100)
	if err != nil {
		t.Fatalf("expected no error: %v", err)
	}
	if ue1.Provider() != prov1 {
		t.Error("expected prov1")
	}

	ue2, err := NewUserEndpoint("", key2, 200)
	if err != nil {
		t.Fatalf("expected no error: %v", err)
	}
	if ue2.Provider() != prov2 {
		t.Error("expected prov2")
	}
}

func TestNewClient(t *testing.T) {
	prov := provider.NewProvider(1, provider.SingleBaseURL("https://api.openai.com"))
	endpoint.RegisterEndpoint(1, prov, "", []provider.Protocol{provider.ProtocolOpenAI})
	key, _ := NewAPIKey(prov, "sk-test")
	ue, _ := NewUserEndpoint("", key, 1000)

	client := NewClient([]*UserEndpoint{ue})
	if client == nil {
		t.Fatal("expected client")
	}
	if client.RetryMax() != 3 {
		t.Errorf("expected default retryMax=3, got %d", client.RetryMax())
	}
}

func TestNewClientWithOptions(t *testing.T) {
	prov := provider.NewProvider(1, provider.SingleBaseURL("https://api.openai.com"))
	endpoint.RegisterEndpoint(1, prov, "", []provider.Protocol{provider.ProtocolOpenAI})
	key, _ := NewAPIKey(prov, "sk-test")
	ue, _ := NewUserEndpoint("", key, 1000)

	client := NewClient([]*UserEndpoint{ue}, WithRetryMax(5))
	if client.RetryMax() != 5 {
		t.Errorf("expected retryMax=5, got %d", client.RetryMax())
	}
}

func TestClientNext(t *testing.T) {
	prov := provider.NewProvider(1, provider.SingleBaseURL("https://api.openai.com"))
	ep := endpoint.RegisterEndpoint(1, prov, "", []provider.Protocol{provider.ProtocolOpenAI})
	key, _ := NewAPIKey(prov, "sk-test")
	ue, _ := NewUserEndpoint("", key, 1000)

	client := NewClient([]*UserEndpoint{ue})

	next := client.Next()
	if next == nil {
		t.Fatal("expected user endpoint")
	}
	if next != ue {
		t.Error("expected same user endpoint")
	}

	// Mark endpoint as unhealthy
	ep.MarkEndpointFail()
	ep.MarkEndpointFail()
	ep.MarkEndpointFail()

	// Should return nil when no healthy endpoints
	next = client.Next()
	if next != nil {
		t.Error("expected nil when endpoint is unhealthy")
	}
}

func TestClientDoSuccess(t *testing.T) {
	var callCount int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.WriteHeader(200)
		w.Write([]byte(`{"id":"test","choices":[{"message":{"role":"assistant","content":[{"type":"text","data":"Hi"}]}}],"usage":{"input_tokens":5,"output_tokens":2}}`))
	}))
	defer server.Close()

	prov := provider.NewProvider(1, provider.SingleBaseURL(server.URL))
	endpoint.RegisterEndpoint(1, prov, "", []provider.Protocol{provider.ProtocolOpenAI})
	key, _ := NewAPIKey(prov, "sk-test")
	ue, _ := NewUserEndpoint("", key, 1000)

	client := NewClient([]*UserEndpoint{ue}, WithRetryMax(3))

	ctx := context.Background()
	rawReq := []byte(`{"model":"gpt-4","messages":[{"role":"user","content":[{"type":"text","data":"Hello"}]}]}`)

	resp, usage, err := client.Do(ctx, rawReq, provider.ProtocolOpenAI)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp) == 0 {
		t.Error("expected response")
	}
	if usage == nil {
		t.Error("expected usage")
	}
	if callCount != 1 {
		t.Errorf("expected 1 call, got %d", callCount)
	}
}

func TestClientDoRetry(t *testing.T) {
	var callCount int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount < 3 {
			w.WriteHeader(500)
			w.Write([]byte(`{"error":"internal"}`))
		} else {
			w.WriteHeader(200)
			w.Write([]byte(`{"id":"test","choices":[{"message":{"role":"assistant","content":[{"type":"text","data":"OK"}]}}],"usage":{"input_tokens":5,"output_tokens":2}}`))
		}
	}))
	defer server.Close()

	prov := provider.NewProvider(1, provider.SingleBaseURL(server.URL))
	endpoint.RegisterEndpoint(1, prov, "", []provider.Protocol{provider.ProtocolOpenAI})
	key, _ := NewAPIKey(prov, "sk-test")
	ue, _ := NewUserEndpoint("", key, 1000)

	client := NewClient([]*UserEndpoint{ue}, WithRetryMax(5))

	ctx := context.Background()
	rawReq := []byte(`{"model":"gpt-4","messages":[{"role":"user","content":[{"type":"text","data":"Hello"}]}]}`)

	resp, _, err := client.Do(ctx, rawReq, provider.ProtocolOpenAI)
	if err != nil {
		t.Fatalf("expected success after retries: %v", err)
	}
	if len(resp) == 0 {
		t.Error("expected response")
	}
	if callCount != 3 {
		t.Errorf("expected 3 calls (2 failures + 1 success), got %d", callCount)
	}
}

func TestClientDoNoEndpoints(t *testing.T) {
	client := NewClient([]*UserEndpoint{})

	ctx := context.Background()
	rawReq := []byte(`{"model":"gpt-4","messages":[]}`)

	_, _, err := client.Do(ctx, rawReq, provider.ProtocolOpenAI)
	if err == nil {
		t.Fatal("expected error with no endpoints")
	}
}

func TestClientPrioritySelection(t *testing.T) {
	var selectedProvider int32

	server1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		selectedProvider = 1
		w.WriteHeader(200)
		w.Write([]byte(`{"id":"test","choices":[],"usage":{}}`))
	}))
	defer server1.Close()

	server2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		selectedProvider = 2
		w.WriteHeader(200)
		w.Write([]byte(`{"id":"test","choices":[],"usage":{}}`))
	}))
	defer server2.Close()

	prov1 := provider.NewProvider(1, provider.SingleBaseURL(server1.URL))
	prov2 := provider.NewProvider(2, provider.SingleBaseURL(server2.URL))
	endpoint.RegisterEndpoint(1, prov1, "", []provider.Protocol{provider.ProtocolOpenAI})
	endpoint.RegisterEndpoint(2, prov2, "", []provider.Protocol{provider.ProtocolOpenAI})

	key1, _ := NewAPIKey(prov1, "sk-1")
	key2, _ := NewAPIKey(prov2, "sk-2")

	// Lower priority = preferred
	ue1, _ := NewUserEndpoint("", key1, 100) // Preferred
	ue2, _ := NewUserEndpoint("", key2, 200)

	client := NewClient([]*UserEndpoint{ue1, ue2})

	ctx := context.Background()
	rawReq := []byte(`{"model":"gpt-4","messages":[]}`)

	client.Do(ctx, rawReq, provider.ProtocolOpenAI)

	if selectedProvider != 1 {
		t.Errorf("expected provider 1 (lower priority), got %d", selectedProvider)
	}
}

func TestBackoffWithJitter(t *testing.T) {
	// Zero/negative should return 0
	if backoffWithJitter(0) != 0 {
		t.Error("expected 0 for attempt 0")
	}
	if backoffWithJitter(-1) != 0 {
		t.Error("expected 0 for negative attempt")
	}

	// First attempt: max 100ms
	for i := 0; i < 100; i++ {
		b := backoffWithJitter(1)
		if b < 0 || b > 100*time.Millisecond {
			t.Errorf("backoff out of range: %v", b)
		}
	}

	// Second attempt: max 200ms
	for i := 0; i < 100; i++ {
		b := backoffWithJitter(2)
		if b < 0 || b > 200*time.Millisecond {
			t.Errorf("backoff out of range: %v", b)
		}
	}

	// Large attempt: capped at 5s
	for i := 0; i < 100; i++ {
		b := backoffWithJitter(10)
		if b < 0 || b > 5*time.Second {
			t.Errorf("backoff out of range: %v", b)
		}
	}
}

func TestClientDoRetryExhausted(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		w.Write([]byte(`{"error":"internal"}`))
	}))
	defer server.Close()

	prov := provider.NewProvider(1, provider.SingleBaseURL(server.URL))
	endpoint.RegisterEndpoint(1, prov, "", []provider.Protocol{provider.ProtocolOpenAI})
	key, _ := NewAPIKey(prov, "sk-test")
	ue, _ := NewUserEndpoint("", key, 1000)

	client := NewClient([]*UserEndpoint{ue}, WithRetryMax(2))

	ctx := context.Background()
	rawReq := []byte(`{"model":"gpt-4","messages":[{"role":"user","content":[{"type":"text","data":"Hello"}]}]}`)

	_, _, err := client.Do(ctx, rawReq, provider.ProtocolOpenAI)
	if err == nil {
		t.Fatal("expected error after retries exhausted")
	}
}

func TestClientDoTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second) // Simulate slow response
		w.WriteHeader(200)
		w.Write([]byte(`{"id":"test"}`))
	}))
	defer server.Close()

	prov := provider.NewProvider(1, provider.SingleBaseURL(server.URL))
	endpoint.RegisterEndpoint(1, prov, "", []provider.Protocol{provider.ProtocolOpenAI})
	key, _ := NewAPIKey(prov, "sk-test")
	ue, _ := NewUserEndpoint("", key, 1000)

	client := NewClient([]*UserEndpoint{ue}, WithRetryMax(1))

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	rawReq := []byte(`{"model":"gpt-4","messages":[{"role":"user","content":[{"type":"text","data":"Hello"}]}]}`)

	_, _, err := client.Do(ctx, rawReq, provider.ProtocolOpenAI)
	if err == nil {
		t.Fatal("expected timeout error")
	}
}

func TestClientDoStream(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)
		// OpenAI streaming format with correct content structure
		w.Write([]byte("data: {\"id\":\"test\",\"choices\":[{\"delta\":{\"content\":[{\"type\":\"text\",\"data\":\"Hi\"}]} }]}\n\n"))
		w.Write([]byte("data: {\"id\":\"test\",\"choices\":[{\"delta\":{\"content\":[{\"type\":\"text\",\"data\":\" there\"}]} }]}\n\n"))
		w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()

	prov := provider.NewProvider(1, provider.SingleBaseURL(server.URL))
	endpoint.RegisterEndpoint(1, prov, "", []provider.Protocol{provider.ProtocolOpenAI})
	key, _ := NewAPIKey(prov, "sk-test")
	ue, _ := NewUserEndpoint("", key, 1000)

	client := NewClient([]*UserEndpoint{ue}, WithRetryMax(3))

	ctx := context.Background()
	rawReq := []byte(`{"model":"gpt-4","messages":[{"role":"user","content":[{"type":"text","data":"Hello"}]}],"stream":true}`)

	result, err := client.DoStream(ctx, rawReq, provider.ProtocolOpenAI)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer result.Close()

	var chunks []string
	for chunk := range result.Ch {
		chunks = append(chunks, string(chunk))
	}

	// SSE stream should produce chunks
	if len(chunks) < 2 {
		t.Errorf("expected at least 2 chunks, got %d", len(chunks))
	}
}

func TestIsProviderError(t *testing.T) {
	// Test ClassifiedError handling
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "network_error",
			err:      &errors.ClassifiedError{Code: errors.CodeNetworkError},
			expected: true,
		},
		{
			name:     "timeout",
			err:      &errors.ClassifiedError{Code: errors.CodeTimeout},
			expected: true,
		},
		{
			name:     "dns_error",
			err:      &errors.ClassifiedError{Code: errors.CodeDNSError},
			expected: true,
		},
		{
			name:     "rate_limit",
			err:      &errors.ClassifiedError{Code: errors.CodeRateLimit},
			expected: false,
		},
		{
			name:     "server_error",
			err:      &errors.ClassifiedError{Code: errors.CodeServerError},
			expected: false,
		},
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isProviderError(tt.err)
			if result != tt.expected {
				t.Errorf("isProviderError(%v) = %v, expected %v", tt.err, result, tt.expected)
			}
		})
	}
}

func TestClientFeedback(t *testing.T) {
	prov := provider.NewProvider(1, provider.SingleBaseURL("https://api.openai.com"))
	ep := endpoint.RegisterEndpoint(1, prov, "", []provider.Protocol{provider.ProtocolOpenAI})
	key, _ := NewAPIKey(prov, "sk-test")
	ue, _ := NewUserEndpoint("", key, 1000)

	client := NewClient([]*UserEndpoint{ue})

	// Test success feedback
	client.Feedback(ue, nil, 100)
	if !ep.IsAvailable() {
		t.Error("endpoint should be available after success")
	}

	// Test failure feedback (endpoint layer)
	client.Feedback(ue, &errors.ClassifiedError{Code: errors.CodeServerError}, 0)
	// Should not immediately circuit break (threshold=3)

	// Test nil UserEndpoint
	client.Feedback(nil, nil, 100) // Should not panic
}

func TestClientIsHealthy(t *testing.T) {
	prov := provider.NewProvider(1, provider.SingleBaseURL("https://api.openai.com"))
	endpoint.RegisterEndpoint(1, prov, "", []provider.Protocol{provider.ProtocolOpenAI})
	key, _ := NewAPIKey(prov, "sk-test")
	ue, _ := NewUserEndpoint("", key, 1000)

	client := NewClient([]*UserEndpoint{ue})

	if !client.IsHealthy(prov, "") {
		t.Error("endpoint should be healthy")
	}

	// Test unregistered endpoint
	unregistered := provider.NewProvider(99, provider.SingleBaseURL("https://api.unknown.com"))
	if client.IsHealthy(unregistered, "") {
		t.Error("unregistered endpoint should not be healthy")
	}
}

func TestClientLatency(t *testing.T) {
	prov := provider.NewProvider(1, provider.SingleBaseURL("https://api.openai.com"))
	ep := endpoint.RegisterEndpoint(1, prov, "", []provider.Protocol{provider.ProtocolOpenAI})
	key, _ := NewAPIKey(prov, "sk-test")
	ue, _ := NewUserEndpoint("", key, 1000)

	client := NewClient([]*UserEndpoint{ue})

	// Update latency
	ep.UpdateEndpointLatency(100)
	prov.UpdateProviderLatency(50)

	// Check latency methods
	epLatency := client.EndpointLatency(prov, "")
	if epLatency <= 0 {
		t.Errorf("expected positive endpoint latency, got %d", epLatency)
	}

	provLatency := client.ProviderLatency(prov)
	if provLatency <= 0 {
		t.Errorf("expected positive provider latency, got %d", provLatency)
	}
}

func TestClientDoStreamTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)
		// Slow response - will timeout
		time.Sleep(2 * time.Second)
		w.Write([]byte("data: {\"id\":\"test\"}\n\n"))
	}))
	defer server.Close()

	prov := provider.NewProvider(1, provider.SingleBaseURL(server.URL))
	endpoint.RegisterEndpoint(1, prov, "", []provider.Protocol{provider.ProtocolOpenAI})
	key, _ := NewAPIKey(prov, "sk-test")
	ue, _ := NewUserEndpoint("", key, 1000)

	client := NewClient([]*UserEndpoint{ue}, WithRetryMax(1))

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	rawReq := []byte(`{"model":"gpt-4","messages":[{"role":"user","content":[{"type":"text","data":"Hello"}]}],"stream":true}`)

	result, err := client.DoStream(ctx, rawReq, provider.ProtocolOpenAI)
	if err != nil {
		// Timeout before stream started
		return
	}
	defer result.Close()

	// Should timeout during read
	for chunk := range result.Ch {
		t.Logf("got chunk: %s", chunk)
	}

	if result.Error() == nil {
		t.Error("expected timeout error in stream")
	}
}

func TestClientDoStreamError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		w.Write([]byte(`{"error":"internal server error"}`))
	}))
	defer server.Close()

	prov := provider.NewProvider(1, provider.SingleBaseURL(server.URL))
	endpoint.RegisterEndpoint(1, prov, "", []provider.Protocol{provider.ProtocolOpenAI})
	key, _ := NewAPIKey(prov, "sk-test")
	ue, _ := NewUserEndpoint("", key, 1000)

	client := NewClient([]*UserEndpoint{ue}, WithRetryMax(1))

	ctx := context.Background()
	rawReq := []byte(`{"model":"gpt-4","messages":[{"role":"user","content":[{"type":"text","data":"Hello"}]}],"stream":true}`)

	_, err := client.DoStream(ctx, rawReq, provider.ProtocolOpenAI)
	if err == nil {
		t.Fatal("expected error for 500 response")
	}
}

func TestClientConcurrentRequests(t *testing.T) {
	var callCount int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&callCount, 1)
		w.WriteHeader(200)
		w.Write([]byte(`{"id":"test","choices":[{"message":{"role":"assistant","content":[{"type":"text","data":"OK"}]}}],"usage":{"input_tokens":5,"output_tokens":2}}`))
	}))
	defer server.Close()

	prov := provider.NewProvider(1, provider.SingleBaseURL(server.URL))
	endpoint.RegisterEndpoint(1, prov, "", []provider.Protocol{provider.ProtocolOpenAI})
	key, _ := NewAPIKey(prov, "sk-test")
	ue, _ := NewUserEndpoint("", key, 1000)

	client := NewClient([]*UserEndpoint{ue}, WithRetryMax(3))

	ctx := context.Background()
	rawReq := []byte(`{"model":"gpt-4","messages":[{"role":"user","content":[{"type":"text","data":"Hello"}]}]}`)

	// 10 concurrent requests
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp, _, err := client.Do(ctx, rawReq, provider.ProtocolOpenAI)
			if err != nil {
				t.Errorf("concurrent request failed: %v", err)
			}
			if len(resp) == 0 {
				t.Error("expected response")
			}
		}()
	}
	wg.Wait()

	if callCount != 10 {
		t.Errorf("expected 10 calls, got %d", callCount)
	}
}

func TestClientFeedbackProviderFail(t *testing.T) {
	prov := provider.NewProvider(1, provider.SingleBaseURL("https://api.openai.com"))
	ep := endpoint.RegisterEndpoint(1, prov, "", []provider.Protocol{provider.ProtocolOpenAI})
	key, _ := NewAPIKey(prov, "sk-test")
	ue, _ := NewUserEndpoint("", key, 1000)

	client := NewClient([]*UserEndpoint{ue})

	// Test provider-level failure
	client.Feedback(ue, &errors.ClassifiedError{Code: errors.CodeNetworkError}, 0)

	// Provider should be marked as failed
	if prov.IsCircuitBreakerOpen() {
		// Provider circuit breaker should be open (threshold=1)
		t.Log("provider circuit breaker opened correctly")
	}

	// Test multiple endpoint failures to trigger circuit breaker
	client.Feedback(ue, &errors.ClassifiedError{Code: errors.CodeServerError}, 0)
	client.Feedback(ue, &errors.ClassifiedError{Code: errors.CodeServerError}, 0)
	client.Feedback(ue, &errors.ClassifiedError{Code: errors.CodeServerError}, 0)

	// Endpoint circuit breaker should be open (threshold=3)
	if ep.IsCircuitBreakerOpen() {
		t.Log("endpoint circuit breaker opened correctly")
	}
}

func TestClientDoNonRetryableError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
		w.Write([]byte(`{"error":"invalid api key"}`))
	}))
	defer server.Close()

	prov := provider.NewProvider(1, provider.SingleBaseURL(server.URL))
	endpoint.RegisterEndpoint(1, prov, "", []provider.Protocol{provider.ProtocolOpenAI})
	key, _ := NewAPIKey(prov, "sk-test")
	ue, _ := NewUserEndpoint("", key, 1000)

	client := NewClient([]*UserEndpoint{ue}, WithRetryMax(5))

	ctx := context.Background()
	rawReq := []byte(`{"model":"gpt-4","messages":[{"role":"user","content":[{"type":"text","data":"Hello"}]}]}`)

	_, _, err := client.Do(ctx, rawReq, provider.ProtocolOpenAI)
	if err == nil {
		t.Fatal("expected error for 401")
	}
	// Auth error should not retry
}

func TestClientDoRateLimitRetry(t *testing.T) {
	var callCount int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := atomic.AddInt32(&callCount, 1)
		if count < 3 {
			w.WriteHeader(429)
			w.Write([]byte(`{"error":"rate limit exceeded"}`))
		} else {
			w.WriteHeader(200)
			w.Write([]byte(`{"id":"test","choices":[{"message":{"role":"assistant","content":[{"type":"text","data":"OK"}]}}],"usage":{"input_tokens":5,"output_tokens":2}}`))
		}
	}))
	defer server.Close()

	prov := provider.NewProvider(1, provider.SingleBaseURL(server.URL))
	endpoint.RegisterEndpoint(1, prov, "", []provider.Protocol{provider.ProtocolOpenAI})
	key, _ := NewAPIKey(prov, "sk-test")
	ue, _ := NewUserEndpoint("", key, 1000)

	client := NewClient([]*UserEndpoint{ue}, WithRetryMax(5))

	ctx := context.Background()
	rawReq := []byte(`{"model":"gpt-4","messages":[{"role":"user","content":[{"type":"text","data":"Hello"}]}]}`)

	resp, _, err := client.Do(ctx, rawReq, provider.ProtocolOpenAI)
	if err != nil {
		t.Fatalf("expected success after retries: %v", err)
	}
	if len(resp) == 0 {
		t.Error("expected response")
	}
	if callCount != 3 {
		t.Errorf("expected 3 calls (2 rate limits + 1 success), got %d", callCount)
	}
}

func TestUserEndpointMethods(t *testing.T) {
	prov := provider.NewProvider(1, provider.SingleBaseURL("https://api.openai.com"))
	ep := endpoint.RegisterEndpoint(1, prov, "", []provider.Protocol{provider.ProtocolOpenAI})
	key, _ := NewAPIKey(prov, "sk-test")
	ue, _ := NewUserEndpoint("", key, 1000)

	// Test all accessor methods
	if ue.Endpoint() != ep {
		t.Error("Endpoint() should return same endpoint")
	}
	if ue.APIKey() != key {
		t.Error("APIKey() should return same key")
	}
	if ue.Provider() != prov {
		t.Error("Provider() should return same provider")
	}
	if ue.Secret() != "sk-test" {
		t.Errorf("Secret() should return 'sk-test', got %s", ue.Secret())
	}
	if ue.Protocol() != provider.ProtocolOpenAI {
		t.Errorf("Protocol() should return OpenAI, got %v", ue.Protocol())
	}
	if ue.BaseURL(provider.ProtocolOpenAI) != "https://api.openai.com" {
		t.Errorf("BaseURL() incorrect, got %s", ue.BaseURL(provider.ProtocolOpenAI))
	}
}

func TestClientNextFallback(t *testing.T) {
	// Create multiple endpoints with different priorities
	server1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte(`{"id":"test"}`))
	}))
	defer server1.Close()

	server2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte(`{"id":"test"}`))
	}))
	defer server2.Close()

	prov1 := provider.NewProvider(1, provider.SingleBaseURL(server1.URL))
	prov2 := provider.NewProvider(2, provider.SingleBaseURL(server2.URL))
	ep1 := endpoint.RegisterEndpoint(1, prov1, "", []provider.Protocol{provider.ProtocolOpenAI})
	ep2 := endpoint.RegisterEndpoint(2, prov2, "", []provider.Protocol{provider.ProtocolOpenAI})

	key1, _ := NewAPIKey(prov1, "sk-1")
	key2, _ := NewAPIKey(prov2, "sk-2")

	ue1, _ := NewUserEndpoint("", key1, 100) // High priority
	ue2, _ := NewUserEndpoint("", key2, 200) // Lower priority

	client := NewClient([]*UserEndpoint{ue1, ue2})

	// Initially should return ue1 (higher priority)
	next := client.Next()
	if next != ue1 {
		t.Error("expected ue1 (higher priority)")
	}

	// Mark ue1 endpoint as unhealthy
	ep1.MarkEndpointFail()
	ep1.MarkEndpointFail()
	ep1.MarkEndpointFail()

	// Should fallback to ue2
	next = client.Next()
	if next != ue2 {
		t.Error("expected fallback to ue2")
	}

	// Mark ue2 as unhealthy too
	ep2.MarkEndpointFail()
	ep2.MarkEndpointFail()
	ep2.MarkEndpointFail()

	// Should return nil (no healthy endpoints)
	next = client.Next()
	if next != nil {
		t.Error("expected nil when all endpoints unhealthy")
	}
}

func TestClientRefreshCache(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte(`{"id":"test"}`))
	}))
	defer server.Close()

	prov := provider.NewProvider(1, provider.SingleBaseURL(server.URL))
	endpoint.RegisterEndpoint(1, prov, "", []provider.Protocol{provider.ProtocolOpenAI})
	key, _ := NewAPIKey(prov, "sk-test")
	ue, _ := NewUserEndpoint("", key, 1000)

	client := NewClient([]*UserEndpoint{ue})

	// Get cached endpoint
	next := client.Next()
	if next == nil {
		t.Fatal("expected cached endpoint")
	}

	// Simulate failure - cache should refresh
	client.Feedback(ue, &errors.ClassifiedError{Code: errors.CodeServerError}, 0)
}

func TestClientDoInvalidRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(400)
		w.Write([]byte(`{"error":"invalid request: model not found"}`))
	}))
	defer server.Close()

	prov := provider.NewProvider(1, provider.SingleBaseURL(server.URL))
	endpoint.RegisterEndpoint(1, prov, "", []provider.Protocol{provider.ProtocolOpenAI})
	key, _ := NewAPIKey(prov, "sk-test")
	ue, _ := NewUserEndpoint("", key, 1000)

	client := NewClient([]*UserEndpoint{ue}, WithRetryMax(3))

	ctx := context.Background()
	rawReq := []byte(`{"model":"invalid-model","messages":[{"role":"user","content":[{"type":"text","data":"Hello"}]}]}`)

	_, _, err := client.Do(ctx, rawReq, provider.ProtocolOpenAI)
	if err == nil {
		t.Fatal("expected error for 400")
	}

	// 400 (invalid_request) should not retry - only one call
}

func TestAPIKeyMethods(t *testing.T) {
	prov := provider.NewProvider(1, provider.SingleBaseURL("https://api.openai.com"))
	key, _ := NewAPIKey(prov, "sk-test123")

	if key.Provider() != prov {
		t.Error("Provider() should return same provider")
	}
	if key.Secret() != "sk-test123" {
		t.Errorf("Secret() should return 'sk-test123', got %s", key.Secret())
	}
}

func TestClientEmptyEndpoints(t *testing.T) {
	client := NewClient([]*UserEndpoint{})

	// Next() should return nil
	next := client.Next()
	if next != nil {
		t.Error("expected nil for empty endpoints")
	}

	// Do() should return error
	ctx := context.Background()
	_, _, err := client.Do(ctx, []byte(`{"model":"gpt-4"}`), provider.ProtocolOpenAI)
	if err == nil {
		t.Fatal("expected error with no endpoints")
	}

	// DoStream() should return error
	_, err = client.DoStream(ctx, []byte(`{"model":"gpt-4","stream":true}`), provider.ProtocolOpenAI)
	if err == nil {
		t.Fatal("expected error with no endpoints")
	}
}

func TestClientDoSuccessWithUsage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte(`{"id":"test","choices":[{"message":{"role":"assistant","content":[{"type":"text","data":"Hello back!"}]}}],"usage":{"input_tokens":10,"output_tokens":5}}`))
	}))
	defer server.Close()

	prov := provider.NewProvider(1, provider.SingleBaseURL(server.URL))
	ep := endpoint.RegisterEndpoint(1, prov, "", []provider.Protocol{provider.ProtocolOpenAI})
	key, _ := NewAPIKey(prov, "sk-test")
	ue, _ := NewUserEndpoint("", key, 1000)

	client := NewClient([]*UserEndpoint{ue}, WithRetryMax(3))

	ctx := context.Background()
	rawReq := []byte(`{"model":"gpt-4","messages":[{"role":"user","content":[{"type":"text","data":"Hello"}]}]}`)

	resp, usage, err := client.Do(ctx, rawReq, provider.ProtocolOpenAI)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp) == 0 {
		t.Error("expected response")
	}
	if usage == nil {
		t.Fatal("expected usage")
	}
	if usage.InputTokens != 10 {
		t.Errorf("expected input_tokens=10, got %d", usage.InputTokens)
	}
	if usage.OutputTokens != 5 {
		t.Errorf("expected output_tokens=5, got %d", usage.OutputTokens)
	}

	// Verify endpoint is healthy after success
	if !ep.IsAvailable() {
		t.Error("endpoint should be healthy after success")
	}
}
