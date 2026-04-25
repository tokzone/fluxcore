package translate

import (
	"bytes"
	"testing"

	"github.com/tokzone/fluxcore/message"
)

func TestCohereResponseToMessageResponseVariants(t *testing.T) {
	t.Run("basic_text", func(t *testing.T) {
		body := []byte(`{"text":"Hello world"}`)
		resp, err := CohereResponseToMessageResponse(body)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(resp.Choices) == 0 {
			t.Fatal("expected at least one choice")
		}
		if resp.Choices[0].Message.Role != "assistant" {
			t.Errorf("expected role assistant, got %s", resp.Choices[0].Message.Role)
		}
	})

	t.Run("with_finish_reason", func(t *testing.T) {
		body := []byte(`{"text":"Hello","finish_reason":"complete"}`)
		resp, err := CohereResponseToMessageResponse(body)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.Choices[0].FinishReason != "complete" {
			t.Errorf("expected finish_reason complete, got %s", resp.Choices[0].FinishReason)
		}
	})

	t.Run("with_meta_billed_units", func(t *testing.T) {
		body := []byte(`{
			"text": "Hello",
			"meta": {
				"billed_units": {
					"input_tokens": 10,
					"output_tokens": 5
				}
			}
		}`)
		resp, err := CohereResponseToMessageResponse(body)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.Usage == nil {
			t.Fatal("expected usage info")
		}
		if resp.Usage.InputTokens != 10 {
			t.Errorf("expected input_tokens 10, got %d", resp.Usage.InputTokens)
		}
		if resp.Usage.OutputTokens != 5 {
			t.Errorf("expected output_tokens 5, got %d", resp.Usage.OutputTokens)
		}
	})

	t.Run("with_token_count", func(t *testing.T) {
		body := []byte(`{
			"text": "Hello",
			"token_count": {
				"input_tokens": 20,
				"output_tokens": 8
			}
		}`)
		resp, err := CohereResponseToMessageResponse(body)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.Usage == nil {
			t.Fatal("expected usage info")
		}
		if resp.Usage.InputTokens != 20 {
			t.Errorf("expected input_tokens 20, got %d", resp.Usage.InputTokens)
		}
	})

	t.Run("with_both_meta_and_token_count", func(t *testing.T) {
		body := []byte(`{
			"text": "Hello",
			"meta": {
				"billed_units": {
					"input_tokens": 10,
					"output_tokens": 5
				}
			},
			"token_count": {
				"input_tokens": 20,
				"output_tokens": 8
			}
		}`)
		resp, err := CohereResponseToMessageResponse(body)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// token_count should override meta
		if resp.Usage.InputTokens != 20 {
			t.Errorf("expected token_count to override, got %d", resp.Usage.InputTokens)
		}
	})

	t.Run("invalid_json", func(t *testing.T) {
		body := []byte(`{invalid}`)
		_, err := CohereResponseToMessageResponse(body)
		if err == nil {
			t.Error("expected error for invalid JSON")
		}
	})

	t.Run("empty_response", func(t *testing.T) {
		body := []byte(`{}`)
		resp, err := CohereResponseToMessageResponse(body)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// Should return empty but valid response
		if len(resp.Choices) != 0 {
			t.Errorf("expected no choices, got %d", len(resp.Choices))
		}
	})

	t.Run("meta_without_billed_units", func(t *testing.T) {
		body := []byte(`{"text":"Hello","meta":{"other":"data"}}`)
		resp, err := CohereResponseToMessageResponse(body)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// Should not have usage
		if resp.Usage != nil {
			t.Error("expected no usage when billed_units missing")
		}
	})
}

func TestMessageResponseToCohereVariants(t *testing.T) {
	t.Run("basic_response", func(t *testing.T) {
		resp := &message.MessageResponse{
			Choices: []message.Choice{
				{
					Message: message.Message{
						Role:    "assistant",
						Content: []message.Content{message.TextContent("Hello")},
					},
					FinishReason: "complete",
				},
			},
		}
		data, err := MessageResponseToCohere(resp)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(data) == 0 {
			t.Error("expected non-empty output")
		}
	})

	t.Run("with_usage", func(t *testing.T) {
		resp := &message.MessageResponse{
			Choices: []message.Choice{
				{
					Message:      message.Message{Role: "assistant", Content: []message.Content{message.TextContent("Hello")}},
					FinishReason: "complete",
				},
			},
			Usage: &message.Usage{
				InputTokens:  10,
				OutputTokens: 5,
			},
		}
		data, err := MessageResponseToCohere(resp)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// Should contain token_count
		if len(data) == 0 {
			t.Error("expected non-empty output")
		}
	})

	t.Run("empty_choices", func(t *testing.T) {
		resp := &message.MessageResponse{
			Choices: []message.Choice{},
		}
		data, err := MessageResponseToCohere(resp)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// Should still produce valid output
		if len(data) == 0 {
			t.Error("expected non-empty output")
		}
	})

	t.Run("multiple_content_parts", func(t *testing.T) {
		resp := &message.MessageResponse{
			Choices: []message.Choice{
				{
					Message: message.Message{
						Content: []message.Content{
							message.TextContent("Hello"),
							message.TextContent(" world"),
						},
					},
				},
			},
		}
		data, err := MessageResponseToCohere(resp)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// Should concatenate text
		if len(data) == 0 {
			t.Error("expected non-empty output")
		}
	})
}

func TestCohereToMessageRequestVariants(t *testing.T) {
	t.Run("with_chat_history", func(t *testing.T) {
		body := []byte(`{
			"message": "Hello",
			"chat_history": [
				{"role": "USER", "message": "Hi"},
				{"role": "CHATBOT", "message": "Hey there"}
			]
		}`)
		req, err := CohereToMessageRequest(bytes.NewReader(body))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(req.Messages) == 0 {
			t.Error("expected messages from chat_history")
		}
	})

	t.Run("with_preamble", func(t *testing.T) {
		body := []byte(`{
			"message": "Hello",
			"preamble": "You are a helpful assistant"
		}`)
		req, err := CohereToMessageRequest(bytes.NewReader(body))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// Should have system message from preamble
		if len(req.Messages) == 0 {
			t.Error("expected messages including preamble")
		}
	})

	t.Run("empty_request", func(t *testing.T) {
		body := []byte(`{}`)
		req, err := CohereToMessageRequest(bytes.NewReader(body))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if req == nil {
			t.Error("expected non-nil request")
		}
	})
}
