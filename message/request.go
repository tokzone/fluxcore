package message

import (
	"encoding/json"
	"errors"
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
	Content []Content `json:"content"`
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

// Validate validates the MessageRequest fields.
func (r *MessageRequest) Validate() error {
	if r.Model == "" {
		return errors.New("model is required")
	}
	if len(r.Messages) == 0 {
		return errors.New("messages is required")
	}
	if r.Temperature < 0 || r.Temperature > 2 {
		return errors.New("temperature must be in [0, 2]")
	}
	if r.TopP < 0 || r.TopP > 1 {
		return errors.New("top_p must be in [0, 1]")
	}
	if r.MaxTokens < 0 {
		return errors.New("max_tokens must be non-negative")
	}
	return nil
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
