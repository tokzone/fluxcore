package provider

import (
	"testing"
)

func TestNewProvider(t *testing.T) {
	p := NewProvider(1, "https://api.openai.com", ProtocolOpenAI)
	if p == nil {
		t.Fatal("expected Provider")
	}
	if p.ID != 1 {
		t.Errorf("expected ID 1, got %d", p.ID)
	}
	if p.BaseURL != "https://api.openai.com" {
		t.Errorf("expected BaseURL https://api.openai.com, got %s", p.BaseURL)
	}
	if p.Protocol != ProtocolOpenAI {
		t.Errorf("expected ProtocolOpenAI, got %v", p.Protocol)
	}
}

func TestProtocolString(t *testing.T) {
	tests := []struct {
		protocol Protocol
		expected string
	}{
		{ProtocolOpenAI, "openai"},
		{ProtocolAnthropic, "anthropic"},
		{ProtocolGemini, "gemini"},
		{ProtocolCohere, "cohere"},
		{Protocol(999), "unknown"},
	}

	for _, tt := range tests {
		if tt.protocol.String() != tt.expected {
			t.Errorf("expected %s, got %s", tt.expected, tt.protocol.String())
		}
	}
}

func TestProviderCircuitBreaker(t *testing.T) {
	p := NewProvider(1, "https://api.openai.com", ProtocolOpenAI)

	// Initially closed
	if p.IsCircuitBreakerOpen() {
		t.Error("expected circuit breaker closed initially")
	}

	// Provider has threshold=1, so one failure triggers circuit
	p.MarkProviderFail()

	if !p.IsCircuitBreakerOpen() {
		t.Error("expected circuit breaker open after 1 failure (threshold=1)")
	}

	// Success resets
	p.MarkProviderSuccess()

	if p.IsCircuitBreakerOpen() {
		t.Error("expected circuit breaker closed after success")
	}
}

func TestProviderLatencyEWMA(t *testing.T) {
	p := NewProvider(1, "https://api.openai.com", ProtocolOpenAI)

	// Initial latency should be 0
	if p.ProviderLatencyEWMA() != 0 {
		t.Errorf("expected initial latency 0, got %d", p.ProviderLatencyEWMA())
	}

	// First update
	p.UpdateProviderLatency(100)
	if p.ProviderLatencyEWMA() != 100 {
		t.Errorf("expected latency 100, got %d", p.ProviderLatencyEWMA())
	}

	// EWMA update: 0.1 * 200 + 0.9 * 100 = 110
	p.UpdateProviderLatency(200)
	if p.ProviderLatencyEWMA() != 110 {
		t.Errorf("expected latency 110, got %d", p.ProviderLatencyEWMA())
	}
}

func TestProviderIsAvailable(t *testing.T) {
	p := NewProvider(1, "https://api.openai.com", ProtocolOpenAI)

	// Initially available (circuit breaker closed)
	if p.IsCircuitBreakerOpen() {
		t.Error("expected available initially")
	}

	// Mark failure
	p.MarkProviderFail()

	if !p.IsCircuitBreakerOpen() {
		t.Error("expected unavailable after failure")
	}
}
