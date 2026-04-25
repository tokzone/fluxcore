package endpoint

import (
	"testing"

	"github.com/tokzone/fluxcore/provider"
)

func TestNewEndpoint(t *testing.T) {
	prov := provider.NewProvider(1, "https://api.openai.com", provider.ProtocolOpenAI)

	ep, err := NewEndpoint(1, prov, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ep == nil {
		t.Fatal("expected Endpoint")
	}
	if ep.ID != 1 {
		t.Errorf("expected ID 1, got %d", ep.ID)
	}
	if ep.Provider() != prov {
		t.Error("expected same provider")
	}
	if ep.Model != "" {
		t.Errorf("expected empty model, got %s", ep.Model)
	}
}

func TestNewEndpointNilProvider(t *testing.T) {
	_, err := NewEndpoint(1, nil, "")
	if err == nil {
		t.Fatal("expected error for nil provider")
	}
}

func TestNewEndpointWithModel(t *testing.T) {
	prov := provider.NewProvider(1, "https://generativelanguage.googleapis.com", provider.ProtocolGemini)

	ep, err := NewEndpoint(1, prov, "gemini-pro")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ep.Model != "gemini-pro" {
		t.Errorf("expected model gemini-pro, got %s", ep.Model)
	}
}

func TestEndpointIsAvailable(t *testing.T) {
	prov := provider.NewProvider(1, "https://api.openai.com", provider.ProtocolOpenAI)
	ep, _ := NewEndpoint(1, prov, "")

	// Initially available
	if !ep.IsAvailable() {
		t.Error("expected endpoint to be available initially")
	}

	// Mark failures
	ep.MarkEndpointFail()
	if !ep.IsAvailable() {
		t.Error("expected still available after 1 failure (threshold=3)")
	}

	ep.MarkEndpointFail()
	if !ep.IsAvailable() {
		t.Error("expected still available after 2 failures (threshold=3)")
	}

	ep.MarkEndpointFail()
	if ep.IsAvailable() {
		t.Error("expected unavailable after 3 failures")
	}

	// Success resets
	ep.MarkEndpointSuccess()
	if !ep.IsAvailable() {
		t.Error("expected available after success")
	}
}

func TestEndpointCircuitBreaker(t *testing.T) {
	prov := provider.NewProvider(1, "https://api.openai.com", provider.ProtocolOpenAI)
	ep, _ := NewEndpoint(1, prov, "")

	if ep.IsCircuitBreakerOpen() {
		t.Error("expected circuit breaker closed initially")
	}

	// Trigger circuit breaker
	for i := 0; i < 3; i++ {
		ep.MarkEndpointFail()
	}

	if !ep.IsCircuitBreakerOpen() {
		t.Error("expected circuit breaker open after 3 failures")
	}
}

func TestEndpointLatencyEWMA(t *testing.T) {
	prov := provider.NewProvider(1, "https://api.openai.com", provider.ProtocolOpenAI)
	ep, _ := NewEndpoint(1, prov, "")

	// Initial latency should be 0
	if ep.EndpointLatencyEWMA() != 0 {
		t.Errorf("expected initial latency 0, got %d", ep.EndpointLatencyEWMA())
	}

	// First update
	ep.UpdateEndpointLatency(100)
	if ep.EndpointLatencyEWMA() != 100 {
		t.Errorf("expected latency 100, got %d", ep.EndpointLatencyEWMA())
	}

	// EWMA update: 0.1 * 200 + 0.9 * 100 = 110
	ep.UpdateEndpointLatency(200)
	if ep.EndpointLatencyEWMA() != 110 {
		t.Errorf("expected latency 110, got %d", ep.EndpointLatencyEWMA())
	}
}

func TestEndpointProtocol(t *testing.T) {
	prov := provider.NewProvider(1, "https://api.anthropic.com", provider.ProtocolAnthropic)
	ep, _ := NewEndpoint(1, prov, "")

	if ep.Protocol() != provider.ProtocolAnthropic {
		t.Errorf("expected ProtocolAnthropic, got %v", ep.Protocol())
	}
}

func TestEndpointBaseURL(t *testing.T) {
	prov := provider.NewProvider(1, "https://api.openai.com", provider.ProtocolOpenAI)
	ep, _ := NewEndpoint(1, prov, "")

	if ep.BaseURL() != "https://api.openai.com" {
		t.Errorf("expected https://api.openai.com, got %s", ep.BaseURL())
	}
}

func TestValidate(t *testing.T) {
	prov := provider.NewProvider(1, "https://api.openai.com", provider.ProtocolOpenAI)
	ep, _ := NewEndpoint(1, prov, "")

	if err := ep.Validate(); err != nil {
		t.Errorf("unexpected validation error: %v", err)
	}
}

func TestValidateGeminiRequiresModel(t *testing.T) {
	prov := provider.NewProvider(1, "https://generativelanguage.googleapis.com", provider.ProtocolGemini)
	ep, _ := NewEndpoint(1, prov, "") // Empty model

	if err := ep.Validate(); err == nil {
		t.Fatal("expected validation error for Gemini without model")
	}
}

func TestValidateGeminiWithModel(t *testing.T) {
	prov := provider.NewProvider(1, "https://generativelanguage.googleapis.com", provider.ProtocolGemini)
	ep, _ := NewEndpoint(1, prov, "gemini-pro")

	if err := ep.Validate(); err != nil {
		t.Errorf("unexpected validation error: %v", err)
	}
}
