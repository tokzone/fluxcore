package routing

import "testing"

func TestProtocolString(t *testing.T) {
	t.Parallel()
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
		if got := tt.protocol.String(); got != tt.expected {
			t.Errorf("Protocol(%d).String() = %s, want %s", tt.protocol, got, tt.expected)
		}
	}
}