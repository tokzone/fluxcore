package fluxcore

import (
	"testing"
)

func TestProtocol_String(t *testing.T) {
	tests := []struct {
		p    Protocol
		want string
	}{
		{ProtocolOpenAI, "openai"},
		{ProtocolAnthropic, "anthropic"},
		{ProtocolGemini, "gemini"},
		{ProtocolCohere, "cohere"},
		{Protocol(99), "unknown"},
	}
	for _, tt := range tests {
		if got := tt.p.String(); got != tt.want {
			t.Errorf("Protocol(%d).String() = %q, want %q", tt.p, got, tt.want)
		}
	}
}

func TestService_BaseURLFor(t *testing.T) {
	svc := Service{
		Name: "openai",
		BaseURLs: map[Protocol]string{
			ProtocolOpenAI: "https://api.openai.com/v1",
		},
	}

	if got := svc.BaseURLFor(ProtocolOpenAI); got != "https://api.openai.com/v1" {
		t.Errorf("expected openai URL, got %q", got)
	}
	if got := svc.BaseURLFor(ProtocolAnthropic); got != "https://api.openai.com/v1" {
		t.Errorf("should fallback to openai, got %q", got)
	}
}
