package routing

import (
	"testing"
)

func BenchmarkSelectBest(b *testing.B) {
	endpoints := createTestEndpoints(10)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = selectBest(endpoints)
	}
}

func BenchmarkSelectBestLarge(b *testing.B) {
	endpoints := createTestEndpoints(100)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = selectBest(endpoints)
	}
}

func BenchmarkEndpointPoolCurrentEp(b *testing.B) {
	endpoints := createTestEndpoints(10)
	pool := NewEndpointPool(endpoints, 3)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = pool.CurrentEp()
	}
}

func BenchmarkEndpointMarkSuccess(b *testing.B) {
	endpoints := createTestEndpoints(10)
	ep := endpoints[0]

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ep.MarkSuccess()
	}
}

func BenchmarkEndpointPoolMarkFail(b *testing.B) {
	endpoints := createTestEndpoints(10)
	pool := NewEndpointPool(endpoints, 3)
	ep := endpoints[0]

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pool.MarkFail(ep)
		// Reset for next iteration
		ep.setHealthy(true)
		ep.state.failCount.Store(0)
	}
}

func BenchmarkEndpointPoolSelectBest(b *testing.B) {
	endpoints := createTestEndpoints(10)
	pool := NewEndpointPool(endpoints, 3)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = pool.SelectBest()
	}
}

func BenchmarkEndpointIsCircuitBreakerOpen(b *testing.B) {
	ep := &Endpoint{ID: 1, state: &endpointState{}}
	ep.setHealthy(true)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = ep.IsCircuitBreakerOpen()
	}
}

func createTestEndpoints(n int) []*Endpoint {
	endpoints := make([]*Endpoint, n)
	for i := 0; i < n; i++ {
		ep := &Endpoint{
			ID:          uint(i + 1),
			Key:         &Key{BaseURL: "https://api.example.com", APIKey: "key", Protocol: ProtocolOpenAI},
			Model:       "model",
			InputPrice:  float64(i+1) * 0.01,
			OutputPrice: float64(i+1) * 0.02,
			state:       &endpointState{},
		}
		ep.setHealthy(true)
		endpoints[i] = ep
	}
	return endpoints
}