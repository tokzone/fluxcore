package fluxcore

import (
	"testing"
	"time"

	"github.com/tokzone/fluxcore/internal/health"
)

func TestRoute_IdentityKey(t *testing.T) {
	se := NewServiceEndpoint(Service{Name: "openai", BaseURLs: map[Protocol]string{ProtocolOpenAI: "https://api.openai.com/v1"}})
	desc := RouteDesc{SvcEP: se, Model: "gpt-4", Credential: "sk-abc123", Priority: 0}
	r := NewRoute(desc)

	key := r.IdentityKey()
	if key != "openai/gpt-4/sk-abc123" {
		t.Errorf("IdentityKey = %q, want openai/gpt-4/sk-abc123", key)
	}

	// Same desc → same key
	r2 := NewRoute(desc)
	if r.IdentityKey() != r2.IdentityKey() {
		t.Error("routes with same desc should have same IdentityKey")
	}

	// Different credential → different key
	desc2 := RouteDesc{SvcEP: se, Model: "gpt-4", Credential: "sk-xyz", Priority: 0}
	r3 := NewRoute(desc2)
	if r.IdentityKey() == r3.IdentityKey() {
		t.Error("routes with different credentials should have different IdentityKey")
	}
}

func TestRoute_IsAvailable(t *testing.T) {
	se := NewServiceEndpoint(Service{Name: "test", BaseURLs: map[Protocol]string{ProtocolOpenAI: "https://example.com"}})
	r := NewRoute(RouteDesc{SvcEP: se, Model: "gpt-4", Credential: "sk-key", Priority: 0})

	if !r.IsAvailable() {
		t.Error("new route should be available")
	}

	// Model failure (threshold=3, so takes 3 to trip)
	r.MarkModelFailure()
	r.MarkModelFailure()
	if !r.IsAvailable() {
		t.Error("should still be available after 2 model failures")
	}
	r.MarkModelFailure()
	if r.IsAvailable() {
		t.Error("should be unavailable after 3 model failures")
	}

	// Service endpoint failure also affects route
	se2 := NewServiceEndpoint(Service{Name: "test2", BaseURLs: map[Protocol]string{ProtocolOpenAI: "https://example.com"}})
	r2 := NewRoute(RouteDesc{SvcEP: se2, Model: "gpt-4", Credential: "sk-key2", Priority: 0})
	if !r2.IsAvailable() {
		t.Error("new route should be available")
	}
	se2.MarkNetworkFailure()
	if r2.IsAvailable() {
		t.Error("route should be unavailable when service endpoint is down")
	}
}

func TestRoute_Desc(t *testing.T) {
	se := NewServiceEndpoint(Service{Name: "openai", BaseURLs: map[Protocol]string{ProtocolOpenAI: "https://api.openai.com/v1"}})
	desc := RouteDesc{SvcEP: se, Model: "gpt-4", Credential: "sk-key", Priority: 5}
	r := NewRoute(desc)

	got := r.Desc()
	if got.Model != "gpt-4" || got.Credential != "sk-key" || got.Priority != 5 {
		t.Error("Desc() should return the original RouteDesc")
	}
}

func TestRoute_SvcEP(t *testing.T) {
	se := NewServiceEndpoint(Service{Name: "test", BaseURLs: map[Protocol]string{ProtocolOpenAI: "https://example.com"}})
	r := NewRoute(RouteDesc{SvcEP: se, Model: "m", Credential: "k", Priority: 0})

	if r.SvcEP() != se {
		t.Error("SvcEP() should return the shared reference")
	}
}

func TestRoute_MarkSuccess(t *testing.T) {
	se := NewServiceEndpoint(Service{Name: "test", BaseURLs: map[Protocol]string{ProtocolOpenAI: "https://example.com"}})
	r := NewRoute(RouteDesc{SvcEP: se, Model: "gpt-4", Credential: "sk-key", Priority: 0})

	r.MarkModelFailure()
	r.MarkModelFailure()
	r.MarkModelFailure()
	if r.IsAvailable() {
		t.Fatal("should be tripped")
	}

	r.MarkSuccess()
	if !r.IsAvailable() {
		t.Error("should be available after MarkSuccess")
	}
	if r.FailCount() != 0 {
		t.Errorf("fail count = %d, want 0", r.FailCount())
	}
}

func TestRoute_Latency(t *testing.T) {
	se := NewServiceEndpoint(Service{Name: "test", BaseURLs: map[Protocol]string{ProtocolOpenAI: "https://example.com"}})
	r := NewRoute(RouteDesc{SvcEP: se, Model: "m", Credential: "k", Priority: 0})

	if l := r.LatencyEWMA(); l != 0 {
		t.Errorf("initial latency = %d, want 0", l)
	}

	r.UpdateLatency(100)
	if l := r.LatencyEWMA(); l != 100 {
		t.Errorf("latency = %d, want 100", l)
	}
}

func TestRoute_Recovery(t *testing.T) {
	se := NewServiceEndpoint(Service{Name: "test", BaseURLs: map[Protocol]string{ProtocolOpenAI: "https://example.com"}})
	r := &Route{
		desc: RouteDesc{SvcEP: se, Model: "m", Credential: "k", Priority: 0},
		cb:   health.New(health.Config{Threshold: 1, Recovery: 10 * time.Millisecond}),
	}

	r.MarkModelFailure()
	if r.IsAvailable() {
		t.Fatal("should be tripped")
	}

	time.Sleep(15 * time.Millisecond)
	if !r.IsAvailable() {
		t.Error("should recover after timeout")
	}
}
