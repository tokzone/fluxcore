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

func TestParseSSEOpenAI(t *testing.T) {
	// Simulate OpenAI SSE stream
	sseData := `data: {"id":"test-1","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"content":[{"type":"text","data":"Hello"}]}}]}

data: {"id":"test-1","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"content":[{"type":"text","data":" world"}]}}]}

data: [DONE]

`

	reader := io.NopCloser(strings.NewReader(sseData))
	ctx := context.Background()
	start := time.Now()

	eventCh := ParseSSEStream(ctx, reader, "openai", start)

	events := make([]SSEParseResult, 0)
	for result := range eventCh {
		events = append(events, result)
	}

	// Should have 3 events: 2 data + 1 done + 1 final usage
	if len(events) < 3 {
		t.Errorf("expected at least 3 events, got %d", len(events))
	}

	// Check first data event
	if events[0].Event.Type != SSETypeData {
		t.Errorf("expected first event type 'data', got '%s'", events[0].Event.Type)
	}

	if events[0].Event.Chunk == nil {
		t.Error("expected first event to have parsed chunk")
	} else {
		if events[0].Event.Chunk.ID != "test-1" {
			t.Errorf("expected chunk ID 'test-1', got '%s'", events[0].Event.Chunk.ID)
		}
	}

	// Check done event
	var foundDone bool
	for _, e := range events {
		if e.Event.Type == SSETypeDone {
			foundDone = true
		}
	}
	if !foundDone {
		t.Error("expected to find 'done' event")
	}
}

func TestParseSSEWithUsage(t *testing.T) {
	sseData := `data: {"id":"test-1","choices":[{"index":0,"delta":{}}],"usage":{"input_tokens":10,"output_tokens":5}}

data: [DONE]

`

	reader := io.NopCloser(strings.NewReader(sseData))
	ctx := context.Background()
	start := time.Now()

	eventCh := ParseSSEStream(ctx, reader, "openai", start)

	var usageResult *SSEParseResult
	for result := range eventCh {
		if result.Usage != nil && result.Usage.IsAccurate {
			usageResult = &result
		}
	}

	if usageResult == nil {
		t.Error("expected to find usage data")
	} else {
		if usageResult.Usage.InputTokens != 10 {
			t.Errorf("expected input_tokens 10, got %d", usageResult.Usage.InputTokens)
		}
		if usageResult.Usage.OutputTokens != 5 {
			t.Errorf("expected output_tokens 5, got %d", usageResult.Usage.OutputTokens)
		}
	}
}

func TestConvertSSEEventNoConversion(t *testing.T) {
	event := SSEEvent{
		Type:   SSETypeData,
		Data:   []byte(`{"id":"test","choices":[{"index":0,"delta":{"content":[{"type":"text","data":"Hello"}]}}]}`),
		Format: "openai",
	}

	// Same format, no conversion
	output := ConvertSSEEvent(event, "openai", "openai")
	if output == nil {
		t.Error("expected output for same format")
	}
	if !bytes.HasPrefix(output, []byte("data: ")) {
		t.Error("expected output to start with 'data: '")
	}
}

func TestConvertSSEEventAnthropicToOpenAI(t *testing.T) {
	// Anthropic content_block_delta event
	anthropicData := `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hello"}}`

	event := SSEEvent{
		Type:   SSETypeData,
		Data:   []byte(anthropicData),
		Format: "anthropic",
	}

	output := ConvertSSEEvent(event, "anthropic", "openai")
	if output == nil {
		t.Error("expected conversion output")
	}

	// Should be OpenAI format
	if !bytes.HasPrefix(output, []byte("data: ")) {
		t.Error("expected OpenAI format to start with 'data: '")
	}
}

func TestConvertSSEEventOpenAIToAnthropic(t *testing.T) {
	chunk := &message.StreamChunk{
		ID:     "test-1",
		Model:  "gpt-4",
		Choices: []message.StreamChoice{
			{
				Index: 0,
				Delta: message.Message{
					Content: []message.Content{message.TextContent("Hello")},
				},
			},
		},
	}

	event := SSEEvent{
		Type:   SSETypeData,
		Data:   []byte(`{"id":"test-1","choices":[{"index":0,"delta":{"content":[{"type":"text","data":"Hello"}]}}]}`),
		Chunk:  chunk,
		Format: "openai",
	}

	output := ConvertSSEEvent(event, "openai", "anthropic")
	if output == nil {
		t.Error("expected conversion output")
	}

	// Should contain Anthropic event types
	if !bytes.Contains(output, []byte("event: content_block_delta")) {
		t.Error("expected Anthropic format to contain 'event: content_block_delta'")
	}
}

func TestFormatSSEOutput(t *testing.T) {
	tests := []struct {
		name     string
		event    SSEEvent
		expected string
	}{
		{
			name: "done event",
			event: SSEEvent{Type: "done"},
			expected: "data: [DONE]\n\n",
		},
		{
			name: "data event",
			event: SSEEvent{Type: SSETypeData, Data: []byte(`{"test":"value"}`)},
			expected: "data: {\"test\":\"value\"}\n\n",
		},
		{
			name: "event type",
			event: SSEEvent{Type: "event", Data: []byte("event: ping")},
			expected: "event: ping\n\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := FormatSSEOutput(tt.event, "openai")
			if string(output) != tt.expected {
				t.Errorf("expected '%s', got '%s'", tt.expected, string(output))
			}
		})
	}
}

func TestParseSSELine(t *testing.T) {
	start := time.Now()
	usageData := &message.Usage{}

	// Test data line
	result := parseSSELine("data: {\"id\":\"test\",\"choices\":[]}", "openai", start, usageData)
	if result.Event.Type != SSETypeData {
		t.Errorf("expected type 'data', got '%s'", result.Event.Type)
	}

	// Test [DONE] line
	result = parseSSELine("data: [DONE]", "openai", start, usageData)
	if result.Event.Type != "done" {
		t.Errorf("expected type 'done', got '%s'", result.Event.Type)
	}

	// Test event line
	result = parseSSELine("event: ping", "openai", start, usageData)
	if result.Event.Type != "event" {
		t.Errorf("expected type 'event', got '%s'", result.Event.Type)
	}

	// Test malformed JSON (should return error)
	result = parseSSELine("data: {invalid json}", "openai", start, usageData)
	if result.Error == nil {
		t.Error("expected error for malformed JSON")
	}
}

func TestParseSSEContextCancellation(t *testing.T) {
	sseData := `data: {"id":"test"}

`
	reader := io.NopCloser(strings.NewReader(sseData))
	ctx, cancel := context.WithCancel(context.Background())
	start := time.Now()

	// Cancel immediately
	cancel()

	eventCh := ParseSSEStream(ctx, reader, "openai", start)

	events := make([]SSEParseResult, 0)
	for result := range eventCh {
		events = append(events, result)
	}

	// Should handle cancellation gracefully
	// The stream should close without hanging
	if len(events) > 0 {
		// Last event should have context error or usage
		last := events[len(events)-1]
		if last.Error == nil && last.Usage == nil {
			t.Error("expected either error or usage on cancellation")
		}
	}
}