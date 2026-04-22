package routing

// Protocol represents the communication protocol format for LLM requests.
type Protocol int

// Protocol constants define the supported communication formats.
const (
	ProtocolOpenAI Protocol = iota
	ProtocolAnthropic
	ProtocolGemini
	ProtocolCohere
)

// String returns the string representation of the protocol.
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

// Key represents connection credentials to an API.
// Key is immutable after creation - create a new Key if you need to change any field.
type Key struct {
	BaseURL  string    // API base URL
	APIKey   string    // API key (decrypted)
	Protocol Protocol  // Protocol format
}