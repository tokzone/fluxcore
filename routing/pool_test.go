package routing

import (
	"sync"
	"testing"
	"time"
)

// helper to create endpoint with proper atomic initialization
func newTestEndpoint(id uint, healthy bool) *Endpoint {
	ep := &Endpoint{
		ID:    id,
		Key:   &Key{BaseURL: "http://test", APIKey: "key", Protocol: ProtocolOpenAI},
		state: &endpointState{},
	}
	ep.setHealthy(healthy)
	return ep
}

func TestNewEndpointPool(t *testing.T) {
	ep1 := newTestEndpoint(1, true)
	ep1.InputPrice = 0.01
	ep1.OutputPrice = 0.03

	ep2 := newTestEndpoint(2, true)
	ep2.InputPrice = 0.02
	ep2.OutputPrice = 0.04

	ep3 := newTestEndpoint(3, false)

	endpoints := []*Endpoint{ep1, ep2, ep3}
	pool := NewEndpointPool(endpoints, 2)

	if pool.RetryMax() != 2 {
		t.Errorf("expected retry max 2, got %d", pool.RetryMax())
	}

	if pool.CurrentEp() == nil {
		t.Error("expected current endpoint to be set")
	}

	// Should select cheapest (ep1)
	if pool.CurrentEp().ID != 1 {
		t.Errorf("expected endpoint 1 as best, got %d", pool.CurrentEp().ID)
	}
}

func TestPoolMarkSuccess(t *testing.T) {
	ep := newTestEndpoint(1, true)
	pool := NewEndpointPool([]*Endpoint{ep}, 2)

	// Mark fail twice
	pool.MarkFail(ep)
	pool.MarkFail(ep)
	if ep.failCount() != 2 {
		t.Errorf("expected fail count 2 after two failures, got %d", ep.failCount())
	}

	// Mark success should reset
	ep.MarkSuccess()
	if ep.failCount() != 0 {
		t.Errorf("expected fail count 0 after success, got %d", ep.failCount())
	}
	if !ep.isHealthy() {
		t.Error("expected endpoint to be healthy after success")
	}
}

func TestPoolMarkFailCircuitBreaker(t *testing.T) {
	ep := newTestEndpoint(1, true)
	pool := NewEndpointPool([]*Endpoint{ep}, 2)

	// Fail 3 times to trigger circuit breaker
	for i := 0; i < 3; i++ {
		pool.MarkFail(ep)
	}

	if ep.isHealthy() {
		t.Error("expected endpoint to be unhealthy after 3 failures")
	}

	// IsCircuitBreakerOpen should return true for unhealthy endpoint
	if !ep.IsCircuitBreakerOpen() {
		t.Error("expected IsCircuitBreakerOpen to return true for unhealthy endpoint")
	}
}

func TestPoolConcurrentAccess(t *testing.T) {
	ep1 := newTestEndpoint(1, true)
	ep1.InputPrice = 0.01
	ep1.OutputPrice = 0.03

	ep2 := newTestEndpoint(2, true)
	ep2.InputPrice = 0.02
	ep2.OutputPrice = 0.04

	endpoints := []*Endpoint{ep1, ep2}
	pool := NewEndpointPool(endpoints, 2)

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(3)
		go func() {
			defer wg.Done()
			_ = pool.CurrentEp()
		}()
		go func() {
			defer wg.Done()
			ep1.MarkSuccess()
		}()
		go func() {
			defer wg.Done()
			pool.MarkFail(ep2)
		}()
	}
	wg.Wait()
}

func TestPoolConcurrentMarkFail(t *testing.T) {
	ep := newTestEndpoint(1, true)
	pool := NewEndpointPool([]*Endpoint{ep}, 2)

	var wg sync.WaitGroup
	// 100 goroutines concurrently MarkFail
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			pool.MarkFail(ep)
		}()
	}
	wg.Wait()

	// FailCount should be exactly 100 (atomic guarantees)
	if ep.failCount() != 100 {
		t.Errorf("expected fail count 100 after 100 concurrent MarkFail, got %d", ep.failCount())
	}

	// Should be unhealthy (>= 3 failures)
	if ep.isHealthy() {
		t.Error("expected endpoint to be unhealthy after many failures")
	}
}

func TestPoolSelectBest(t *testing.T) {
	tests := []struct {
		name      string
		endpoints []*Endpoint
		expected  uint
	}{
		{
			name: "select cheapest",
			endpoints: func() []*Endpoint {
				ep1 := newTestEndpoint(1, true)
				ep1.InputPrice = 10
				ep1.OutputPrice = 20

				ep2 := newTestEndpoint(2, true)
				ep2.InputPrice = 1
				ep2.OutputPrice = 2

				return []*Endpoint{ep1, ep2}
			}(),
			expected: 2,
		},
		{
			name: "select faster when same price",
			endpoints: func() []*Endpoint {
				ep1 := newTestEndpoint(1, true)
				ep1.InputPrice = 1
				ep1.OutputPrice = 2
				ep1.LatencyMs = 500

				ep2 := newTestEndpoint(2, true)
				ep2.InputPrice = 1
				ep2.OutputPrice = 2
				ep2.LatencyMs = 100

				return []*Endpoint{ep1, ep2}
			}(),
			expected: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pool := NewEndpointPool(tt.endpoints, 2)
			if pool.CurrentEp().ID != tt.expected {
				t.Errorf("expected %d, got %d", tt.expected, pool.CurrentEp().ID)
			}
		})
	}
}

func TestIsCircuitBreakerOpenRecoveryTimeout(t *testing.T) {
	t.Run("recent_fail_should_skip", func(t *testing.T) {
		ep := newTestEndpoint(1, false)
		ep.setLastFail(time.Now()) // Recent failure

		// Should skip (within recovery timeout)
		if !ep.IsCircuitBreakerOpen() {
			t.Error("expected IsCircuitBreakerOpen=true for recent failure")
		}
	})

	t.Run("old_fail_can_retry", func(t *testing.T) {
		ep := newTestEndpoint(1, false)
		ep.setLastFail(time.Now().Add(-2 * DefaultRecoveryTimeout)) // Old failure

		// Should NOT skip (past recovery timeout)
		if ep.IsCircuitBreakerOpen() {
			t.Error("expected IsCircuitBreakerOpen=false for old failure (past recovery timeout)")
		}
	})

	t.Run("healthy_never_skip", func(t *testing.T) {
		ep := newTestEndpoint(1, true)

		// Should NOT skip (healthy)
		if ep.IsCircuitBreakerOpen() {
			t.Error("expected IsCircuitBreakerOpen=false for healthy endpoint")
		}
	})

	t.Run("unhealthy_no_fail_time", func(t *testing.T) {
		ep := newTestEndpoint(1, false)
		// No LastFail set (zero value)

		// Should skip (no recovery info)
		if !ep.IsCircuitBreakerOpen() {
			t.Error("expected IsCircuitBreakerOpen=true when unhealthy with no fail time")
		}
	})
}

func TestPoolMarkSuccessResetsHealth(t *testing.T) {
	t.Run("success_resets_unhealthy_endpoint", func(t *testing.T) {
		ep := newTestEndpoint(1, true)
		pool := NewEndpointPool([]*Endpoint{ep}, 2)

		// Fail it
		for i := 0; i < DefaultCircuitBreakerThreshold; i++ {
			pool.MarkFail(ep)
		}

		// Should be unhealthy
		if ep.isHealthy() {
			t.Error("expected endpoint to be unhealthy after failures")
		}

		// Mark success
		ep.MarkSuccess()

		// Should be healthy now
		if !ep.isHealthy() {
			t.Error("expected endpoint to be healthy after MarkSuccess")
		}

		// IsCircuitBreakerOpen should be false now
		if ep.IsCircuitBreakerOpen() {
			t.Error("expected IsCircuitBreakerOpen=false after recovery")
		}
	})
}

// Concurrent tests

func TestConcurrentHealthStatus(t *testing.T) {
	ep := &Endpoint{ID: 1, state: &endpointState{}}
	ep.setHealthy(true)

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(setHealthy bool) {
			defer wg.Done()
			if setHealthy {
				ep.setHealthy(true)
			} else {
				ep.setHealthy(false)
			}
		}(i%2 == 0)
	}
	wg.Wait()

	// Final state should be consistent (true or false)
	_ = ep.isHealthy()
}

func TestPoolRaceConditions(t *testing.T) {
	endpoints := []*Endpoint{
		{ID: 1, Key: &Key{Protocol: ProtocolOpenAI}, InputPrice: 0.01, state: &endpointState{}},
		{ID: 2, Key: &Key{Protocol: ProtocolOpenAI}, InputPrice: 0.02, state: &endpointState{}},
	}
	for _, ep := range endpoints {
		ep.setHealthy(true)
	}

	pool := NewEndpointPool(endpoints, 3)

	var wg sync.WaitGroup

	// Reader goroutines
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				_ = pool.CurrentEp()
				_ = pool.SelectBest()
			}
		}()
	}

	// Writer goroutines
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			ep := endpoints[idx%2]
			for j := 0; j < 50; j++ {
				if j%3 == 0 {
					ep.MarkSuccess()
				} else {
					pool.MarkFail(ep)
				}
			}
		}(i)
	}

	wg.Wait()
}

func TestIsCircuitBreakerOpenConcurrent(t *testing.T) {
	ep := &Endpoint{ID: 1, state: &endpointState{}}
	ep.setHealthy(false)
	ep.setLastFail(time.Now().Add(-30 * time.Second)) // Within recovery timeout

	var wg sync.WaitGroup
	skipCount := 0
	var countMu sync.Mutex

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if ep.IsCircuitBreakerOpen() {
				countMu.Lock()
				skipCount++
				countMu.Unlock()
			}
		}()
	}
	wg.Wait()

	// All reads should return consistent result (should skip)
	if skipCount != 100 {
		t.Errorf("expected 100 skips, got %d", skipCount)
	}
}