package message

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestMessageRequestMarshal(t *testing.T) {
	req := &MessageRequest{
		Model:    "gpt-4",
		Messages: []Message{{Role: "user", Content: []Content{TextContent("hello")}}},
		Stream:   false,
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var parsed MessageRequest
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if parsed.Model != "gpt-4" {
		t.Errorf("expected model gpt-4, got %s", parsed.Model)
	}
	if len(parsed.Messages) != 1 {
		t.Errorf("expected 1 message, got %d", len(parsed.Messages))
	}
}

func TestTextContent(t *testing.T) {
	c := TextContent("test")
	if c.Type != "text" {
		t.Errorf("expected type text, got %s", c.Type)
	}
	if c.AsText() != "test" {
		t.Errorf("expected text test, got %s", c.AsText())
	}
}

func TestStreamChunkJSON(t *testing.T) {
	chunk := &StreamChunk{
		ID:     "test-123",
		Object: "chat.completion.chunk",
		Model:  "gpt-4",
		Choices: []StreamChoice{
			{
				Index: 0,
				Delta: Message{
					Content: []Content{TextContent("hello")},
				},
			},
		},
	}

	data, err := json.Marshal(chunk)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var parsed StreamChunk
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if parsed.ID != "test-123" {
		t.Errorf("expected ID test-123, got %s", parsed.ID)
	}
}

func TestParseRequest(t *testing.T) {
	jsonData := `{"model":"gpt-4","messages":[{"role":"user","content":[{"type":"text","data":"Hello"}]}],"max_tokens":100}`
	req, err := ParseRequest([]byte(jsonData))
	if err != nil {
		t.Fatalf("ParseRequest failed: %v", err)
	}
	if req.Model != "gpt-4" {
		t.Errorf("expected model gpt-4, got %s", req.Model)
	}
	if len(req.Messages) != 1 {
		t.Errorf("expected 1 message, got %d", len(req.Messages))
	}
	if req.MaxTokens != 100 {
		t.Errorf("expected max_tokens 100, got %d", req.MaxTokens)
	}
}

func TestParseRequestInvalidJSON(t *testing.T) {
	_, err := ParseRequest([]byte(`{invalid}`))
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestParseRequestEmpty(t *testing.T) {
	req, err := ParseRequest([]byte(`{}`))
	if err != nil {
		t.Fatalf("ParseRequest failed: %v", err)
	}
	if req.Model != "" {
		t.Errorf("expected empty model, got %s", req.Model)
	}
}

func TestParseResponse(t *testing.T) {
	jsonData := `{"id":"test-123","model":"gpt-4","choices":[{"index":0,"message":{"role":"assistant","content":[{"type":"text","data":"Hello"}]},"finish_reason":"stop"}],"usage":{"input_tokens":10,"output_tokens":5}}`
	resp, err := ParseResponse([]byte(jsonData))
	if err != nil {
		t.Fatalf("ParseResponse failed: %v", err)
	}
	if resp.ID != "test-123" {
		t.Errorf("expected ID test-123, got %s", resp.ID)
	}
	if len(resp.Choices) != 1 {
		t.Errorf("expected 1 choice, got %d", len(resp.Choices))
	}
	if resp.Usage == nil {
		t.Fatal("expected usage")
	}
	if resp.Usage.InputTokens != 10 {
		t.Errorf("expected input_tokens 10, got %d", resp.Usage.InputTokens)
	}
}

func TestParseResponseInvalidJSON(t *testing.T) {
	_, err := ParseResponse([]byte(`{invalid}`))
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestWithStream(t *testing.T) {
	req := &MessageRequest{
		Model:    "gpt-4",
		Messages: []Message{{Role: "user", Content: []Content{TextContent("hello")}}},
		Stream:   false,
	}

	newReq := req.WithStream(true)
	if newReq.Stream != true {
		t.Error("expected Stream to be true")
	}
	// Original should be unchanged
	if req.Stream != false {
		t.Error("original request should be unchanged")
	}
	// Should be a copy, not same pointer
	if newReq == req {
		t.Error("WithStream should return a copy")
	}
}

func TestMessageRequestValidate(t *testing.T) {
	tests := []struct {
		name    string
		req     *MessageRequest
		wantErr bool
		errMsg  string
	}{
		{
			name:    "missing_model",
			req:     &MessageRequest{},
			wantErr: true,
			errMsg:  "model is required",
		},
		{
			name:    "missing_messages",
			req:     &MessageRequest{Model: "gpt-4"},
			wantErr: true,
			errMsg:  "messages is required",
		},
		{
			name:    "negative_temperature",
			req:     &MessageRequest{Model: "gpt-4", Messages: []Message{{}}, Temperature: -0.1},
			wantErr: true,
			errMsg:  "temperature must be in [0, 2]",
		},
		{
			name:    "temperature_too_high",
			req:     &MessageRequest{Model: "gpt-4", Messages: []Message{{}}, Temperature: 2.1},
			wantErr: true,
			errMsg:  "temperature must be in [0, 2]",
		},
		{
			name:    "temperature_boundary_zero",
			req:     &MessageRequest{Model: "gpt-4", Messages: []Message{{}}, Temperature: 0},
			wantErr: false,
		},
		{
			name:    "temperature_boundary_two",
			req:     &MessageRequest{Model: "gpt-4", Messages: []Message{{}}, Temperature: 2},
			wantErr: false,
		},
		{
			name:    "negative_top_p",
			req:     &MessageRequest{Model: "gpt-4", Messages: []Message{{}}, TopP: -0.1},
			wantErr: true,
			errMsg:  "top_p must be in [0, 1]",
		},
		{
			name:    "top_p_too_high",
			req:     &MessageRequest{Model: "gpt-4", Messages: []Message{{}}, TopP: 1.1},
			wantErr: true,
			errMsg:  "top_p must be in [0, 1]",
		},
		{
			name:    "negative_max_tokens",
			req:     &MessageRequest{Model: "gpt-4", Messages: []Message{{}}, MaxTokens: -1},
			wantErr: true,
			errMsg:  "max_tokens must be non-negative",
		},
		{
			name:    "valid_request",
			req:     &MessageRequest{Model: "gpt-4", Messages: []Message{{Role: "user", Content: []Content{TextContent("hi")}}}},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.req.Validate()
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error containing %q, got nil", tt.errMsg)
				} else if !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("error %q should contain %q", err.Error(), tt.errMsg)
				}
			} else {
				if err != nil {
					t.Errorf("expected no error, got %v", err)
				}
			}
		})
	}
}
func FuzzParseRequest(f *testing.F) {
	f.Add([]byte(`{"model":"gpt-4","messages":[]}`))
	f.Add([]byte(`{}`))
	f.Add([]byte(``))

	f.Fuzz(func(t *testing.T, data []byte) {
		// Should not panic
		req, err := ParseRequest(data)
		_ = req
		_ = err
	})
}

func FuzzParseResponse(f *testing.F) {
	f.Add([]byte(`{"id":"test","choices":[],"usage":{}}`))
	f.Add([]byte(`{}`))
	f.Add([]byte(``))

	f.Fuzz(func(t *testing.T, data []byte) {
		// Should not panic
		resp, err := ParseResponse(data)
		_ = resp
		_ = err
	})
}
