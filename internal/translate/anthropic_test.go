package translate

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/tokzone/fluxcore/message"
)

func TestAnthropicRequestRoundTrip(t *testing.T) {
	anthropicReq := map[string]interface{}{
		"model":      "claude-3-opus",
		"max_tokens": 100,
		"messages": []interface{}{
			map[string]interface{}{
				"role":    "user",
				"content": "Hello",
			},
		},
	}

	reqBytes, _ := json.Marshal(anthropicReq)

	req, err := AnthropicToMessageRequest(bytes.NewReader(reqBytes))
	if err != nil {
		t.Fatalf("AnthropicToMessageRequest failed: %v", err)
	}

	if req.Model != "claude-3-opus" {
		t.Errorf("expected model claude-3-opus, got %s", req.Model)
	}

	if len(req.Messages) != 1 {
		t.Errorf("expected 1 message, got %d", len(req.Messages))
	}

	// Convert back to Anthropic format
	outBytes, err := MessageRequestToAnthropic(req)
	if err != nil {
		t.Fatalf("MessageRequestToAnthropic failed: %v", err)
	}

	var out map[string]interface{}
	json.Unmarshal(outBytes, &out)

	if out["model"] != "claude-3-opus" {
		t.Errorf("expected model in output, got %v", out["model"])
	}
}

func TestAnthropicResponseRoundTrip(t *testing.T) {
	anthropicResp := map[string]interface{}{
		"id":    "msg-123",
		"model": "claude-3-opus",
		"content": []interface{}{
			map[string]interface{}{
				"type": "text",
				"text": "Hello there!",
			},
		},
		"stop_reason": "end_turn",
		"usage": map[string]int{
			"input_tokens":  10,
			"output_tokens": 5,
		},
	}

	respBytes, _ := json.Marshal(anthropicResp)

	resp, err := AnthropicResponseToMessageResponse(respBytes)
	if err != nil {
		t.Fatalf("AnthropicResponseToMessageResponse failed: %v", err)
	}

	if resp.ID != "msg-123" {
		t.Errorf("expected ID msg-123, got %s", resp.ID)
	}

	if len(resp.Choices) != 1 {
		t.Errorf("expected 1 choice, got %d", len(resp.Choices))
	}

	if resp.Usage.InputTokens != 10 {
		t.Errorf("expected input tokens 10, got %d", resp.Usage.InputTokens)
	}

	// Convert back
	outBytes, err := MessageResponseToAnthropic(resp)
	if err != nil {
		t.Fatalf("MessageResponseToAnthropic failed: %v", err)
	}

	var out map[string]interface{}
	json.Unmarshal(outBytes, &out)

	if out["id"] != "msg-123" {
		t.Errorf("expected id in output, got %v", out["id"])
	}
}

func TestAnthropicSSEConversion(t *testing.T) {
	// Test Anthropic SSE to OpenAI SSE
	anthropicEvent := map[string]interface{}{
		"type":  "content_block_delta",
		"index": 0,
		"delta": map[string]interface{}{
			"type": "text_delta",
			"text": "Hello",
		},
	}

	eventBytes, _ := json.Marshal(anthropicEvent)
	converted := AnthropicSSEToOpenAISSE(eventBytes)

	if converted == nil {
		t.Fatal("expected converted output, got nil")
	}

	// Check it starts with "data: "
	if !bytes.HasPrefix(converted, []byte("data: ")) {
		t.Error("expected output to start with 'data: '")
	}

	// Test OpenAI SSE to Anthropic SSE
	openaiChunk := &message.StreamChunk{
		ID:    "test",
		Model: "gpt-4",
		Choices: []message.StreamChoice{
			{
				Index: 0,
				Delta: message.Message{
					Content: []message.Content{message.TextContent("Hello")},
				},
			},
		},
	}

	events := OpenAISSEToAnthropicSSE(openaiChunk)
	if len(events) == 0 {
		t.Fatal("expected events, got empty")
	}

	// Check event format
	if !bytes.Contains([]byte(events[0]), []byte("event: content_block_delta")) {
		t.Error("expected content_block_delta event type")
	}
}

func TestAnthropicContentArrayParsing(t *testing.T) {
	anthropicReq := map[string]interface{}{
		"model": "claude-3",
		"messages": []interface{}{
			map[string]interface{}{
				"role": "user",
				"content": []interface{}{
					map[string]interface{}{
						"type": "text",
						"text": "Hello",
					},
					map[string]interface{}{
						"type": "image",
						"source": map[string]interface{}{
							"type": "url",
							"url":  "https://example.com/image.png",
						},
					},
				},
			},
		},
	}

	reqBytes, _ := json.Marshal(anthropicReq)
	req, err := AnthropicToMessageRequest(bytes.NewReader(reqBytes))
	if err != nil {
		t.Fatalf("AnthropicToMessageRequest failed: %v", err)
	}

	if len(req.Messages) != 1 {
		t.Errorf("expected 1 message, got %d", len(req.Messages))
	}

	// Currently only text content is extracted
	if len(req.Messages[0].Content) == 0 {
		t.Error("expected content to be extracted")
	}
}
