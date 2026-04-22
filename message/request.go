package message

import "encoding/json"

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

// TotalTokens returns total tokens used
func (u *Usage) TotalTokens() int {
	return u.InputTokens + u.OutputTokens
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
		return nil, err
	}
	return &req, nil
}

// ParseResponse parses JSON bytes into a MessageResponse.
func ParseResponse(data []byte) (*MessageResponse, error) {
	var resp MessageResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// WithStream returns a copy of the request with Stream set to the given value.
func (r *MessageRequest) WithStream(stream bool) *MessageRequest {
	newReq := *r
	newReq.Stream = stream
	return &newReq
}

// WithModel returns a copy of the request with Model set to the given value.
func (r *MessageRequest) WithModel(model string) *MessageRequest {
	newReq := *r
	newReq.Model = model
	return &newReq
}

// WithMaxTokens returns a copy of the request with MaxTokens set to the given value.
func (r *MessageRequest) WithMaxTokens(maxTokens int) *MessageRequest {
	newReq := *r
	newReq.MaxTokens = maxTokens
	return &newReq
}

// Clone returns a deep copy of the request.
func (r *MessageRequest) Clone() *MessageRequest {
	newReq := *r
	newReq.Messages = make([]Message, len(r.Messages))
	copy(newReq.Messages, r.Messages)
	return &newReq
}