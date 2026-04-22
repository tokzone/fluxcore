package call

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tokzone/fluxcore/routing"
)

func TestRequestRetrySuccess(t *testing.T) {
	var callCount int32

	// First server fails, second succeeds
	server1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&callCount, 1)
		w.WriteHeader(500) // Retryable error
		w.Write([]byte(`{"error":"internal error"}`))
	}))
	defer server1.Close()

	server2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&callCount, 1)
		w.WriteHeader(200)
		w.Write([]byte(`{"id":"test","choices":[{"message":{"role":"assistant","content":[{"type":"text","data":"Success"}]}}],"usage":{"input_tokens":5,"output_tokens":2}}`))
	}))
	defer server2.Close()

	// Create endpoints
	ep1 := testEndpointWithPrice(1, server1.URL, "key1", routing.ProtocolOpenAI, 0.01)
	ep2 := testEndpointWithPrice(2, server2.URL, "key2", routing.ProtocolOpenAI, 0.02)

	pool := routing.NewEndpointPool([]*routing.Endpoint{ep1, ep2}, 3)

	ctx := context.Background()
	rawReq := []byte(`{"model":"gpt-4","messages":[{"role":"user","content":[{"type":"text","data":"Hello"}]}]}`)

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

	// Should have called both servers (ep1 failed, ep2 succeeded)
	if callCount != 2 {
		t.Errorf("expected 2 calls, got %d", callCount)
	}
}

func TestRequestRetryAllFail(t *testing.T) {
	var callCount int32

	// All servers fail
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&callCount, 1)
		w.WriteHeader(500)
		w.Write([]byte(`{"error":"internal error"}`))
	}))
	defer server.Close()

	// Create multiple endpoints so we can retry beyond circuit breaker threshold
	ep1 := testEndpointWithPrice(1, server.URL, "key1", routing.ProtocolOpenAI, 0.01)
	ep2 := testEndpointWithPrice(2, server.URL, "key2", routing.ProtocolOpenAI, 0.02)

	pool := routing.NewEndpointPool([]*routing.Endpoint{ep1, ep2}, 3)

	ctx := context.Background()
	rawReq := []byte(`{"model":"gpt-4","messages":[{"role":"user","content":[{"type":"text","data":"Hello"}]}]}`)

	_, _, err := Request(ctx, pool, rawReq, routing.ProtocolOpenAI)
	if err == nil {
		t.Fatal("expected error when all endpoints fail")
	}

	// Should have retried up to retryMax times (4 iterations: 0, 1, 2, 3)
	if callCount != 4 {
		t.Errorf("expected 4 calls (1 + 3 retries), got %d", callCount)
	}
}

func TestRequestNoRetryOnNonRetryable(t *testing.T) {
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
	rawReq := []byte(`{"model":"gpt-4","messages":[{"role":"user","content":[{"type":"text","data":"Hello"}]}]}`)

	_, _, err := Request(ctx, pool, rawReq, routing.ProtocolOpenAI)
	if err == nil {
		t.Fatal("expected error for 401")
	}

	// Should NOT retry on non-retryable error
	if callCount != 1 {
		t.Errorf("expected 1 call (no retry), got %d", callCount)
	}
}

func TestRequestSuccessNoRetry(t *testing.T) {
	var callCount int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&callCount, 1)
		w.WriteHeader(200)
		w.Write([]byte(`{"id":"test","choices":[{"message":{"role":"assistant","content":[{"type":"text","data":"Hello"}]}}],"usage":{"input_tokens":5,"output_tokens":2}}`))
	}))
	defer server.Close()

	ep := testEndpointWithPrice(1, server.URL, "key", routing.ProtocolOpenAI, 0.01)
	pool := routing.NewEndpointPool([]*routing.Endpoint{ep}, 3)

	ctx := context.Background()
	rawReq := []byte(`{"model":"gpt-4","messages":[{"role":"user","content":[{"type":"text","data":"Hello"}]}]}`)

	resp, _, err := Request(ctx, pool, rawReq, routing.ProtocolOpenAI)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp) == 0 {
		t.Error("expected non-empty response")
	}

	// Should only call once on success
	if callCount != 1 {
		t.Errorf("expected 1 call, got %d", callCount)
	}
}

func TestRequestContextCancellationDuringRetry(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
		w.WriteHeader(200)
	}))
	defer server.Close()

	ep := testEndpointWithPrice(1, server.URL, "key", routing.ProtocolOpenAI, 0.01)
	pool := routing.NewEndpointPool([]*routing.Endpoint{ep}, 3)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	rawReq := []byte(`{"model":"gpt-4","messages":[{"role":"user","content":[{"type":"text","data":"Hello"}]}]}`)

	_, _, err := Request(ctx, pool, rawReq, routing.ProtocolOpenAI)
	if err == nil {
		t.Fatal("expected context cancellation error")
	}
}

func TestRequestNoEndpoints(t *testing.T) {
	pool := routing.NewEndpointPool([]*routing.Endpoint{}, 3)

	ctx := context.Background()
	rawReq := []byte(`{"model":"gpt-4","messages":[{"role":"user","content":[{"type":"text","data":"Hello"}]}]}`)

	_, _, err := Request(ctx, pool, rawReq, routing.ProtocolOpenAI)
	if err == nil {
		t.Fatal("expected error for no endpoints")
	}
}

func TestRequestPoolMarkSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte(`{"id":"test","choices":[{"message":{"role":"assistant","content":[{"type":"text","data":"Hello"}]}}],"usage":{"input_tokens":5,"output_tokens":2}}`))
	}))
	defer server.Close()

	ep := testEndpointWithPrice(1, server.URL, "key", routing.ProtocolOpenAI, 0.01)
	pool := routing.NewEndpointPool([]*routing.Endpoint{ep}, 3)

	ctx := context.Background()
	rawReq := []byte(`{"model":"gpt-4","messages":[{"role":"user","content":[{"type":"text","data":"Hello"}]}]}`)

	_, _, err := Request(ctx, pool, rawReq, routing.ProtocolOpenAI)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Endpoint should still be healthy after success
	if !!ep.IsCircuitBreakerOpen() {
		t.Error("endpoint should be healthy after success")
	}
}

func TestRequestPoolMarkFail(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		w.Write([]byte(`{"error":"internal error"}`))
	}))
	defer server.Close()

	ep := testEndpointWithPrice(1, server.URL, "key", routing.ProtocolOpenAI, 0.01)
	pool := routing.NewEndpointPool([]*routing.Endpoint{ep}, 3)

	ctx := context.Background()
	rawReq := []byte(`{"model":"gpt-4","messages":[{"role":"user","content":[{"type":"text","data":"Hello"}]}]}`)

	_, _, err := Request(ctx, pool, rawReq, routing.ProtocolOpenAI)
	if err == nil {
		t.Fatal("expected error")
	}

	// Endpoint should be unhealthy after failures exceed threshold
	if !ep.IsCircuitBreakerOpen() {
		t.Error("endpoint should be unhealthy after failures")
	}
}

func TestRequestRateLimitRetry(t *testing.T) {
	var callCount int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := atomic.AddInt32(&callCount, 1)
		if count < 3 {
			// First two calls: rate limit
			w.WriteHeader(429)
			w.Write([]byte(`{"error":"rate limit"}`))
		} else {
			// Third call: success
			w.WriteHeader(200)
			w.Write([]byte(`{"id":"test","choices":[{"message":{"role":"assistant","content":[{"type":"text","data":"OK"}]}}],"usage":{"input_tokens":5,"output_tokens":2}}`))
		}
	}))
	defer server.Close()

	ep := testEndpointWithPrice(1, server.URL, "key", routing.ProtocolOpenAI, 0.01)
	pool := routing.NewEndpointPool([]*routing.Endpoint{ep}, 3)

	ctx := context.Background()
	rawReq := []byte(`{"model":"gpt-4","messages":[{"role":"user","content":[{"type":"text","data":"Hello"}]}]}`)

	resp, _, err := Request(ctx, pool, rawReq, routing.ProtocolOpenAI)
	if err != nil {
		t.Fatalf("expected success after retries, got: %v", err)
	}
	if len(resp) == 0 {
		t.Error("expected non-empty response")
	}

	if callCount != 3 {
		t.Errorf("expected 3 calls (2 rate limits + 1 success), got %d", callCount)
	}
}

// Backoff tests

func TestBackoffWithJitterZeroAttempt(t *testing.T) {
	// Attempt <= 0 should return 0
	if backoffWithJitter(0) != 0 {
		t.Error("expected 0 backoff for attempt 0")
	}
	if backoffWithJitter(-1) != 0 {
		t.Error("expected 0 backoff for negative attempt")
	}
}

func TestBackoffWithJitterFirstAttempt(t *testing.T) {
	// Attempt 1: base * 2^0 = 100ms
	for i := 0; i < 100; i++ {
		backoff := backoffWithJitter(1)
		if backoff < 0 || backoff > 100*time.Millisecond {
			t.Errorf("backoff out of range [0, 100ms]: %v", backoff)
		}
	}
}

func TestBackoffWithJitterExponential(t *testing.T) {
	// Verify exponential growth (with jitter, just check max bounds)
	// Attempt 2: base * 2^1 = 200ms
	// Attempt 3: base * 2^2 = 400ms
	// Attempt 4: base * 2^3 = 800ms
	// Attempt 5: base * 2^4 = 1600ms
	// Attempt 6: base * 2^5 = 3200ms
	// Attempt 7: base * 2^6 = 6400ms -> capped at 5000ms

	maxBackoffs := []time.Duration{
		100 * time.Millisecond,  // attempt 1
		200 * time.Millisecond,  // attempt 2
		400 * time.Millisecond,  // attempt 3
		800 * time.Millisecond,  // attempt 4
		1600 * time.Millisecond, // attempt 5
		3200 * time.Millisecond, // attempt 6
		5000 * time.Millisecond, // attempt 7 (capped)
		5000 * time.Millisecond, // attempt 8 (capped)
	}

	for attempt := 1; attempt <= 8; attempt++ {
		for i := 0; i < 10; i++ {
			backoff := backoffWithJitter(attempt)
			if backoff < 0 || backoff > maxBackoffs[attempt-1] {
				t.Errorf("attempt %d: backoff %v exceeds max %v", attempt, backoff, maxBackoffs[attempt-1])
			}
		}
	}
}

func TestBackoffWithJitterRandomness(t *testing.T) {
	// Verify jitter produces different values (probabilistic)
	var values []time.Duration
	for i := 0; i < 100; i++ {
		values = append(values, backoffWithJitter(5))
	}

	// With 100 samples, should have at least 10 unique values
	unique := make(map[time.Duration]bool)
	for _, v := range values {
		unique[v] = true
	}
	if len(unique) < 10 {
		t.Errorf("jitter not random enough: only %d unique values in 100 samples", len(unique))
	}
}

// Concurrent tests merged from concurrent_test.go

func TestRequestConcurrentCalls(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte(`{"id":"test","choices":[{"message":{"role":"assistant","content":[{"type":"text","data":"OK"}]}}],"usage":{"input_tokens":1,"output_tokens":1}}`))
	}))
	defer server.Close()

	ep1 := testEndpointWithPrice(1, server.URL, "key", routing.ProtocolOpenAI, 0.01)
	ep2 := testEndpointWithPrice(2, server.URL, "key", routing.ProtocolOpenAI, 0.02)

	pool := routing.NewEndpointPool([]*routing.Endpoint{ep1, ep2}, 3)

	ctx := context.Background()
	rawReq := []byte(`{"model":"gpt-4","messages":[{"role":"user","content":[{"type":"text","data":"Hello"}]}]}`)

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _, err := Request(ctx, pool, rawReq, routing.ProtocolOpenAI)
			if err != nil {
				t.Errorf("Request failed: %v", err)
			}
		}()
	}
	wg.Wait()
}

func TestTransportConcurrentCalls(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	ep := testEndpoint(1, server.URL, "key", routing.ProtocolOpenAI)

	ctx := context.Background()
	body := []byte(`{}`)

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := transport(ctx, ep, body)
			if err != nil {
				t.Errorf("transport failed: %v", err)
			}
		}()
	}
	wg.Wait()
}