package translate

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/tokzone/fluxcore/message"
)

func TestGeminiToMessageRequest(t *testing.T) {
	geminiReq := map[string]interface{}{
		"contents": []interface{}{
			map[string]interface{}{
				"role": "user",
				"parts": []interface{}{
					map[string]interface{}{"text": "Hello"},
				},
			},
			map[string]interface{}{
				"role": "model",
				"parts": []interface{}{
					map[string]interface{}{"text": "Hi there!"},
				},
			},
		},
		"generationConfig": map[string]interface{}{
			"maxOutputTokens": 100.0,
			"temperature":     0.7,
			"topP":            0.9,
		},
		"systemInstruction": map[string]interface{}{
			"parts": []interface{}{
				map[string]interface{}{"text": "You are helpful"},
			},
		},
	}

	reqBytes, _ := json.Marshal(geminiReq)

	req, err := GeminiToMessageRequest(bytes.NewReader(reqBytes))
	if err != nil {
		t.Fatalf("GeminiToMessageRequest failed: %v", err)
	}

	// Check system message extracted
	if len(req.Messages) == 0 || req.Messages[0].Role != "system" {
		t.Error("expected first message to be system")
	}

	// Check temperature
	if req.Temperature != 0.7 {
		t.Errorf("expected temperature 0.7, got %f", req.Temperature)
	}

	// Check max tokens
	if req.MaxTokens != 100 {
		t.Errorf("expected max tokens 100, got %d", req.MaxTokens)
	}

	// Check role conversion (model -> assistant)
	if len(req.Messages) < 3 {
		t.Fatalf("expected at least 3 messages, got %d", len(req.Messages))
	}
	if req.Messages[2].Role != "assistant" {
		t.Errorf("expected third message role 'assistant', got '%s'", req.Messages[2].Role)
	}
}

func TestMessageRequestToGemini(t *testing.T) {
	req := &message.MessageRequest{
		Model:       "gemini-pro",
		MaxTokens:   100,
		Temperature: 0.7,
		TopP:        0.9,
		Messages: []message.Message{
			{Role: "system", Content: []message.Content{message.TextContent("You are helpful")}},
			{Role: "user", Content: []message.Content{message.TextContent("Hello")}},
			{Role: "assistant", Content: []message.Content{message.TextContent("Hi there!")}},
		},
	}

	outBytes, err := MessageRequestToGemini(req)
	if err != nil {
		t.Fatalf("MessageRequestToGemini failed: %v", err)
	}

	var out map[string]interface{}
	json.Unmarshal(outBytes, &out)

	// Check system instruction
	if _, ok := out["systemInstruction"]; !ok {
		t.Error("expected systemInstruction in output")
	}

	// Check generation config
	gc, ok := out["generationConfig"].(map[string]interface{})
	if !ok {
		t.Error("expected generationConfig in output")
	}
	if gc["maxOutputTokens"] != 100.0 {
		t.Errorf("expected maxOutputTokens 100, got %v", gc["maxOutputTokens"])
	}

	// Check contents
	contents, ok := out["contents"].([]interface{})
	if !ok || len(contents) != 2 {
		t.Errorf("expected 2 content items (excluding system), got %d", len(contents))
	}

	// Check role conversion (assistant -> model)
	secondContent := contents[1].(map[string]interface{})
	if secondContent["role"] != "model" {
		t.Errorf("expected role 'model' for assistant, got '%v'", secondContent["role"])
	}
}

func TestGeminiResponseToMessageResponse(t *testing.T) {
	geminiResp := map[string]interface{}{
		"candidates": []interface{}{
			map[string]interface{}{
				"index": 0,
				"content": map[string]interface{}{
					"role":  "model",
					"parts": []interface{}{
						map[string]interface{}{"text": "Hello there!"},
					},
				},
				"finishReason": "STOP",
			},
		},
		"usageMetadata": map[string]interface{}{
			"promptTokenCount":     10.0,
			"candidatesTokenCount": 5.0,
			"totalTokenCount":      15.0,
		},
	}

	respBytes, _ := json.Marshal(geminiResp)

	resp, err := GeminiResponseToMessageResponse(respBytes)
	if err != nil {
		t.Fatalf("GeminiResponseToMessageResponse failed: %v", err)
	}

	if len(resp.Choices) != 1 {
		t.Errorf("expected 1 choice, got %d", len(resp.Choices))
	}

	if resp.Usage == nil {
		t.Fatal("expected usage metadata")
	}
	if resp.Usage.InputTokens != 10 {
		t.Errorf("expected input tokens 10, got %d", resp.Usage.InputTokens)
	}
	if resp.Usage.OutputTokens != 5 {
		t.Errorf("expected output tokens 5, got %d", resp.Usage.OutputTokens)
	}
}

func TestMessageResponseToGemini(t *testing.T) {
	resp := &message.MessageResponse{
		ID:    "test-id",
		Model: "gemini-pro",
		Choices: []message.Choice{
			{
				Index: 0,
				Message: message.Message{
					Role:    "assistant",
					Content: []message.Content{message.TextContent("Hello!")},
				},
				FinishReason: "stop",
			},
		},
		Usage: &message.Usage{
			InputTokens:  10,
			OutputTokens: 5,
		},
	}

	outBytes, err := MessageResponseToGemini(resp)
	if err != nil {
		t.Fatalf("MessageResponseToGemini failed: %v", err)
	}

	var out map[string]interface{}
	json.Unmarshal(outBytes, &out)

	if _, ok := out["candidates"]; !ok {
		t.Error("expected candidates in output")
	}

	if _, ok := out["usageMetadata"]; !ok {
		t.Error("expected usageMetadata in output")
	}
}

func TestGeminiSSEToOpenAISSE(t *testing.T) {
	geminiData := `{"candidates":[{"index":0,"content":{"role":"model","parts":[{"text":"Hello"}]}}]}`

	converted := GeminiSSEToOpenAISSE([]byte(geminiData))
	if converted == nil {
		t.Fatal("expected converted output, got nil")
	}

	if !bytes.HasPrefix(converted, []byte("data: ")) {
		t.Error("expected output to start with 'data: '")
	}

	// Parse the converted output
	dataStr := string(converted[6:]) // skip "data: "
	dataStr = dataStr[:len(dataStr)-2] // skip "\n\n"

	var chunk message.StreamChunk
	if err := json.Unmarshal([]byte(dataStr), &chunk); err != nil {
		t.Fatalf("failed to parse converted chunk: %v", err)
	}

	if len(chunk.Choices) != 1 {
		t.Errorf("expected 1 choice, got %d", len(chunk.Choices))
	}

	if chunk.Choices[0].Delta.Role != "assistant" {
		t.Errorf("expected role 'assistant', got '%s'", chunk.Choices[0].Delta.Role)
	}
}

func TestOpenAISSEToGeminiSSE(t *testing.T) {
	chunk := &message.StreamChunk{
		ID:    "test",
		Model: "gemini-pro",
		Choices: []message.StreamChoice{
			{
				Index: 0,
				Delta: message.Message{
					Role:    "assistant",
					Content: []message.Content{message.TextContent("Hello")},
				},
			},
		},
	}

	geminiOutput := OpenAISSEToGeminiSSE(chunk)
	if geminiOutput == nil {
		t.Fatal("expected Gemini output, got nil")
	}

	if !bytes.HasPrefix(geminiOutput, []byte("data: ")) {
		t.Error("expected Gemini output to start with 'data: '")
	}

	// Parse the Gemini output
	dataStr := string(geminiOutput[6:])
	dataStr = dataStr[:len(dataStr)-2]

	var gemini map[string]interface{}
	if err := json.Unmarshal([]byte(dataStr), &gemini); err != nil {
		t.Fatalf("failed to parse Gemini output: %v", err)
	}

	candidates, ok := gemini["candidates"].([]interface{})
	if !ok || len(candidates) != 1 {
		t.Errorf("expected 1 candidate, got %d", len(candidates))
	}

	candidate := candidates[0].(map[string]interface{})
	content := candidate["content"].(map[string]interface{})
	if content["role"] != "model" {
		t.Errorf("expected role 'model', got '%v'", content["role"])
	}
}

func TestGeminiRoundTrip(t *testing.T) {
	// Test full round trip: Gemini -> IR -> Gemini
	original := map[string]interface{}{
		"contents": []interface{}{
			map[string]interface{}{
				"role": "user",
				"parts": []interface{}{
					map[string]interface{}{"text": "Hello"},
				},
			},
		},
		"generationConfig": map[string]interface{}{
			"temperature": 0.5,
		},
	}

	originalBytes, _ := json.Marshal(original)

	// Step 1: Gemini -> IR
	req, err := GeminiToMessageRequest(bytes.NewReader(originalBytes))
	if err != nil {
		t.Fatalf("GeminiToMessageRequest failed: %v", err)
	}

	// Step 2: IR -> Gemini
	roundTripBytes, err := MessageRequestToGemini(req)
	if err != nil {
		t.Fatalf("MessageRequestToGemini failed: %v", err)
	}

	var roundTrip map[string]interface{}
	json.Unmarshal(roundTripBytes, &roundTrip)

	// Verify contents preserved
	contents, ok := roundTrip["contents"].([]interface{})
	if !ok || len(contents) != 1 {
		t.Errorf("expected 1 content, got %d", len(contents))
	}

	// Verify generation config preserved
	gc, ok := roundTrip["generationConfig"].(map[string]interface{})
	if !ok {
		t.Error("expected generationConfig")
	}
	if gc["temperature"] != 0.5 {
		t.Errorf("expected temperature 0.5, got %v", gc["temperature"])
	}
}