package translate

import (
	"bytes"
	"testing"

	"github.com/tokzone/fluxcore/message"
)

func TestAnthropicSSEToOpenAISSEEventTypes(t *testing.T) {
	t.Run("content_block_delta", func(t *testing.T) {
		data := []byte(`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hello"}}`)
		result := AnthropicSSEToOpenAISSE(data)
		if result == nil {
			t.Error("expected non-nil result")
		}
		if !bytes.Contains(result, []byte("data:")) {
			t.Error("expected data: prefix")
		}
	})

	t.Run("message_delta", func(t *testing.T) {
		data := []byte(`{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":10}}`)
		result := AnthropicSSEToOpenAISSE(data)
		if result == nil {
			t.Error("expected non-nil result")
		}
		if !bytes.Contains(result, []byte("finish_reason")) {
			t.Error("expected finish_reason in result")
		}
	})

	t.Run("message_start", func(t *testing.T) {
		data := []byte(`{"type":"message_start","message":{"id":"msg_123","role":"assistant"}}`)
		result := AnthropicSSEToOpenAISSE(data)
		// message_start is not converted (returns nil)
		if result != nil {
			t.Error("expected nil for message_start event")
		}
	})

	t.Run("content_block_start", func(t *testing.T) {
		data := []byte(`{"type":"content_block_start","index":0,"content_block":{"type":"text"}}`)
		result := AnthropicSSEToOpenAISSE(data)
		// content_block_start is not converted (returns nil)
		if result != nil {
			t.Error("expected nil for content_block_start event")
		}
	})

	t.Run("content_block_stop", func(t *testing.T) {
		data := []byte(`{"type":"content_block_stop","index":0}`)
		result := AnthropicSSEToOpenAISSE(data)
		// content_block_stop is not converted (returns nil)
		if result != nil {
			t.Error("expected nil for content_block_stop event")
		}
	})

	t.Run("invalid_json", func(t *testing.T) {
		data := []byte(`{invalid}`)
		result := AnthropicSSEToOpenAISSE(data)
		if result != nil {
			t.Error("expected nil for invalid JSON")
		}
	})

	t.Run("unknown_type", func(t *testing.T) {
		data := []byte(`{"type":"unknown_event"}`)
		result := AnthropicSSEToOpenAISSE(data)
		if result != nil {
			t.Error("expected nil for unknown event type")
		}
	})
}

func TestOpenAISSEToAnthropicSSEEventTypes(t *testing.T) {
	t.Run("with_role", func(t *testing.T) {
		chunk := &message.StreamChunk{
			ID:    "msg_123",
			Model: "claude-3",
			Choices: []message.StreamChoice{
				{
					Delta: message.Message{
						Role: "assistant",
					},
				},
			},
		}
		events := OpenAISSEToAnthropicSSE(chunk)
		if len(events) == 0 {
			t.Error("expected at least one event")
		}
		if !bytes.Contains([]byte(events[0]), []byte("message_start")) {
			t.Error("expected message_start event")
		}
	})

	t.Run("with_content", func(t *testing.T) {
		chunk := &message.StreamChunk{
			Choices: []message.StreamChoice{
				{
					Index: 0,
					Delta: message.Message{
						Content: []message.Content{message.TextContent("Hello")},
					},
				},
			},
		}
		events := OpenAISSEToAnthropicSSE(chunk)
		if len(events) == 0 {
			t.Error("expected at least one event")
		}
		if !bytes.Contains([]byte(events[0]), []byte("content_block_delta")) {
			t.Error("expected content_block_delta event")
		}
	})

	t.Run("with_finish_reason", func(t *testing.T) {
		finishReason := "end_turn"
		chunk := &message.StreamChunk{
			Choices: []message.StreamChoice{
				{
					FinishReason: &finishReason,
				},
			},
		}
		events := OpenAISSEToAnthropicSSE(chunk)
		if len(events) == 0 {
			t.Error("expected at least one event")
		}
		if !bytes.Contains([]byte(events[0]), []byte("message_delta")) {
			t.Error("expected message_delta event")
		}
	})

	t.Run("with_usage", func(t *testing.T) {
		finishReason := "end_turn"
		chunk := &message.StreamChunk{
			Choices: []message.StreamChoice{
				{
					FinishReason: &finishReason,
				},
			},
			Usage: &message.Usage{
				OutputTokens: 10,
			},
		}
		events := OpenAISSEToAnthropicSSE(chunk)
		if len(events) == 0 {
			t.Error("expected at least one event")
		}
		// Should contain usage info
		found := false
		for _, e := range events {
			if bytes.Contains([]byte(e), []byte("output_tokens")) {
				found = true
				break
			}
		}
		if !found {
			t.Error("expected output_tokens in events")
		}
	})

	t.Run("empty_choices", func(t *testing.T) {
		chunk := &message.StreamChunk{
			Choices: []message.StreamChoice{},
		}
		events := OpenAISSEToAnthropicSSE(chunk)
		if events != nil {
			t.Error("expected nil for empty choices")
		}
	})

	t.Run("complete_message", func(t *testing.T) {
		finishReason := "end_turn"
		chunk := &message.StreamChunk{
			ID:    "msg_123",
			Model: "claude-3",
			Choices: []message.StreamChoice{
				{
					Index: 0,
					Delta: message.Message{
						Role:    "assistant",
						Content: []message.Content{message.TextContent("Hello")},
					},
					FinishReason: &finishReason,
				},
			},
			Usage: &message.Usage{
				OutputTokens: 5,
			},
		}
		events := OpenAISSEToAnthropicSSE(chunk)
		// Should have 3 events: message_start, content_block_delta, message_delta
		if len(events) != 3 {
			t.Errorf("expected 3 events, got %d", len(events))
		}
	})
}

func TestGeminiSSEToOpenAISSEEventTypes(t *testing.T) {
	t.Run("text_content", func(t *testing.T) {
		data := []byte(`{"candidates":[{"content":{"parts":[{"text":"Hello"}]},"index":0}]}`)
		result := GeminiSSEToOpenAISSE(data)
		if result == nil {
			t.Error("expected non-nil result")
		}
	})

	t.Run("with_finish_reason", func(t *testing.T) {
		data := []byte(`{"candidates":[{"content":{"parts":[{"text":"Hello"}]},"finishReason":"STOP","index":0}]}`)
		result := GeminiSSEToOpenAISSE(data)
		if result == nil {
			t.Error("expected non-nil result")
		}
		if !bytes.Contains(result, []byte("finish_reason")) {
			t.Error("expected finish_reason in result")
		}
	})

	t.Run("with_usage", func(t *testing.T) {
		data := []byte(`{"candidates":[{"content":{"parts":[{"text":"Hello"}]},"index":0}],"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":5}}`)
		result := GeminiSSEToOpenAISSE(data)
		if result == nil {
			t.Error("expected non-nil result")
		}
	})

	t.Run("invalid_json", func(t *testing.T) {
		data := []byte(`{invalid}`)
		result := GeminiSSEToOpenAISSE(data)
		if result != nil {
			t.Error("expected nil for invalid JSON")
		}
	})
}

func TestCohereSSEToOpenAISSEEventTypes(t *testing.T) {
	t.Run("text_generation", func(t *testing.T) {
		data := []byte(`{"event_type":"text-generation","text":"Hello"}`)
		result := CohereSSEToOpenAISSE(data)
		if result == nil {
			t.Error("expected non-nil result")
		}
	})

	t.Run("stream_end", func(t *testing.T) {
		data := []byte(`{"event_type":"stream-end","finish_reason":"complete","response":{"tokens":{"input_tokens":10,"output_tokens":5}}}`)
		result := CohereSSEToOpenAISSE(data)
		if result == nil {
			t.Error("expected non-nil result")
		}
	})

	t.Run("other_event_type", func(t *testing.T) {
		data := []byte(`{"event_type":"stream-start","text":""}`)
		result := CohereSSEToOpenAISSE(data)
		if result != nil {
			t.Error("expected nil for non-text-generation event")
		}
	})

	t.Run("invalid_json", func(t *testing.T) {
		data := []byte(`{invalid}`)
		result := CohereSSEToOpenAISSE(data)
		if result != nil {
			t.Error("expected nil for invalid JSON")
		}
	})
}

func TestConvertSSEEventIndirectConversion(t *testing.T) {
	t.Run("anthropic_to_gemini_via_openai", func(t *testing.T) {
		event := SSEEvent{
			Type: SSETypeData,
			Data: []byte(`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hello"}}`),
		}
		result := ConvertSSEEvent(event, "anthropic", "gemini")
		if result == nil {
			t.Error("expected non-nil result for indirect conversion")
		}
	})

	t.Run("gemini_to_cohere_via_openai", func(t *testing.T) {
		event := SSEEvent{
			Type: SSETypeData,
			Data: []byte(`{"candidates":[{"content":{"parts":[{"text":"Hello"}]},"index":0}]}`),
		}
		result := ConvertSSEEvent(event, "gemini", "cohere")
		if result == nil {
			t.Error("expected non-nil result for indirect conversion")
		}
	})

	t.Run("cohere_to_anthropic_via_openai", func(t *testing.T) {
		event := SSEEvent{
			Type: SSETypeData,
			Data: []byte(`{"event_type":"text-generation","text":"Hello"}`),
		}
		result := ConvertSSEEvent(event, "cohere", "anthropic")
		if result == nil {
			t.Error("expected non-nil result for indirect conversion")
		}
	})
}