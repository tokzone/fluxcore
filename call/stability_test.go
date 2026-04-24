package call

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	fluxerrors "github.com/tokzone/fluxcore/errors"
	"github.com/tokzone/fluxcore/routing"
)

// =============================================================================
// Network Latency & Timeout Tests
// =============================================================================

func TestTransportSlowResponse(t *testing.T) {
	t.Parallel()

	// Simulate slow response (within timeout)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second) // 2s delay
		w.WriteHeader(200)
		w.Write([]byte(`{"id":"test","choices":[],"usage":{"input_tokens":0,"output_tokens":0}}`))
	}))
	defer server.Close()

	ep := testEndpoint(1, server.URL, "key", routing.ProtocolOpenAI)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := transport(ctx, ep, []byte(`{}`))
	if err != nil {
		t.Errorf("slow response should succeed within timeout: %v", err)
	}
}

func TestTransportTimeoutExceeded(t *testing.T) {
	t.Parallel()

	// Simulate response that exceeds timeout
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(10 * time.Second) // 10s delay
		w.WriteHeader(200)
	}))
	defer server.Close()

	ep := testEndpoint(1, server.URL, "key", routing.ProtocolOpenAI)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	_, err := transport(ctx, ep, []byte(`{}`))
	if err == nil {
		t.Error("expected timeout error")
	}

	var classified *fluxerrors.ClassifiedError
	if !errors.As(err, &classified) {
		t.Errorf("expected ClassifiedError, got %T", err)
	} else if classified.Code != fluxerrors.CodeTimeout {
		t.Errorf("expected CodeTimeout, got %s", classified.Code)
	}
}

func TestTransportConnectionRefused(t *testing.T) {
	t.Parallel()

	// Invalid port (connection refused)
	ep := testEndpoint(1, "http://localhost:9999", "key", routing.ProtocolOpenAI)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err := transport(ctx, ep, []byte(`{}`))
	if err == nil {
		t.Error("expected connection error")
	}

	var classified *fluxerrors.ClassifiedError
	if !errors.As(err, &classified) {
		t.Errorf("expected ClassifiedError, got %T", err)
	} else if classified.Code != fluxerrors.CodeNetworkError {
		t.Errorf("expected CodeNetworkError, got %s", classified.Code)
	}
}

func TestTransportIntermittentFailures(t *testing.T) {
	t.Parallel()

	var callCount atomic.Int32

	// Server that fails first 2 calls, then succeeds
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := callCount.Add(1)
		if count <= 2 {
			w.WriteHeader(503) // Service unavailable
			w.Write([]byte(`{"error":"temporarily unavailable"}`))
		} else {
			w.WriteHeader(200)
			w.Write([]byte(`{"id":"test","choices":[],"usage":{"input_tokens":0,"output_tokens":0}}`))
		}
	}))
	defer server.Close()

	ep1 := testEndpointWithPriority(1, server.URL, "key1", routing.ProtocolOpenAI, 10)
	ep2 := testEndpointWithPriority(2, server.URL, "key2", routing.ProtocolOpenAI, 20)

	pool := routing.NewEndpointPool([]*routing.Endpoint{ep1, ep2}, 3)

	ctx := context.Background()
	rawReq := []byte(`{"model":"gpt-4","messages":[{"role":"user","content":[{"type":"text","data":"Hi"}]}]}`)

	_, _, err := Request(ctx, pool, rawReq, routing.ProtocolOpenAI)
	if err != nil {
		t.Errorf("expected success after retries, got: %v", err)
	}

	if callCount.Load() != 3 {
		t.Errorf("expected 3 calls (2 failures + 1 success), got %d", callCount.Load())
	}
}

// =============================================================================
// Error Classification Tests (using existing MarkFail API)
// =============================================================================

func TestErrorClassificationServerError(t *testing.T) {
	t.Parallel()

	// ServerError (5xx) should be retryable
	err := fluxerrors.ClassifyHTTPError(500, "internal error")

	if !fluxerrors.IsRetryable(err) {
		t.Error("server error should be retryable")
	}
	if err.Code != fluxerrors.CodeServerError {
		t.Errorf("expected CodeServerError, got %s", err.Code)
	}
}

func TestErrorClassificationRateLimit(t *testing.T) {
	t.Parallel()

	// RateLimit (429) should be retryable
	err := fluxerrors.ClassifyHTTPError(429, "rate limited")

	if !fluxerrors.IsRetryable(err) {
		t.Error("rate limit should be retryable")
	}
	if err.Code != fluxerrors.CodeRateLimit {
		t.Errorf("expected CodeRateLimit, got %s", err.Code)
	}
}

func TestErrorClassificationAuthError(t *testing.T) {
	t.Parallel()

	// Auth error (401/403) should NOT be retryable
	err := fluxerrors.ClassifyHTTPError(401, "unauthorized")

	if fluxerrors.IsRetryable(err) {
		t.Error("auth error should NOT be retryable")
	}
	if err.Code != fluxerrors.CodeAuthError {
		t.Errorf("expected CodeAuthError, got %s", err.Code)
	}
}

func TestErrorClassificationNetworkError(t *testing.T) {
	t.Parallel()

	// Network errors should be retryable
	err := fluxerrors.ClassifyNetError(context.DeadlineExceeded)

	if !fluxerrors.IsRetryable(err) {
		t.Error("timeout should be retryable")
	}
	if err.Code != fluxerrors.CodeTimeout {
		t.Errorf("expected CodeTimeout, got %s", err.Code)
	}
}

// =============================================================================
// Retry Backoff Parameter Tests
// =============================================================================

func TestBackoffOptimalParameters(t *testing.T) {
	t.Parallel()

	// Verify backoff curve is reasonable for different scenarios
	tests := []struct {
		name     string
		attempt  int
		maxBound time.Duration
	}{
		{"attempt_1", 1, 100 * time.Millisecond},
		{"attempt_2", 2, 200 * time.Millisecond},
		{"attempt_3", 3, 400 * time.Millisecond},
		{"attempt_5", 5, 1600 * time.Millisecond},
		{"attempt_7", 7, 5000 * time.Millisecond}, // capped at max
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for i := 0; i < 100; i++ {
				backoff := backoffWithJitter(tt.attempt)
				if backoff < 0 {
					t.Error("backoff should not be negative")
				}
				if backoff > tt.maxBound {
					t.Errorf("backoff %v exceeds max bound %v", backoff, tt.maxBound)
				}
			}
		})
	}
}

func TestBackoffDistribution(t *testing.T) {
	t.Parallel()

	// Collect samples to verify jitter distribution
	samples := make([]time.Duration, 1000)
	for i := 0; i < 1000; i++ {
		samples[i] = backoffWithJitter(5) // 1600ms max
	}

	// Check distribution properties
	min, max := samples[0], samples[0]
	for _, s := range samples {
		if s < min {
			min = s
		}
		if s > max {
			max = s
		}
	}

	// With full jitter [0, max], should see variety
	if min > 100*time.Millisecond {
		t.Errorf("min backoff should be near 0, got %v", min)
	}
	if max > 1600*time.Millisecond {
		t.Errorf("max backoff should be <=1600ms, got %v", max)
	}
}

// =============================================================================
// Real-world Scenario Tests
// =============================================================================

func TestRequestWithFlakyServer(t *testing.T) {
	t.Parallel()

	var callCount atomic.Int32

	// Server that alternates success/failure
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := callCount.Add(1)
		if count%2 == 0 {
			w.WriteHeader(200)
			w.Write([]byte(`{"id":"test","choices":[],"usage":{"input_tokens":0,"output_tokens":0}}`))
		} else {
			w.WriteHeader(503)
			w.Write([]byte(`{"error":"service unavailable"}`))
		}
	}))
	defer server.Close()

	ep := testEndpointWithPriority(1, server.URL, "key", routing.ProtocolOpenAI, 10)
	pool := routing.NewEndpointPool([]*routing.Endpoint{ep}, 5)

	ctx := context.Background()
	rawReq := []byte(`{"model":"gpt-4","messages":[{"role":"user","content":[{"type":"text","data":"Hi"}]}]}`)

	// Multiple requests - some should succeed
	var successCount atomic.Int32
	var wg sync.WaitGroup

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _, err := Request(ctx, pool, rawReq, routing.ProtocolOpenAI)
			if err == nil {
				successCount.Add(1)
			}
		}()
	}
	wg.Wait()

	// At least some requests should succeed
	if successCount.Load() == 0 {
		t.Error("expected at least some successful requests")
	}
}

func TestRequestWithSlowEndpoints(t *testing.T) {
	t.Parallel()

	// Fast endpoint
	fastServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte(`{"id":"test","choices":[],"usage":{"input_tokens":0,"output_tokens":0}}`))
	}))
	defer fastServer.Close()

	// Slow endpoint (500ms)
	slowServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(500 * time.Millisecond)
		w.WriteHeader(200)
		w.Write([]byte(`{"id":"test","choices":[],"usage":{"input_tokens":0,"output_tokens":0}}`))
	}))
	defer slowServer.Close()

	fastEp := testEndpointWithPriority(1, fastServer.URL, "key", routing.ProtocolOpenAI, 10)
	slowEp := testEndpointWithPriority(2, slowServer.URL, "key", routing.ProtocolOpenAI, 20) // Higher priority (preferred)

	pool := routing.NewEndpointPool([]*routing.Endpoint{fastEp, slowEp}, 3)

	ctx := context.Background()
	rawReq := []byte(`{"model":"gpt-4","messages":[{"role":"user","content":[{"type":"text","data":"Hi"}]}]}`)

	// Request should prefer lower priority endpoint
	_, _, err := Request(ctx, pool, rawReq, routing.ProtocolOpenAI)
	if err != nil {
		t.Errorf("request should succeed: %v", err)
	}
}

// =============================================================================
// Concurrent Request Stress Tests
// =============================================================================

func TestRequestHighConcurrencyStress(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 95% success rate
		if time.Now().UnixNano()%20 == 0 {
			w.WriteHeader(500)
		} else {
			w.WriteHeader(200)
			w.Write([]byte(`{"id":"test","choices":[],"usage":{"input_tokens":0,"output_tokens":0}}`))
		}
	}))
	defer server.Close()

	ep1 := testEndpointWithPriority(1, server.URL, "key1", routing.ProtocolOpenAI, 10)
	ep2 := testEndpointWithPriority(2, server.URL, "key2", routing.ProtocolOpenAI, 20)

	pool := routing.NewEndpointPool([]*routing.Endpoint{ep1, ep2}, 3)

	ctx := context.Background()
	rawReq := []byte(`{"model":"gpt-4","messages":[{"role":"user","content":[{"type":"text","data":"Hi"}]}]}`)

	var wg sync.WaitGroup
	var successCount atomic.Int32
	var failCount atomic.Int32

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _, err := Request(ctx, pool, rawReq, routing.ProtocolOpenAI)
			if err == nil {
				successCount.Add(1)
			} else {
				failCount.Add(1)
			}
		}()
	}
	wg.Wait()

	t.Logf("results: %d success, %d fail", successCount.Load(), failCount.Load())

	// Should have mostly successes
	if successCount.Load() < 50 {
		t.Errorf("expected at least 50 successes, got %d", successCount.Load())
	}
}

func TestStreamHighConcurrencyStress(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)
		w.Write([]byte(sseOpenAIEvent("chunk")))
		w.Write([]byte(sseDone()))
	}))
	defer server.Close()

	ep1 := testEndpointWithPriority(1, server.URL, "key1", routing.ProtocolOpenAI, 10)
	ep2 := testEndpointWithPriority(2, server.URL, "key2", routing.ProtocolOpenAI, 20)

	pool := routing.NewEndpointPool([]*routing.Endpoint{ep1, ep2}, 3)

	ctx := context.Background()
	rawReq := []byte(`{"model":"gpt-4","messages":[{"role":"user","content":[{"type":"text","data":"Hi"}]}]}`)

	var wg sync.WaitGroup
	var successCount atomic.Int32

	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			result, err := RequestStream(ctx, pool, rawReq, routing.ProtocolOpenAI)
			if err != nil {
				return
			}
			defer result.Close()
			for range result.Ch {
			}
			successCount.Add(1)
		}()
	}
	wg.Wait()

	if successCount.Load() < 40 {
		t.Errorf("expected at least 40 stream successes, got %d", successCount.Load())
	}
}

func TestMixedHTTPStatusCodes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		statusCode int
		retryable  bool
	}{
		{"400_bad_request", 400, false},
		{"401_unauthorized", 401, false},
		{"403_forbidden", 403, false},
		{"404_not_found", 404, false},
		{"429_rate_limit", 429, true},
		{"500_server_error", 500, true},
		{"502_bad_gateway", 502, true},
		{"503_service_unavailable", 503, true},
		{"504_gateway_timeout", 504, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := fluxerrors.ClassifyHTTPError(tt.statusCode, "test")

			if fluxerrors.IsRetryable(err) != tt.retryable {
				t.Errorf("status %d: expected retryable=%v, got %v", tt.statusCode, tt.retryable, fluxerrors.IsRetryable(err))
			}
		})
	}
}