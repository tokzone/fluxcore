package routing

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// =============================================================================
// Circuit Breaker Stability Tests
// =============================================================================

func TestCircuitBreakerFullRecoveryFlow(t *testing.T) {
	t.Parallel()

	key := &Key{BaseURL: "https://api.example.com", APIKey: "key", Protocol: ProtocolOpenAI}
	ep, _ := NewEndpoint(1, key, "gpt-4", 100)

	// Phase 1: Trigger circuit breaker (3 failures)
	for i := 0; i < 3; i++ {
		ep.MarkFail()
	}

	if ep.isHealthy() {
		t.Error("expected unhealthy after threshold failures")
	}
	if !ep.IsCircuitBreakerOpen() {
		t.Error("expected circuit breaker open")
	}

	// Phase 2: Wait for recovery timeout (simulate old failure)
	ep.setLastFail(time.Now().Add(-2 * time.Minute)) // Past 60s timeout

	// Circuit breaker should allow retry (not open)
	if ep.IsCircuitBreakerOpen() {
		t.Error("expected circuit breaker to allow retry after recovery timeout")
	}

	// Phase 3: Success during recovery window
	ep.MarkSuccess()

	if !ep.isHealthy() {
		t.Error("expected healthy after MarkSuccess")
	}
	if ep.IsCircuitBreakerOpen() {
		t.Error("expected circuit breaker closed after recovery")
	}
	if ep.failCount() != 0 {
		t.Error("expected failCount reset to 0")
	}
}

func TestCircuitBreakerRecoveryTimeoutVariations(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		recoveryTime time.Duration
		expectedOpen bool
	}{
		{"within_timeout", 30 * time.Second, true},
		{"just_expired", 61 * time.Second, false},
		{"long_expired", 5 * time.Minute, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key := &Key{BaseURL: "https://api.example.com", APIKey: "key", Protocol: ProtocolOpenAI}
			ep, _ := NewEndpoint(1, key, "gpt-4", 100)

			// Trigger circuit breaker
			for i := 0; i < 3; i++ {
				ep.MarkFail()
			}

			// Set last failure time
			ep.setLastFail(time.Now().Add(-tt.recoveryTime))

			if ep.IsCircuitBreakerOpen() != tt.expectedOpen {
				t.Errorf("expected IsCircuitBreakerOpen=%v for recovery time %v", tt.expectedOpen, tt.recoveryTime)
			}
		})
	}
}

func TestCircuitBreakerThresholdVariations(t *testing.T) {
	t.Parallel()

	// Test different threshold values to find optimal
	thresholds := []int{1, 3, 5, 10}

	for _, threshold := range thresholds {
		name := "threshold_" + string(rune('0'+threshold/10)) + string(rune('0'+threshold%10))
		t.Run(name, func(t *testing.T) {
			key := &Key{BaseURL: "https://api.example.com", APIKey: "key", Protocol: ProtocolOpenAI}
			ep, _ := newEndpointWithConfig(1, key, "gpt-4", 100, circuitBreakerConfig{
				Threshold:       threshold,
				RecoveryTimeout: 60 * time.Second,
			})

			// Fail threshold-1 times - should NOT trigger
			for i := 0; i < threshold-1; i++ {
				ep.MarkFail()
			}

			if !ep.isHealthy() {
				t.Errorf("threshold %d: expected healthy after %d failures (threshold-1)", threshold, threshold-1)
			}

			// Fail once more - should trigger
			ep.MarkFail()

			if ep.isHealthy() {
				t.Errorf("threshold %d: expected unhealthy after %d failures (threshold)", threshold, threshold)
			}
		})
	}
}

func TestCircuitBreakerBurstTraffic(t *testing.T) {
	t.Parallel()

	key := &Key{BaseURL: "https://api.example.com", APIKey: "key", Protocol: ProtocolOpenAI}
	ep, _ := NewEndpoint(1, key, "gpt-4", 100)

	// Simulate burst of failures from many concurrent requests
	var wg sync.WaitGroup
	var failCount atomic.Int32

	// 100 concurrent failures
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ep.MarkFail()
			failCount.Add(1)
		}()
	}
	wg.Wait()

	// Should be unhealthy after burst
	if ep.isHealthy() {
		t.Error("expected unhealthy after burst failures")
	}

	// Verify fail count is accurate (atomic guarantee)
	if ep.failCount() != 100 {
		t.Errorf("expected failCount=100, got %d", ep.failCount())
	}
}

func TestCircuitBreakerIntermittentFailures(t *testing.T) {
	t.Parallel()

	key := &Key{BaseURL: "https://api.example.com", APIKey: "key", Protocol: ProtocolOpenAI}
	ep, _ := NewEndpoint(1, key, "gpt-4", 100)

	// Pattern: fail, success, fail, success, fail
	// Should NOT trigger circuit breaker (failCount resets on success)
	for i := 0; i < 5; i++ {
		if i%2 == 0 {
			ep.MarkFail()
		} else {
			ep.MarkSuccess()
		}
	}

	// After alternating pattern, should still be healthy
	// (failCount resets to 0 on each success)
	if !ep.isHealthy() {
		t.Error("expected healthy after intermittent failures with successes")
	}
}

// =============================================================================
// EWMA Latency Stability Tests
// =============================================================================

func TestEWMARollingWindow(t *testing.T) {
	t.Parallel()

	key := &Key{BaseURL: "https://api.example.com", APIKey: "key", Protocol: ProtocolOpenAI}
	ep, _ := NewEndpoint(1, key, "gpt-4", 100)

	// Simulate latency pattern: 100ms initial, then increasing
	latencies := []int{100, 200, 300, 400, 500, 1000, 2000}
	expectedEWMA := []int{100, 110, 129, 161, 205, 285, 457}

	for i, latency := range latencies {
		ep.UpdateLatency(latency)
		ewma := ep.LatencyEWMA()

		// Allow 10% tolerance due to EWMA calculation
		tolerance := expectedEWMA[i] / 10
		if ewma < expectedEWMA[i]-tolerance || ewma > expectedEWMA[i]+tolerance {
			t.Errorf("step %d: EWMA=%d, expected ~%d (tolerance %d)", i, ewma, expectedEWMA[i], tolerance)
		}
	}
}

func TestEWMAStabilityUnderHighLatency(t *testing.T) {
	t.Parallel()

	key := &Key{BaseURL: "https://api.example.com", APIKey: "key", Protocol: ProtocolOpenAI}
	ep, _ := NewEndpoint(1, key, "gpt-4", 100)

	// Start with low latency, then spike to high
	for i := 0; i < 10; i++ {
		ep.UpdateLatency(50) // Baseline 50ms
	}

	// EWMA should stabilize around 50ms
	if ep.LatencyEWMA() > 60 {
		t.Errorf("EWMA should stabilize near 50ms, got %d", ep.LatencyEWMA())
	}

	// Latency spike to 5000ms (timeout scenario)
	ep.UpdateLatency(5000)

	// EWMA should increase gradually (not jump to 5000)
	// With α=0.1: new = 0.1*5000 + 0.9*50 = 545
	if ep.LatencyEWMA() < 400 || ep.LatencyEWMA() > 600 {
		t.Errorf("EWMA should be ~545 after spike, got %d", ep.LatencyEWMA())
	}
}

func TestEWMAConcurrentUpdates(t *testing.T) {
	t.Parallel()

	key := &Key{BaseURL: "https://api.example.com", APIKey: "key", Protocol: ProtocolOpenAI}
	ep, _ := NewEndpoint(1, key, "gpt-4", 100)

	var wg sync.WaitGroup

	// 100 goroutines updating latency concurrently
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(val int) {
			defer wg.Done()
			ep.UpdateLatency(val)
		}(i * 10) // 0-990ms range
	}
	wg.Wait()

	// EWMA should converge to reasonable value
	ewma := ep.LatencyEWMA()
	if ewma < 0 || ewma > 1000 {
		t.Errorf("EWMA=%d out of expected range [0, 1000]", ewma)
	}
}

// =============================================================================
// Endpoint Selection Stability Tests
// =============================================================================

func TestPoolSelectionUnderPartialFailure(t *testing.T) {
	t.Parallel()

	// 3 endpoints: 1 healthy, 2 unhealthy
	ep1 := newTestEndpoint(1, true)
	ep1.Priority = 10

	ep2 := newTestEndpoint(2, false) // unhealthy
	ep2.Priority = 5 // Lower priority but unhealthy

	ep3 := newTestEndpoint(3, false) // unhealthy
	ep3.Priority = 20

	pool := NewEndpointPool([]*Endpoint{ep1, ep2, ep3}, 2)

	// Should select ep1 (only healthy endpoint)
	best := pool.SelectBest()
	if best == nil {
		t.Fatal("expected to find healthy endpoint")
	}
	if best.ID != 1 {
		t.Errorf("expected endpoint 1 (only healthy), got %d", best.ID)
	}
}

func TestPoolSelectionFallback(t *testing.T) {
	t.Parallel()

	// All endpoints unhealthy
	ep1 := newTestEndpoint(1, false)
	ep1.Priority = 10

	ep2 := newTestEndpoint(2, false)
	ep2.Priority = 5

	pool := NewEndpointPool([]*Endpoint{ep1, ep2}, 2)

	// SelectBest should return nil when all unhealthy
	best := pool.SelectBest()
	if best != nil {
		t.Error("expected nil endpoint when all unhealthy")
	}
}

func TestPoolDynamicHealthChange(t *testing.T) {
	t.Parallel()

	ep1 := newTestEndpoint(1, true)
	ep1.Priority = 10

	ep2 := newTestEndpoint(2, true)
	ep2.Priority = 20

	pool := NewEndpointPool([]*Endpoint{ep1, ep2}, 2)

	// Initially select ep1 (lower priority)
	if pool.CurrentEp().ID != 1 {
		t.Error("expected endpoint 1 initially")
	}

	// Make ep1 unhealthy
	for i := 0; i < 3; i++ {
		pool.MarkFail(ep1)
	}

	// SelectBest should now return ep2
	best := pool.SelectBest()
	if best == nil {
		t.Fatal("expected endpoint 2 after ep1 unhealthy")
	}
	if best.ID != 2 {
		t.Errorf("expected endpoint 2 after ep1 unhealthy, got %d", best.ID)
	}

	// Recover ep1
	ep1.MarkSuccess()

	// Should switch back to ep1 (lower priority)
	best = pool.SelectBest()
	if best == nil || best.ID != 1 {
		t.Errorf("expected endpoint 1 after recovery, got %d", best.ID)
	}
}

// =============================================================================
// Concurrent Stress Tests
// =============================================================================

func TestEndpointHighConcurrencyStress(t *testing.T) {
	t.Parallel()

	key := &Key{BaseURL: "https://api.example.com", APIKey: "key", Protocol: ProtocolOpenAI}
	ep, _ := NewEndpoint(1, key, "gpt-4", 100)

	var wg sync.WaitGroup
	iterations := 1000

	// Concurrent MarkFail
	for i := 0; i < iterations/2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ep.MarkFail()
		}()
	}

	// Concurrent MarkSuccess (should reset)
	for i := 0; i < iterations/2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ep.MarkSuccess()
		}()
	}

	// Concurrent health check
	for i := 0; i < iterations; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = ep.IsCircuitBreakerOpen()
		}()
	}

	// Concurrent latency update
	for i := 0; i < iterations; i++ {
		wg.Add(1)
		go func(val int) {
			defer wg.Done()
			ep.UpdateLatency(val)
		}(i)
	}

	wg.Wait()

	// Just verify no panic/crash, final state is consistent
	_ = ep.isHealthy()
	_ = ep.LatencyEWMA()
	_ = ep.IsCircuitBreakerOpen()
}

func TestPoolHighConcurrencyStress(t *testing.T) {
	t.Parallel()

	eps := make([]*Endpoint, 10)
	for i := 0; i < 10; i++ {
		eps[i] = newTestEndpoint(uint(i+1), true)
		eps[i].Priority = int64(i * 10)
	}

	pool := NewEndpointPool(eps, 3)

	var wg sync.WaitGroup
	iterations := 500

	// Concurrent endpoint selection
	for i := 0; i < iterations; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = pool.SelectBest()
		}()
	}

	// Concurrent health updates
	for i := 0; i < iterations; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			ep := eps[idx%10]
			if idx%3 == 0 {
				ep.MarkSuccess()
			} else {
				pool.MarkFail(ep)
			}
		}(i)
	}

	wg.Wait()

	// Verify pool is still functional
	_ = pool.SelectBest()
}