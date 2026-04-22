package translate

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/tokzone/fluxcore/message"
)

func TestConvertSSEEvent(t *testing.T) {
	t.Run("same_format_passthrough", func(t *testing.T) {
		event := SSEEvent{
			Type: SSETypeData,
			Data: []byte(`{"choices":[{"delta":{"content":"Hello"}}]}`),
		}
		result := ConvertSSEEvent(event, "openai", "openai")
		if result == nil {
			t.Error("expected non-nil result for same format")
		}
		if !bytes.Contains(result, []byte("data:")) {
			t.Error("expected data: prefix in result")
		}
	})

	t.Run("anthropic_to_openai", func(t *testing.T) {
		event := SSEEvent{
			Type: SSETypeData,
			Data: []byte(`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hello"}}`),
		}
		result := ConvertSSEEvent(event, "anthropic", "openai")
		if result == nil {
			t.Error("expected non-nil result")
		}
	})

	t.Run("gemini_to_openai", func(t *testing.T) {
		event := SSEEvent{
			Type: SSETypeData,
			Data: []byte(`{"candidates":[{"content":{"parts":[{"text":"Hello"}]},"index":0}]}`),
		}
		result := ConvertSSEEvent(event, "gemini", "openai")
		if result == nil {
			t.Error("expected non-nil result")
		}
	})

	t.Run("cohere_to_openai", func(t *testing.T) {
		event := SSEEvent{
			Type: SSETypeData,
			Data: []byte(`{"event_type":"text-generation","is_finished":false,"text":"Hello"}`),
		}
		result := ConvertSSEEvent(event, "cohere", "openai")
		if result == nil {
			t.Error("expected non-nil result")
		}
	})

	t.Run("openai_to_anthropic", func(t *testing.T) {
		chunk := &message.StreamChunk{
			Choices: []message.StreamChoice{
				{Delta: message.Message{Content: []message.Content{message.TextContent("Hello")}}},
			},
		}
		event := SSEEvent{
			Type:  SSETypeData,
			Chunk: chunk,
		}
		result := ConvertSSEEvent(event, "openai", "anthropic")
		if result == nil {
			t.Error("expected non-nil result")
		}
	})

	t.Run("openai_to_gemini", func(t *testing.T) {
		chunk := &message.StreamChunk{
			Choices: []message.StreamChoice{
				{Delta: message.Message{Content: []message.Content{message.TextContent("Hello")}}},
			},
		}
		event := SSEEvent{
			Type:  SSETypeData,
			Chunk: chunk,
		}
		result := ConvertSSEEvent(event, "openai", "gemini")
		if result == nil {
			t.Error("expected non-nil result")
		}
	})

	t.Run("openai_to_cohere", func(t *testing.T) {
		chunk := &message.StreamChunk{
			Choices: []message.StreamChoice{
				{Delta: message.Message{Content: []message.Content{message.TextContent("Hello")}}},
			},
		}
		event := SSEEvent{
			Type:  SSETypeData,
			Chunk: chunk,
		}
		result := ConvertSSEEvent(event, "openai", "cohere")
		if result == nil {
			t.Error("expected non-nil result")
		}
	})

	t.Run("non_data_event_returns_nil", func(t *testing.T) {
		event := SSEEvent{
			Type: SSETypeEvent,
			Data: []byte(`event: ping`),
		}
		result := ConvertSSEEvent(event, "openai", "anthropic")
		if result != nil {
			t.Error("expected nil for non-data event")
		}
	})

	t.Run("unknown_format_returns_nil", func(t *testing.T) {
		event := SSEEvent{
			Type: SSETypeData,
			Data: []byte(`{}`),
		}
		result := ConvertSSEEvent(event, "unknown", "openai")
		if result != nil {
			t.Error("expected nil for unknown format")
		}
	})
}

func TestConvertViaOpenAI(t *testing.T) {
	t.Run("anthropic_via_openai_to_gemini", func(t *testing.T) {
		data := []byte(`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hello"}}`)
		result := convertViaOpenAI(data, AnthropicSSEToOpenAISSE, OpenAISSEToGeminiSSE)
		if result == nil {
			t.Error("expected non-nil result")
		}
	})

	t.Run("nil_input", func(t *testing.T) {
		toOpenAI := func([]byte) []byte { return nil }
		fromOpenAI := func(*message.StreamChunk) []byte { return nil }
		result := convertViaOpenAI([]byte{}, toOpenAI, fromOpenAI)
		if result != nil {
			t.Error("expected nil for nil input")
		}
	})

	t.Run("invalid_json", func(t *testing.T) {
		data := []byte(`{"type":"content_block_delta","delta":{"text":"Hello"}}`)
		result := convertViaOpenAI(data, AnthropicSSEToOpenAISSE, OpenAISSEToGeminiSSE)
		// Should handle gracefully
		_ = result
	})
}

func TestConvertToAnthropic(t *testing.T) {
	t.Run("gemini_to_anthropic", func(t *testing.T) {
		data := []byte(`{"candidates":[{"content":{"parts":[{"text":"Hello"}]},"index":0}]}`)
		result := convertToAnthropic(data, GeminiSSEToOpenAISSE)
		if result == nil {
			t.Error("expected non-nil result")
		}
	})

	t.Run("nil_input", func(t *testing.T) {
		toOpenAI := func([]byte) []byte { return nil }
		result := convertToAnthropic([]byte{}, toOpenAI)
		if result != nil {
			t.Error("expected nil for nil input")
		}
	})
}

func TestJoinAnthropicEvents(t *testing.T) {
	t.Run("multiple_events", func(t *testing.T) {
		events := []string{
			"event: content_block_delta\ndata: {\"type\":\"text_delta\",\"text\":\"Hello\"}\n\n",
			"event: content_block_delta\ndata: {\"type\":\"text_delta\",\"text\":\" world\"}\n\n",
		}
		result := joinAnthropicEvents(events)
		if len(result) == 0 {
			t.Error("expected non-empty result")
		}
	})

	t.Run("empty_events", func(t *testing.T) {
		result := joinAnthropicEvents([]string{})
		if result != nil {
			t.Error("expected nil for empty events")
		}
	})

	t.Run("single_event", func(t *testing.T) {
		events := []string{"data: test\n\n"}
		result := joinAnthropicEvents(events)
		if string(result) != "data: test\n\n" {
			t.Errorf("unexpected result: %s", result)
		}
	})
}

func TestFormatSSEOutputExtended(t *testing.T) {
	t.Run("done_event", func(t *testing.T) {
		event := SSEEvent{Type: SSETypeDone}
		result := FormatSSEOutput(event, "openai")
		if string(result) != "data: [DONE]\n\n" {
			t.Errorf("unexpected done output: %s", result)
		}
	})

	t.Run("event_type", func(t *testing.T) {
		event := SSEEvent{
			Type: SSETypeEvent,
			Data: []byte("event: ping"),
		}
		result := FormatSSEOutput(event, "openai")
		if string(result) != "event: ping\n\n" {
			t.Errorf("unexpected event output: %s", result)
		}
	})

	t.Run("data_event", func(t *testing.T) {
		event := SSEEvent{
			Type: SSETypeData,
			Data: []byte(`{"test":true}`),
		}
		result := FormatSSEOutput(event, "openai")
		if string(result) != "data: {\"test\":true}\n\n" {
			t.Errorf("unexpected data output: %s", result)
		}
	})

	t.Run("unknown_type", func(t *testing.T) {
		event := SSEEvent{Type: "unknown"}
		result := FormatSSEOutput(event, "openai")
		if result != nil {
			t.Error("expected nil for unknown type")
		}
	})
}

func TestParseSSELineExtended(t *testing.T) {
	startTime := time.Now()
	usageData := &message.Usage{}

	t.Run("data_line", func(t *testing.T) {
		line := "data: {\"choices\":[{\"delta\":{\"content\":[{\"type\":\"text\",\"data\":\"Hello\"}]}}]}"
		result := parseSSELine(line, "openai", startTime, usageData)
		if result.Event.Type != SSETypeData {
			t.Errorf("expected SSETypeData, got %s", result.Event.Type)
		}
	})

	t.Run("done_line", func(t *testing.T) {
		line := "data: [DONE]"
		result := parseSSELine(line, "openai", startTime, usageData)
		if result.Event.Type != SSETypeDone {
			t.Errorf("expected SSETypeDone, got %s", result.Event.Type)
		}
	})

	t.Run("event_line", func(t *testing.T) {
		line := "event: content_block_start"
		result := parseSSELine(line, "openai", startTime, usageData)
		if result.Event.Type != SSETypeEvent {
			t.Errorf("expected SSETypeEvent, got %s", result.Event.Type)
		}
	})

	t.Run("empty_line", func(t *testing.T) {
		result := parseSSELine("", "openai", startTime, usageData)
		if result.Event.Type != "" {
			t.Errorf("expected empty type, got %s", result.Event.Type)
		}
	})

	t.Run("invalid_json", func(t *testing.T) {
		line := "data: {invalid json}"
		result := parseSSELine(line, "openai", startTime, usageData)
		if result.Error == nil {
			t.Error("expected error for invalid JSON")
		}
	})

	t.Run("anthropic_format", func(t *testing.T) {
		line := "data: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"Hello\"}}"
		result := parseSSELine(line, "anthropic", startTime, usageData)
		if result.Event.Type != SSETypeData {
			t.Errorf("expected SSETypeData, got %s", result.Event.Type)
		}
	})

	t.Run("gemini_format", func(t *testing.T) {
		line := "data: {\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"Hello\"}]},\"index\":0}]}"
		result := parseSSELine(line, "gemini", startTime, usageData)
		if result.Event.Type != SSETypeData {
			t.Errorf("expected SSETypeData, got %s", result.Event.Type)
		}
	})

	t.Run("cohere_format", func(t *testing.T) {
		line := "data: {\"event_type\":\"text-generation\",\"is_finished\":false,\"text\":\"Hello\"}"
		result := parseSSELine(line, "cohere", startTime, usageData)
		if result.Event.Type != SSETypeData {
			t.Errorf("expected SSETypeData, got %s", result.Event.Type)
		}
	})
}

func TestParseSSEStreamContextExtended(t *testing.T) {
	t.Run("context_cancellation", func(t *testing.T) {
		reader := io.NopCloser(strings.NewReader("data: test\n\n"))
		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		ch := ParseSSEStream(ctx, reader, "openai", time.Now())

		// Should receive result with context error
		result := <-ch
		if result.Error == nil {
			t.Error("expected context error")
		}
	})
}

func TestGetChunkParser(t *testing.T) {
	t.Run("gemini_format", func(t *testing.T) {
		parser := getChunkParser("gemini")
		if parser == nil {
			t.Error("expected parser for gemini")
		}
	})

	t.Run("cohere_format", func(t *testing.T) {
		parser := getChunkParser("cohere")
		if parser == nil {
			t.Error("expected parser for cohere")
		}
	})

	t.Run("openai_format", func(t *testing.T) {
		parser := getChunkParser("openai")
		if parser != nil {
			t.Error("expected nil for openai (default format)")
		}
	})

	t.Run("unknown_format", func(t *testing.T) {
		parser := getChunkParser("unknown")
		if parser != nil {
			t.Error("expected nil for unknown format")
		}
	})
}

func TestRegisterChunkParser(t *testing.T) {
	// Register a custom parser
	customParser := func(data []byte) (*message.StreamChunk, error) {
		return &message.StreamChunk{}, nil
	}
	RegisterChunkParser("custom", customParser)

	parser := getChunkParser("custom")
	if parser == nil {
		t.Error("expected registered parser")
	}
}