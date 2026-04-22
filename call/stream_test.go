package call

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tokzone/fluxcore/routing"
)

// SSE helper: format OpenAI-style SSE event
func sseOpenAIEvent(content string) string {
	return fmt.Sprintf("data: {\"id\":\"1\",\"choices\":[{\"delta\":{\"content\":[{\"type\":\"text\",\"data\":\"%s\"}]}}]}\n\n", content)
}

// SSE helper: format done event
func sseDone() string {
	return "data: [DONE]\n\n"
}

func TestRequestStreamBasic(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify streaming headers
		if r.Header.Get("Accept") != "text/event-stream" {
			t.Error("expected Accept: text/event-stream")
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)

		// Send SSE events (OpenAI format with content array)
		w.Write([]byte(sseOpenAIEvent("Hello")))
		w.Write([]byte(sseOpenAIEvent(" World")))
		w.Write([]byte(sseDone()))
	}))
	defer server.Close()

	ep := testEndpoint(1, server.URL, "key", routing.ProtocolOpenAI)
	pool := routing.NewEndpointPool([]*routing.Endpoint{ep}, 3)

	ctx := context.Background()
	rawReq := []byte(`{"model":"gpt-4","messages":[{"role":"user","content":[{"type":"text","data":"Hi"}]}]}`)

	result, err := RequestStream(ctx, pool, rawReq, routing.ProtocolOpenAI)
	if err != nil {
		t.Fatalf("RequestStream failed: %v", err)
	}
	defer result.Close()

	var chunks []string
	for chunk := range result.Ch {
		chunks = append(chunks, string(chunk))
	}

	if len(chunks) < 1 {
		t.Errorf("expected at least 1 chunk, got %d", len(chunks))
	}
}

func TestRequestStreamRetrySuccess(t *testing.T) {
	var callCount int32

	// First server fails, second succeeds
	server1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&callCount, 1)
		w.WriteHeader(500)
		w.Write([]byte(`{"error":"internal error"}`))
	}))
	defer server1.Close()

	server2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&callCount, 1)
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)
		w.Write([]byte(sseOpenAIEvent("OK")))
		w.Write([]byte(sseDone()))
	}))
	defer server2.Close()

	ep1 := testEndpointWithPrice(1, server1.URL, "key1", routing.ProtocolOpenAI, 0.01)
	ep2 := testEndpointWithPrice(2, server2.URL, "key2", routing.ProtocolOpenAI, 0.02)

	pool := routing.NewEndpointPool([]*routing.Endpoint{ep1, ep2}, 3)

	ctx := context.Background()
	rawReq := []byte(`{"model":"gpt-4","messages":[{"role":"user","content":[{"type":"text","data":"Hi"}]}]}`)

	result, err := RequestStream(ctx, pool, rawReq, routing.ProtocolOpenAI)
	if err != nil {
		t.Fatalf("RequestStream failed: %v", err)
	}
	defer result.Close()

	// Drain channel
	for range result.Ch {
	}

	if callCount != 2 {
		t.Errorf("expected 2 calls (fail + success), got %d", callCount)
	}
}

func TestRequestStreamAllFail(t *testing.T) {
	var callCount int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&callCount, 1)
		w.WriteHeader(500)
		w.Write([]byte(`{"error":"internal error"}`))
	}))
	defer server.Close()

	ep1 := testEndpointWithPrice(1, server.URL, "key1", routing.ProtocolOpenAI, 0.01)
	ep2 := testEndpointWithPrice(2, server.URL, "key2", routing.ProtocolOpenAI, 0.02)

	pool := routing.NewEndpointPool([]*routing.Endpoint{ep1, ep2}, 3)

	ctx := context.Background()
	rawReq := []byte(`{"model":"gpt-4","messages":[{"role":"user","content":[{"type":"text","data":"Hi"}]}]}`)

	_, err := RequestStream(ctx, pool, rawReq, routing.ProtocolOpenAI)
	if err == nil {
		t.Fatal("expected error when all endpoints fail")
	}

	// Should retry up to retryMax times
	if callCount != 4 {
		t.Errorf("expected 4 calls (1 + 3 retries), got %d", callCount)
	}
}

func TestRequestStreamNoRetryOnNonRetryable(t *testing.T) {
	var callCount int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&callCount, 1)
		w.WriteHeader(401) // Non-retryable auth error
		w.Write([]byte(`{"error":"unauthorized"}`))
	}))
	defer server.Close()

	ep := testEndpointWithPrice(1, server.URL, "key", routing.ProtocolOpenAI, 0.01)
	pool := routing.NewEndpointPool([]*routing.Endpoint{ep}, 3)

	ctx := context.Background()
	rawReq := []byte(`{"model":"gpt-4","messages":[{"role":"user","content":[{"type":"text","data":"Hi"}]}]}`)

	_, err := RequestStream(ctx, pool, rawReq, routing.ProtocolOpenAI)
	if err == nil {
		t.Fatal("expected error for 401")
	}

	// Should NOT retry on non-retryable error
	if callCount != 1 {
		t.Errorf("expected 1 call (no retry), got %d", callCount)
	}
}

func TestRequestStreamContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Slow response
		time.Sleep(2 * time.Second)
		w.WriteHeader(200)
	}))
	defer server.Close()

	ep := testEndpointWithPrice(1, server.URL, "key", routing.ProtocolOpenAI, 0.01)
	pool := routing.NewEndpointPool([]*routing.Endpoint{ep}, 3)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	rawReq := []byte(`{"model":"gpt-4","messages":[{"role":"user","content":[{"type":"text","data":"Hi"}]}]}`)

	_, err := RequestStream(ctx, pool, rawReq, routing.ProtocolOpenAI)
	if err == nil {
		t.Fatal("expected context cancellation error")
	}
}

func TestRequestStreamContextCancellationMidStream(t *testing.T) {
	blockCh := make(chan struct{})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)

		// Send one chunk, then block
		w.Write([]byte(sseOpenAIEvent("Hello")))
		w.(http.Flusher).Flush()

		<-blockCh // Block until test signals
	}))
	defer server.Close()
	defer close(blockCh) // Ensure cleanup

	ep := testEndpointWithPrice(1, server.URL, "key", routing.ProtocolOpenAI, 0.01)
	pool := routing.NewEndpointPool([]*routing.Endpoint{ep}, 3)

	ctx, cancel := context.WithCancel(context.Background())

	rawReq := []byte(`{"model":"gpt-4","messages":[{"role":"user","content":[{"type":"text","data":"Hi"}]}]}`)

	result, err := RequestStream(ctx, pool, rawReq, routing.ProtocolOpenAI)
	if err != nil {
		t.Fatalf("RequestStream failed: %v", err)
	}
	defer result.Close()

	// Cancel context after a short delay
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()

	// Drain channel (should close due to cancel)
	for range result.Ch {
	}
}

func TestStreamResultClose(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)
		w.Write([]byte(sseOpenAIEvent("Hello")))
		w.Write([]byte(sseDone()))
	}))
	defer server.Close()

	ep := testEndpointWithPrice(1, server.URL, "key", routing.ProtocolOpenAI, 0.01)
	pool := routing.NewEndpointPool([]*routing.Endpoint{ep}, 3)

	ctx := context.Background()
	rawReq := []byte(`{"model":"gpt-4","messages":[{"role":"user","content":[{"type":"text","data":"Hi"}]}]}`)

	result, err := RequestStream(ctx, pool, rawReq, routing.ProtocolOpenAI)
	if err != nil {
		t.Fatalf("RequestStream failed: %v", err)
	}

	// Close should not panic
	result.Close()

	// Double close should be safe
	result.Close()
}

func TestRequestStreamUsage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)
		// OpenAI-style SSE with usage
		w.Write([]byte(sseOpenAIEvent("Hello")))
		w.Write([]byte(sseDone()))
	}))
	defer server.Close()

	ep := testEndpointWithPrice(1, server.URL, "key", routing.ProtocolOpenAI, 0.01)
	pool := routing.NewEndpointPool([]*routing.Endpoint{ep}, 3)

	ctx := context.Background()
	rawReq := []byte(`{"model":"gpt-4","messages":[{"role":"user","content":[{"type":"text","data":"Hi"}]}]}`)

	result, err := RequestStream(ctx, pool, rawReq, routing.ProtocolOpenAI)
	if err != nil {
		t.Fatalf("RequestStream failed: %v", err)
	}
	defer result.Close()

	// Drain channel
	for range result.Ch {
	}

	usage := result.Usage()
	if usage == nil {
		t.Fatal("expected usage info")
	}
	// Note: actual token values depend on SSE parsing
}

func TestRequestStreamNoEndpoints(t *testing.T) {
	pool := routing.NewEndpointPool([]*routing.Endpoint{}, 3)

	ctx := context.Background()
	rawReq := []byte(`{"model":"gpt-4","messages":[{"role":"user","content":[{"type":"text","data":"Hi"}]}]}`)

	_, err := RequestStream(ctx, pool, rawReq, routing.ProtocolOpenAI)
	if err == nil {
		t.Fatal("expected error for no endpoints")
	}
}

func TestRequestStreamConcurrentCalls(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)
		w.Write([]byte(sseOpenAIEvent("OK")))
		w.Write([]byte(sseDone()))
	}))
	defer server.Close()

	ep1 := testEndpointWithPrice(1, server.URL, "key1", routing.ProtocolOpenAI, 0.01)
	ep2 := testEndpointWithPrice(2, server.URL, "key2", routing.ProtocolOpenAI, 0.02)

	pool := routing.NewEndpointPool([]*routing.Endpoint{ep1, ep2}, 3)

	ctx := context.Background()
	rawReq := []byte(`{"model":"gpt-4","messages":[{"role":"user","content":[{"type":"text","data":"Hi"}]}]}`)

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			result, err := RequestStream(ctx, pool, rawReq, routing.ProtocolOpenAI)
			if err != nil {
				t.Errorf("RequestStream failed: %v", err)
				return
			}
			defer result.Close()
			for range result.Ch {
			}
		}()
	}
	wg.Wait()
}

func TestRequestStreamConcurrentUsageAccess(t *testing.T) {
	// Test that Usage() can be called concurrently during streaming
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)
		// Send multiple chunks with usage
		for i := 0; i < 5; i++ {
			w.Write([]byte(sseOpenAIEvent("chunk")))
			time.Sleep(10 * time.Millisecond)
		}
		w.Write([]byte(sseDone()))
	}))
	defer server.Close()

	ep := testEndpointWithPrice(1, server.URL, "key", routing.ProtocolOpenAI, 0.01)
	pool := routing.NewEndpointPool([]*routing.Endpoint{ep}, 3)

	ctx := context.Background()
	rawReq := []byte(`{"model":"gpt-4","messages":[{"role":"user","content":[{"type":"text","data":"Hi"}]}]}`)

	result, err := RequestStream(ctx, pool, rawReq, routing.ProtocolOpenAI)
	if err != nil {
		t.Fatalf("RequestStream failed: %v", err)
	}
	defer result.Close()

	var wg sync.WaitGroup

	// Reader goroutine
	wg.Add(1)
	go func() {
		defer wg.Done()
		for range result.Ch {
		}
	}()

	// Concurrent Usage() callers (this tests thread-safety)
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				usage := result.Usage()
				if usage == nil {
					t.Error("Usage() returned nil")
				}
				// Access fields to ensure no race
				_ = usage.InputTokens
				_ = usage.OutputTokens
				_ = usage.LatencyMs
				_ = usage.IsAccurate
				time.Sleep(5 * time.Millisecond)
			}
		}()
	}

	wg.Wait()
}

func TestStreamTransportSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)
		w.Write([]byte("data: test\n\n"))
	}))
	defer server.Close()

	ep := testEndpoint(1, server.URL, "key", routing.ProtocolOpenAI)

	ctx := context.Background()
	body, cancel, err := streamTransport(ctx, ep, []byte(`{}`))
	if err != nil {
		t.Fatalf("streamTransport failed: %v", err)
	}
	defer cancel()
	defer body.Close()

	data, err := io.ReadAll(body)
	if err != nil {
		t.Fatalf("ReadAll failed: %v", err)
	}
	if !strings.Contains(string(data), "data: test") {
		t.Errorf("unexpected response: %s", string(data))
	}
}

func TestStreamTransportError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		w.Write([]byte(`{"error":"internal error"}`))
	}))
	defer server.Close()

	ep := testEndpoint(1, server.URL, "key", routing.ProtocolOpenAI)

	ctx := context.Background()
	_, _, err := streamTransport(ctx, ep, []byte(`{}`))
	if err == nil {
		t.Fatal("expected error for 500")
	}
}