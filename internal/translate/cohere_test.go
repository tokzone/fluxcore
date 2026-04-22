package translate

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/tokzone/fluxcore/message"
)

func TestCohereToMessageRequest(t *testing.T) {
	cohereReq := map[string]interface{}{
		"message": "Hello, how are you?",
		"chat_history": []interface{}{
			map[string]interface{}{
				"role":    "USER",
				"message": "Hi",
			},
			map[string]interface{}{
				"role":    "CHATBOT",
				"message": "Hello!",
			},
		},
		"preamble": "You are a helpful assistant",
		"max_tokens": 100.0,
		"temperature": 0.7,
		"p": 0.9,
		"stream": true,
	}

	reqBytes, _ := json.Marshal(cohereReq)

	req, err := CohereToMessageRequest(bytes.NewReader(reqBytes))
	if err != nil {
		t.Fatalf("CohereToMessageRequest failed: %v", err)
	}

	// Check preamble -> system message
	if len(req.Messages) == 0 || req.Messages[0].Role != "system" {
		t.Error("expected first message to be system (from preamble)")
	}

	// Check temperature
	if req.Temperature != 0.7 {
		t.Errorf("expected temperature 0.7, got %f", req.Temperature)
	}

	// Check top_p (p field)
	if req.TopP != 0.9 {
		t.Errorf("expected top_p 0.9, got %f", req.TopP)
	}

	// Check max tokens
	if req.MaxTokens != 100 {
		t.Errorf("expected max tokens 100, got %d", req.MaxTokens)
	}

	// Check role conversion (CHATBOT -> assistant)
	// Messages should be: system, user (chat_history), assistant (chat_history), user (message)
	if len(req.Messages) < 4 {
		t.Fatalf("expected at least 4 messages, got %d", len(req.Messages))
	}
	if req.Messages[2].Role != "assistant" {
		t.Errorf("expected third message role 'assistant', got '%s'", req.Messages[2].Role)
	}

	// Check last message is current user message
	if req.Messages[3].Role != "user" {
		t.Errorf("expected last message role 'user', got '%s'", req.Messages[3].Role)
	}
}

func TestMessageRequestToCohere(t *testing.T) {
	req := &message.MessageRequest{
		MaxTokens:   100,
		Temperature: 0.7,
		TopP:        0.9,
		Stream:      true,
		Messages: []message.Message{
			{Role: "system", Content: []message.Content{message.TextContent("You are helpful")}},
			{Role: "user", Content: []message.Content{message.TextContent("Hi")}},
			{Role: "assistant", Content: []message.Content{message.TextContent("Hello!")}},
			{Role: "user", Content: []message.Content{message.TextContent("How are you?")}},
		},
	}

	outBytes, err := MessageRequestToCohere(req)
	if err != nil {
		t.Fatalf("MessageRequestToCohere failed: %v", err)
	}

	var out map[string]interface{}
	json.Unmarshal(outBytes, &out)

	// Check preamble
	if out["preamble"] != "You are helpful" {
		t.Errorf("expected preamble 'You are helpful', got '%v'", out["preamble"])
	}

	// Check current message
	if out["message"] != "How are you?" {
		t.Errorf("expected message 'How are you?', got '%v'", out["message"])
	}

	// Check chat_history
	chatHistory, ok := out["chat_history"].([]interface{})
	if !ok || len(chatHistory) != 2 {
		t.Errorf("expected 2 chat_history items, got %d", len(chatHistory))
	}

	// Check role conversion in chat_history
	firstHist := chatHistory[0].(map[string]interface{})
	if firstHist["role"] != "USER" {
		t.Errorf("expected role 'USER', got '%v'", firstHist["role"])
	}

	secondHist := chatHistory[1].(map[string]interface{})
	if secondHist["role"] != "CHATBOT" {
		t.Errorf("expected role 'CHATBOT', got '%v'", secondHist["role"])
	}

	// Check temperature
	if out["temperature"] != 0.7 {
		t.Errorf("expected temperature 0.7, got %v", out["temperature"])
	}
}

func TestCohereResponseToMessageResponse(t *testing.T) {
	cohereResp := map[string]interface{}{
		"text": "Hello there! How can I help you?",
		"is_finished": true,
		"finish_reason": "complete",
		"meta": map[string]interface{}{
			"billed_units": map[string]interface{}{
				"input_tokens":  10.0,
				"output_tokens": 20.0,
			},
		},
	}

	respBytes, _ := json.Marshal(cohereResp)

	resp, err := CohereResponseToMessageResponse(respBytes)
	if err != nil {
		t.Fatalf("CohereResponseToMessageResponse failed: %v", err)
	}

	if len(resp.Choices) != 1 {
		t.Errorf("expected 1 choice, got %d", len(resp.Choices))
	}

	if resp.Choices[0].Message.Role != "assistant" {
		t.Errorf("expected role 'assistant', got '%s'", resp.Choices[0].Message.Role)
	}

	// Check text content
	text := ""
	for _, c := range resp.Choices[0].Message.Content {
		if c.Type == "text" {
			if t := c.AsText(); t != "" {
				text += t
			}
		}
	}
	if text != "Hello there! How can I help you?" {
		t.Errorf("expected text content, got '%s'", text)
	}

	if resp.Usage == nil {
		t.Fatal("expected usage metadata")
	}
	if resp.Usage.InputTokens != 10 {
		t.Errorf("expected input tokens 10, got %d", resp.Usage.InputTokens)
	}
	if resp.Usage.OutputTokens != 20 {
		t.Errorf("expected output tokens 20, got %d", resp.Usage.OutputTokens)
	}
}

func TestMessageResponseToCohere(t *testing.T) {
	resp := &message.MessageResponse{
		ID:    "test-id",
		Model: "command",
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

	outBytes, err := MessageResponseToCohere(resp)
	if err != nil {
		t.Fatalf("MessageResponseToCohere failed: %v", err)
	}

	var out map[string]interface{}
	json.Unmarshal(outBytes, &out)

	if out["text"] != "Hello!" {
		t.Errorf("expected text 'Hello!', got '%v'", out["text"])
	}

	if out["is_finished"] != true {
		t.Errorf("expected is_finished true, got %v", out["is_finished"])
	}

	if _, ok := out["token_count"]; !ok {
		t.Error("expected token_count in output")
	}
}

func TestCohereSSEToOpenAISSE(t *testing.T) {
	cohereData := `{"event_type":"text-generation","text":"Hello"}`

	converted := CohereSSEToOpenAISSE([]byte(cohereData))
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

func TestCohereSSEStreamEnd(t *testing.T) {
	cohereData := `{"event_type":"stream-end","finish_reason":"complete","is_finished":true,"token_count":{"input_tokens":10,"output_tokens":5}}`

	converted := CohereSSEToOpenAISSE([]byte(cohereData))
	if converted == nil {
		t.Fatal("expected converted output, got nil")
	}

	dataStr := string(converted[6:])
	dataStr = dataStr[:len(dataStr)-2]

	var chunk message.StreamChunk
	if err := json.Unmarshal([]byte(dataStr), &chunk); err != nil {
		t.Fatalf("failed to parse converted chunk: %v", err)
	}

	if chunk.Choices[0].FinishReason == nil || *chunk.Choices[0].FinishReason != "complete" {
		t.Errorf("expected finish_reason 'complete'")
	}

	if chunk.Usage == nil || chunk.Usage.InputTokens != 10 {
		t.Errorf("expected input tokens 10")
	}
}

func TestOpenAISSEToCohereSSE(t *testing.T) {
	chunk := &message.StreamChunk{
		ID:    "test",
		Model: "command",
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

	cohereOutput := OpenAISSEToCohereSSE(chunk)
	if cohereOutput == nil {
		t.Fatal("expected Cohere output, got nil")
	}

	if !bytes.HasPrefix(cohereOutput, []byte("data: ")) {
		t.Error("expected Cohere output to start with 'data: '")
	}

	dataStr := string(cohereOutput[6:])
	dataStr = dataStr[:len(dataStr)-2]

	var cohere map[string]interface{}
	if err := json.Unmarshal([]byte(dataStr), &cohere); err != nil {
		t.Fatalf("failed to parse Cohere output: %v", err)
	}

	if cohere["event_type"] != "text-generation" {
		t.Errorf("expected event_type 'text-generation', got '%v'", cohere["event_type"])
	}

	if cohere["text"] != "Hello" {
		t.Errorf("expected text 'Hello', got '%v'", cohere["text"])
	}
}

func TestCohereRoundTrip(t *testing.T) {
	// Test full round trip: Cohere -> IR -> Cohere
	original := map[string]interface{}{
		"message": "How are you?",
		"chat_history": []interface{}{
			map[string]interface{}{
				"role":    "USER",
				"message": "Hi",
			},
			map[string]interface{}{
				"role":    "CHATBOT",
				"message": "Hello!",
			},
		},
		"preamble": "Be helpful",
		"temperature": 0.5,
	}

	originalBytes, _ := json.Marshal(original)

	// Step 1: Cohere -> IR
	req, err := CohereToMessageRequest(bytes.NewReader(originalBytes))
	if err != nil {
		t.Fatalf("CohereToMessageRequest failed: %v", err)
	}

	// Step 2: IR -> Cohere
	roundTripBytes, err := MessageRequestToCohere(req)
	if err != nil {
		t.Fatalf("MessageRequestToCohere failed: %v", err)
	}

	var roundTrip map[string]interface{}
	json.Unmarshal(roundTripBytes, &roundTrip)

	// Verify preamble preserved
	if roundTrip["preamble"] != "Be helpful" {
		t.Errorf("expected preamble preserved, got '%v'", roundTrip["preamble"])
	}

	// Verify temperature preserved
	if roundTrip["temperature"] != 0.5 {
		t.Errorf("expected temperature 0.5, got %v", roundTrip["temperature"])
	}

	// Verify message preserved
	if roundTrip["message"] != "How are you?" {
		t.Errorf("expected message preserved, got '%v'", roundTrip["message"])
	}

	// Verify chat_history preserved
	chatHistory, ok := roundTrip["chat_history"].([]interface{})
	if !ok || len(chatHistory) != 2 {
		t.Errorf("expected 2 chat_history items, got %d", len(chatHistory))
	}
}