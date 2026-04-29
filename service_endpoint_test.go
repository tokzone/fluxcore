package fluxcore

import (
	"testing"
	"time"

	"github.com/tokzone/fluxcore/internal/health"
)

func TestServiceEndpoint_NewIsAvailable(t *testing.T) {
	se := NewServiceEndpoint(Service{Name: "test", BaseURLs: map[Protocol]string{ProtocolOpenAI: "https://example.com"}})

	if !se.IsAvailable() {
		t.Error("new service endpoint should be available")
	}
	if se.FailCount() != 0 {
		t.Errorf("fail count = %d, want 0", se.FailCount())
	}
}

func TestServiceEndpoint_MarkNetworkFailure(t *testing.T) {
	se := NewServiceEndpoint(Service{Name: "test", BaseURLs: map[Protocol]string{ProtocolOpenAI: "https://example.com"}})

	// Network CB has threshold=1, so a single failure trips it
	se.MarkNetworkFailure()
	if se.IsAvailable() {
		t.Error("should be unavailable after network failure (threshold=1)")
	}
	if se.FailCount() != 1 {
		t.Errorf("fail count = %d, want 1", se.FailCount())
	}
}

func TestServiceEndpoint_MarkSuccess(t *testing.T) {
	se := NewServiceEndpoint(Service{Name: "test", BaseURLs: map[Protocol]string{ProtocolOpenAI: "https://example.com"}})

	se.MarkNetworkFailure()
	if se.IsAvailable() {
		t.Fatal("should be tripped")
	}

	se.MarkSuccess()
	if !se.IsAvailable() {
		t.Error("should be available after MarkSuccess")
	}
	if se.FailCount() != 0 {
		t.Errorf("fail count = %d, want 0", se.FailCount())
	}
}

func TestServiceEndpoint_Service(t *testing.T) {
	svc := Service{Name: "openai", BaseURLs: map[Protocol]string{ProtocolOpenAI: "https://api.openai.com/v1"}}
	se := NewServiceEndpoint(svc)

	if got := se.Service().Name; got != "openai" {
		t.Errorf("service name = %q, want openai", got)
	}
}

func TestServiceEndpoint_Latency(t *testing.T) {
	se := NewServiceEndpoint(Service{Name: "test", BaseURLs: map[Protocol]string{ProtocolOpenAI: "https://example.com"}})

	if l := se.LatencyEWMA(); l != 0 {
		t.Errorf("initial latency = %d, want 0", l)
	}

	se.UpdateLatency(100)
	if l := se.LatencyEWMA(); l != 100 {
		t.Errorf("latency = %d, want 100", l)
	}
}

func TestServiceEndpoint_Recovery(t *testing.T) {
	// Create with short recovery for testing
	se := &ServiceEndpoint{
		svc: Service{Name: "test", BaseURLs: map[Protocol]string{ProtocolOpenAI: "https://example.com"}},
		cb:  health.New(health.Config{Threshold: 1, Recovery: 10 * time.Millisecond}),
	}

	se.MarkNetworkFailure()
	if se.IsAvailable() {
		t.Fatal("should be tripped")
	}

	time.Sleep(15 * time.Millisecond)
	if !se.IsAvailable() {
		t.Error("should recover after timeout")
	}
}
