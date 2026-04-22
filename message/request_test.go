package message

import (
	"encoding/json"
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

func TestUsageTotalTokens(t *testing.T) {
	usage := &Usage{
		InputTokens:  100,
		OutputTokens: 50,
	}

	if usage.TotalTokens() != 150 {
		t.Errorf("expected 150 total tokens, got %d", usage.TotalTokens())
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

func TestImageContent(t *testing.T) {
	c := ImageContent("https://example.com/image.png", "image/png", "")
	if c.Type != "image" {
		t.Errorf("expected type image, got %s", c.Type)
	}

	media := c.AsMedia()
	if media == nil {
		t.Fatal("expected media data, got nil")
	}
	if media.URL != "https://example.com/image.png" {
		t.Errorf("expected URL, got %s", media.URL)
	}
	if media.MediaType != "image/png" {
		t.Errorf("expected mediaType image/png, got %s", media.MediaType)
	}
}

func TestImageContentWithBase64(t *testing.T) {
	c := ImageContent("", "image/jpeg", "base64data")
	media := c.AsMedia()
	if media == nil {
		t.Fatal("expected media data, got nil")
	}
	if media.URL != "" {
		t.Errorf("expected empty URL, got %s", media.URL)
	}
	if media.Base64 != "base64data" {
		t.Errorf("expected base64data, got %s", media.Base64)
	}
}

func TestImageContentJSON(t *testing.T) {
	c := ImageContent("https://example.com/img.png", "image/png", "")
	data, err := json.Marshal(c)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var parsed Content
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if parsed.Type != "image" {
		t.Errorf("expected type image, got %s", parsed.Type)
	}

	media := parsed.AsMedia()
	if media == nil {
		t.Fatal("expected media after unmarshal")
	}
	if media.URL != "https://example.com/img.png" {
		t.Errorf("expected URL after unmarshal, got %s", media.URL)
	}
}

func TestAudioContent(t *testing.T) {
	c := AudioContent("https://example.com/audio.mp3", "audio/mp3", "")
	if c.Type != "audio" {
		t.Errorf("expected type audio, got %s", c.Type)
	}

	media := c.AsMedia()
	if media == nil {
		t.Fatal("expected media data, got nil")
	}
	if media.URL != "https://example.com/audio.mp3" {
		t.Errorf("expected URL, got %s", media.URL)
	}
	if media.MediaType != "audio/mp3" {
		t.Errorf("expected mediaType audio/mp3, got %s", media.MediaType)
	}
}

func TestAudioContentWithBase64(t *testing.T) {
	c := AudioContent("", "audio/wav", "base64audio")
	media := c.AsMedia()
	if media == nil {
		t.Fatal("expected media data, got nil")
	}
	if media.URL != "" {
		t.Errorf("expected empty URL, got %s", media.URL)
	}
	if media.Base64 != "base64audio" {
		t.Errorf("expected base64audio, got %s", media.Base64)
	}
}

func TestAudioContentJSON(t *testing.T) {
	c := AudioContent("https://example.com/audio.mp3", "audio/mp3", "")
	data, err := json.Marshal(c)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var parsed Content
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if parsed.Type != "audio" {
		t.Errorf("expected type audio, got %s", parsed.Type)
	}

	media := parsed.AsMedia()
	if media == nil {
		t.Fatal("expected media after unmarshal")
	}
	if media.URL != "https://example.com/audio.mp3" {
		t.Errorf("expected URL after unmarshal, got %s", media.URL)
	}
}

func TestContentTypeMismatch(t *testing.T) {
	// Text content should not return media
	text := TextContent("hello")
	if text.AsMedia() != nil {
		t.Error("text content should not return media")
	}

	// Image content should not return text
	img := ImageContent("url", "image/png", "")
	if img.AsText() != "" {
		t.Error("image content should not return text")
	}
	// Image content should return media via AsMedia
	if img.AsMedia() == nil {
		t.Error("image content should return media via AsMedia")
	}

	// Audio content should not return text
	aud := AudioContent("url", "audio/mp3", "")
	if aud.AsText() != "" {
		t.Error("audio content should not return text")
	}
	// Audio content should return media via AsMedia
	if aud.AsMedia() == nil {
		t.Error("audio content should return media via AsMedia")
	}
}

func TestMultimodalMessage(t *testing.T) {
	msg := Message{
		Role: "user",
		Content: []Content{
			TextContent("What's in this image?"),
			ImageContent("https://example.com/photo.jpg", "image/jpeg", ""),
		},
	}

	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal multimodal message failed: %v", err)
	}

	var parsed Message
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if len(parsed.Content) != 2 {
		t.Errorf("expected 2 content items, got %d", len(parsed.Content))
	}
	if parsed.Content[0].Type != "text" {
		t.Errorf("expected first content type text, got %s", parsed.Content[0].Type)
	}
	if parsed.Content[1].Type != "image" {
		t.Errorf("expected second content type image, got %s", parsed.Content[1].Type)
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

func TestWithModel(t *testing.T) {
	req := &MessageRequest{
		Model:    "gpt-4",
		Messages: []Message{{Role: "user", Content: []Content{TextContent("hello")}}},
	}

	newReq := req.WithModel("gpt-3.5-turbo")
	if newReq.Model != "gpt-3.5-turbo" {
		t.Errorf("expected model gpt-3.5-turbo, got %s", newReq.Model)
	}
	// Original should be unchanged
	if req.Model != "gpt-4" {
		t.Error("original request should be unchanged")
	}
}

func TestWithMaxTokens(t *testing.T) {
	req := &MessageRequest{
		Model:    "gpt-4",
		MaxTokens: 100,
	}

	newReq := req.WithMaxTokens(200)
	if newReq.MaxTokens != 200 {
		t.Errorf("expected MaxTokens 200, got %d", newReq.MaxTokens)
	}
	// Original should be unchanged
	if req.MaxTokens != 100 {
		t.Error("original request should be unchanged")
	}
}

func TestClone(t *testing.T) {
	req := &MessageRequest{
		Model: "gpt-4",
		Messages: []Message{
			{Role: "user", Content: []Content{TextContent("hello")}},
			{Role: "assistant", Content: []Content{TextContent("world")}},
		},
		MaxTokens: 100,
	}

	cloned := req.Clone()
	if cloned == req {
		t.Error("Clone should return a different pointer")
	}
	if cloned.Model != req.Model {
		t.Errorf("expected model %s, got %s", req.Model, cloned.Model)
	}
	if len(cloned.Messages) != len(req.Messages) {
		t.Errorf("expected %d messages, got %d", len(req.Messages), len(cloned.Messages))
	}

	// Modify clone, original should be unchanged
	cloned.Messages[0] = Message{Role: "system", Content: []Content{TextContent("modified")}}
	if req.Messages[0].Role == "system" {
		t.Error("modifying clone should not affect original")
	}
}

func TestCloneEmptyMessages(t *testing.T) {
	req := &MessageRequest{
		Model:    "gpt-4",
		Messages: nil,
	}

	cloned := req.Clone()
	if cloned.Messages != nil && len(cloned.Messages) != 0 {
		t.Errorf("expected empty messages, got %d", len(cloned.Messages))
	}
}