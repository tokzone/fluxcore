package message

import (
	"encoding/json"
	"testing"
)

func TestMessageMarshalJSON(t *testing.T) {
	t.Run("text_only_content", func(t *testing.T) {
		msg := Message{
			Role:    "assistant",
			Content: []Content{TextContent("Hello")},
		}
		data, err := json.Marshal(msg)
		if err != nil {
			t.Fatalf("MarshalJSON failed: %v", err)
		}
		var result map[string]interface{}
		if err := json.Unmarshal(data, &result); err != nil {
			t.Fatalf("unmarshal failed: %v", err)
		}
		content, ok := result["content"].(string)
		if !ok {
			t.Fatalf("expected content to be string, got %T: %v", result["content"], result["content"])
		}
		if content != "Hello" {
			t.Errorf("expected content 'Hello', got '%s'", content)
		}
		if result["role"] != "assistant" {
			t.Errorf("expected role 'assistant', got '%v'", result["role"])
		}
	})

	t.Run("empty_content", func(t *testing.T) {
		msg := Message{
			Role:    "assistant",
			Content: []Content{},
		}
		data, err := json.Marshal(msg)
		if err != nil {
			t.Fatalf("MarshalJSON failed: %v", err)
		}
		var result map[string]interface{}
		if err := json.Unmarshal(data, &result); err != nil {
			t.Fatalf("unmarshal failed: %v", err)
		}
		content, ok := result["content"].(string)
		if !ok {
			t.Fatalf("expected content to be string, got %T", result["content"])
		}
		if content != "" {
			t.Errorf("expected empty content, got '%s'", content)
		}
	})

	t.Run("multiple_text_content", func(t *testing.T) {
		msg := Message{
			Role: "assistant",
			Content: []Content{
				TextContent("Hello "),
				TextContent("World"),
			},
		}
		data, err := json.Marshal(msg)
		if err != nil {
			t.Fatalf("MarshalJSON failed: %v", err)
		}
		var result map[string]interface{}
		if err := json.Unmarshal(data, &result); err != nil {
			t.Fatalf("unmarshal failed: %v", err)
		}
		content, ok := result["content"].(string)
		if !ok {
			t.Fatalf("expected content to be string, got %T", result["content"])
		}
		if content != "Hello World" {
			t.Errorf("expected 'Hello World', got '%s'", content)
		}
	})
}

func TestMessageUnmarshalJSON_StringContent(t *testing.T) {
	data := []byte(`{"role":"user","content":"Hello, world!"}`)
	var msg Message
	if err := json.Unmarshal(data, &msg); err != nil {
		t.Fatalf("UnmarshalJSON failed: %v", err)
	}
	if msg.Role != "user" {
		t.Errorf("expected role 'user', got '%s'", msg.Role)
	}
	if len(msg.Content) != 1 {
		t.Fatalf("expected 1 content item, got %d", len(msg.Content))
	}
	if !msg.Content[0].IsText() {
		t.Error("expected text content")
	}
	if msg.Content[0].AsText() != "Hello, world!" {
		t.Errorf("expected 'Hello, world!', got '%s'", msg.Content[0].AsText())
	}
}

func TestMessageUnmarshalJSON_ArrayContent(t *testing.T) {
	data := []byte(`{"role":"user","content":[{"type":"text","text":"Describe"},{"type":"image_url","image_url":{"url":"http://x"}}]}`)
	var msg Message
	if err := json.Unmarshal(data, &msg); err != nil {
		t.Fatalf("UnmarshalJSON failed: %v", err)
	}
	if len(msg.Content) != 2 {
		t.Fatalf("expected 2 content items, got %d", len(msg.Content))
	}
	if !msg.Content[0].IsText() || msg.Content[0].AsText() != "Describe" {
		t.Errorf("content[0]: expected text 'Describe', got type=%s text=%s", msg.Content[0].Type, msg.Content[0].AsText())
	}
	if msg.Content[1].IsText() {
		t.Error("content[1] should not be text (image)")
	}
}

func TestMessageUnmarshalJSON_MissingContent(t *testing.T) {
	data := []byte(`{"role":"assistant"}`)
	var msg Message
	if err := json.Unmarshal(data, &msg); err != nil {
		t.Fatalf("UnmarshalJSON failed: %v", err)
	}
	if msg.Role != "assistant" {
		t.Errorf("expected role 'assistant', got '%s'", msg.Role)
	}
	if len(msg.Content) != 0 {
		t.Errorf("expected 0 content items, got %d", len(msg.Content))
	}
}

func TestMessageRoundTrip_ParseRemarshal(t *testing.T) {
	// Simulate Chat Completions JSON → parse → remarshal → verify content preserved
	chatJSON := `{"model":"gpt-4","messages":[{"role":"user","content":"Hello"},{"role":"assistant","content":"Hi there!"}],"stream":true}`
	req, err := ParseRequest([]byte(chatJSON))
	if err != nil {
		t.Fatalf("ParseRequest failed: %v", err)
	}
	if len(req.Messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(req.Messages))
	}
	if text := ExtractAllText(req.Messages[0].Content); text != "Hello" {
		t.Errorf("msg[0]: expected 'Hello', got '%s'", text)
	}
	if text := ExtractAllText(req.Messages[1].Content); text != "Hi there!" {
		t.Errorf("msg[1]: expected 'Hi there!', got '%s'", text)
	}

	// Remarshal and re-parse
	out, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}
	req2, err := ParseRequest(out)
	if err != nil {
		t.Fatalf("Re-parse failed: %v", err)
	}
	if text := ExtractAllText(req2.Messages[0].Content); text != "Hello" {
		t.Errorf("round-trip msg[0]: expected 'Hello', got '%s'", text)
	}
	if text := ExtractAllText(req2.Messages[1].Content); text != "Hi there!" {
		t.Errorf("round-trip msg[1]: expected 'Hi there!', got '%s'", text)
	}
	if !req2.Stream {
		t.Error("expected stream: true after round-trip")
	}
}

func TestMessageMarshalJSON_Streaming(t *testing.T) {
	t.Run("stream_chunk_text_only", func(t *testing.T) {
		chunk := StreamChunk{
			ID:     "chatcmpl-123",
			Object: "chat.completion.chunk",
			Model:  "gpt-4",
			Choices: []StreamChoice{{
				Index: 0,
				Delta: Message{
					Content: []Content{TextContent("Hello")},
				},
			}},
		}
		data, err := json.Marshal(chunk)
		if err != nil {
			t.Fatalf("Marshal failed: %v", err)
		}
		var result map[string]interface{}
		if err := json.Unmarshal(data, &result); err != nil {
			t.Fatalf("unmarshal failed: %v", err)
		}
		choices := result["choices"].([]interface{})
		delta := choices[0].(map[string]interface{})["delta"].(map[string]interface{})
		content, ok := delta["content"].(string)
		if !ok {
			t.Fatalf("expected delta.content to be string, got %T: %v", delta["content"], delta["content"])
		}
		if content != "Hello" {
			t.Errorf("expected 'Hello', got '%s'", content)
		}
	})
}
