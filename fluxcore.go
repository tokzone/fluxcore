package fluxcore

import "fmt"

// Protocol represents a communication protocol format for LLM API requests.
type Protocol int

const (
	ProtocolOpenAI Protocol = iota
	ProtocolAnthropic
	ProtocolGemini
	ProtocolCohere
)

// String returns the lowercase string representation of the protocol.
func (p Protocol) String() string {
	switch p {
	case ProtocolOpenAI:
		return "openai"
	case ProtocolAnthropic:
		return "anthropic"
	case ProtocolGemini:
		return "gemini"
	case ProtocolCohere:
		return "cohere"
	default:
		return "unknown"
	}
}

// ParseProtocol parses a string to a Protocol. Returns an error for unknown protocols.
func ParseProtocol(s string) (Protocol, error) {
	switch s {
	case "openai":
		return ProtocolOpenAI, nil
	case "anthropic":
		return ProtocolAnthropic, nil
	case "gemini":
		return ProtocolGemini, nil
	case "cohere":
		return ProtocolCohere, nil
	default:
		return ProtocolOpenAI, fmt.Errorf("invalid protocol: %q", s)
	}
}

// ProtocolPriority returns all protocols in deterministic priority order.
func ProtocolPriority() []Protocol {
	return []Protocol{ProtocolOpenAI, ProtocolAnthropic, ProtocolGemini, ProtocolCohere}
}

// Model is a model identifier string.
type Model string

// Service is a value object representing an external AI service provider.
type Service struct {
	Name     string
	BaseURLs map[Protocol]string
}

// BaseURLFor returns the base URL for the given protocol.
// Falls back to the OpenAI URL if the protocol is not configured.
func (s Service) BaseURLFor(proto Protocol) string {
	if url, ok := s.BaseURLs[proto]; ok && url != "" {
		return url
	}
	return s.BaseURLs[ProtocolOpenAI]
}
