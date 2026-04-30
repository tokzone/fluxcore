package message

import (
	"encoding/json"
	"fmt"
)

// MessageRequest is the unified request format (OpenAI format as IR)
type MessageRequest struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	MaxTokens   int       `json:"max_tokens,omitempty"`
	Temperature float64   `json:"temperature,omitempty"`
	TopP        float64   `json:"top_p,omitempty"`
	Stream      bool      `json:"stream,omitempty"`
}

// Message represents a single message
type Message struct {
	Role    string    `json:"role"`
	Content []Content `json:"-"`
}

// UnmarshalJSON implements custom JSON unmarshaling for Message.
// Accepts both string content ("content": "Hello") and array content ("content": [...]).
func (m *Message) UnmarshalJSON(data []byte) error {
	var raw struct {
		Role    string          `json:"role"`
		Content json.RawMessage `json:"content"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	m.Role = raw.Role
	if len(raw.Content) == 0 {
		return nil
	}
	if raw.Content[0] == '"' {
		var text string
		if err := json.Unmarshal(raw.Content, &text); err != nil {
			return fmt.Errorf("unmarshal message text content: %w", err)
		}
		m.Content = []Content{TextContent(text)}
		return nil
	}
	var contents []Content
	if err := json.Unmarshal(raw.Content, &contents); err != nil {
		return fmt.Errorf("unmarshal message content array: %w", err)
	}
	m.Content = contents
	return nil
}

// MarshalJSON implements custom JSON marshaling for Message.
// Text-only content is serialized as a plain string; mixed content as an array.
func (m Message) MarshalJSON() ([]byte, error) {
	if allText(m.Content) {
		return json.Marshal(map[string]interface{}{
			"role":    m.Role,
			"content": ExtractAllText(m.Content),
		})
	}
	return json.Marshal(map[string]interface{}{
		"role":    m.Role,
		"content": m.Content,
	})
}

// allText returns true if all content items are text type.
func allText(contents []Content) bool {
	for _, c := range contents {
		if !c.IsText() {
			return false
		}
	}
	return true
}

// MessageResponse is the unified response format
type MessageResponse struct {
	ID      string   `json:"id"`
	Model   string   `json:"model"`
	Choices []Choice `json:"choices"`
	Usage   *Usage   `json:"usage,omitempty"`
}

// Choice represents a response choice
type Choice struct {
	Index        int     `json:"index"`
	Message      Message `json:"message"`
	FinishReason string  `json:"finish_reason,omitempty"`
}

// Usage represents token usage statistics
type Usage struct {
	InputTokens  int  `json:"input_tokens"`
	OutputTokens int  `json:"output_tokens"`
	LatencyMs    int  `json:"latency_ms"`
	IsAccurate   bool `json:"is_accurate"` // Provider returned accurate usage (not estimated)
}

// UnmarshalJSON handles both OpenAI wire format (prompt_tokens/completion_tokens)
// and IR format (input_tokens/output_tokens).
func (u *Usage) UnmarshalJSON(data []byte) error {
	// Try OpenAI standard field names first
	var openai struct {
		PromptTokens     int  `json:"prompt_tokens"`
		CompletionTokens int  `json:"completion_tokens"`
		LatencyMs        int  `json:"latency_ms"`
		IsAccurate       bool `json:"is_accurate"`
	}
	if err := json.Unmarshal(data, &openai); err != nil {
		return err
	}
	if openai.PromptTokens > 0 || openai.CompletionTokens > 0 {
		u.InputTokens = openai.PromptTokens
		u.OutputTokens = openai.CompletionTokens
		u.LatencyMs = openai.LatencyMs
		u.IsAccurate = openai.IsAccurate
		return nil
	}

	// Fall back to IR field names
	var ir struct {
		InputTokens  int  `json:"input_tokens"`
		OutputTokens int  `json:"output_tokens"`
		LatencyMs    int  `json:"latency_ms"`
		IsAccurate   bool `json:"is_accurate"`
	}
	if err := json.Unmarshal(data, &ir); err != nil {
		return err
	}
	u.InputTokens = ir.InputTokens
	u.OutputTokens = ir.OutputTokens
	u.LatencyMs = ir.LatencyMs
	u.IsAccurate = ir.IsAccurate
	return nil
}

// StreamChunk represents a streaming response chunk (OpenAI format)
type StreamChunk struct {
	ID      string         `json:"id"`
	Object  string         `json:"object"`
	Model   string         `json:"model"`
	Choices []StreamChoice `json:"choices"`
	Usage   *Usage         `json:"usage,omitempty"`
}

// StreamChoice represents a streaming choice
type StreamChoice struct {
	Index        int     `json:"index"`
	Delta        Message `json:"delta"`
	FinishReason *string `json:"finish_reason,omitempty"`
}

// ParseRequest parses JSON bytes into a MessageRequest.
func ParseRequest(data []byte) (*MessageRequest, error) {
	var req MessageRequest
	if err := json.Unmarshal(data, &req); err != nil {
		return nil, fmt.Errorf("parse request: %w", err)
	}
	return &req, nil
}

// ParseResponse parses JSON bytes into a MessageResponse.
func ParseResponse(data []byte) (*MessageResponse, error) {
	var resp MessageResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}
	return &resp, nil
}

// WithStream returns a copy of the request with Stream set to the given value.
func (r *MessageRequest) WithStream(stream bool) *MessageRequest {
	newReq := *r
	newReq.Stream = stream
	return &newReq
}
