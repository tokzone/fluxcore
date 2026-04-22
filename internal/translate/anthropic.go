package translate

import (
	"encoding/json"
	"io"
	"strings"

	"github.com/tokzone/fluxcore/message"
)

// AnthropicRequest represents Anthropic API request structure
type AnthropicRequest struct {
	Model       string          `json:"model"`
	MaxTokens   int             `json:"max_tokens"`
	Temperature float64         `json:"temperature,omitempty"`
	TopP        float64         `json:"top_p,omitempty"`
	Stream      bool            `json:"stream,omitempty"`
	System      string          `json:"system,omitempty"`
	Messages    []AnthropicMsg  `json:"messages"`
}

// AnthropicMsg represents a message in Anthropic format
type AnthropicMsg struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"` // Can be string or []AnthropicContentBlock
}

// AnthropicContentBlock represents a content block
type AnthropicContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

// AnthropicResponse represents Anthropic API response structure
type AnthropicResponse struct {
	ID         string                  `json:"id"`
	Model      string                  `json:"model"`
	Content    []AnthropicContentBlock `json:"content"`
	StopReason string                  `json:"stop_reason,omitempty"`
	Usage      *AnthropicUsage         `json:"usage,omitempty"`
}

// AnthropicUsage represents token usage in Anthropic format
type AnthropicUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// AnthropicToMessageRequest converts Anthropic format to MessageRequest
func AnthropicToMessageRequest(body io.Reader) (*message.MessageRequest, error) {
	var ar AnthropicRequest
	if err := json.NewDecoder(body).Decode(&ar); err != nil {
		return nil, err
	}

	req := &message.MessageRequest{
		Model:       ar.Model,
		MaxTokens:   ar.MaxTokens,
		Temperature: ar.Temperature,
		TopP:        ar.TopP,
		Stream:      ar.Stream,
	}

	// System message from system field
	if ar.System != "" {
		req.Messages = append(req.Messages, message.Message{
			Role:    "system",
			Content: []message.Content{message.TextContent(ar.System)},
		})
	}

	// Messages
	for _, m := range ar.Messages {
		var contents []message.Content

		// Content can be string or array
		// Try to parse as string first
		var contentStr string
		if err := json.Unmarshal(m.Content, &contentStr); err == nil {
			contents = []message.Content{message.TextContent(contentStr)}
		} else {
			// Parse as array of content blocks
			var blocks []AnthropicContentBlock
			if err := json.Unmarshal(m.Content, &blocks); err == nil {
				for _, b := range blocks {
					if b.Type == "text" && b.Text != "" {
						contents = append(contents, message.TextContent(b.Text))
					}
				}
			}
		}

		req.Messages = append(req.Messages, message.Message{
			Role:    m.Role,
			Content: contents,
		})
	}

	return req, nil
}

// MessageRequestToAnthropic converts MessageRequest to Anthropic format
func MessageRequestToAnthropic(req *message.MessageRequest) ([]byte, error) {
	raw := map[string]interface{}{
		"model":      req.Model,
		"max_tokens": req.MaxTokens,
		"stream":      req.Stream,
	}

	if req.Temperature > 0 {
		raw["temperature"] = req.Temperature
	}
	if req.TopP > 0 {
		raw["top_p"] = req.TopP
	}

	// Anthropic specific: system (from first system message)
	for _, msg := range req.Messages {
		if msg.Role == "system" && len(msg.Content) > 0 {
			if len(msg.Content) == 1 && msg.Content[0].Type == "text" {
				if text := msg.Content[0].AsText(); text != "" {
					raw["system"] = text
				}
			}
		}
	}

	// Messages (Anthropic format)
	messages := make([]map[string]interface{}, 0)
	for _, msg := range req.Messages {
		if msg.Role == "system" {
			continue // system is separate field in Anthropic
		}

		// Convert content
		content := make([]map[string]interface{}, 0)
		for _, c := range msg.Content {
			if c.IsText() {
				if text := c.AsText(); text != "" {
					content = append(content, map[string]interface{}{
						"type": "text",
						"text": text,
					})
				}
			}
		}

		messages = append(messages, map[string]interface{}{
			"role":    msg.Role,
			"content": content,
		})
	}
	raw["messages"] = messages

	return json.Marshal(raw)
}

// AnthropicResponseToMessageResponse converts Anthropic response to MessageResponse
func AnthropicResponseToMessageResponse(body []byte) (*message.MessageResponse, error) {
	var ar AnthropicResponse
	if err := json.Unmarshal(body, &ar); err != nil {
		return nil, err
	}

	resp := &message.MessageResponse{
		ID:    ar.ID,
		Model: ar.Model,
	}

	// Content
	var sb strings.Builder
	for _, block := range ar.Content {
		if block.Type == "text" {
			sb.WriteString(block.Text)
		}
	}
	text := sb.String()
	resp.Choices = append(resp.Choices, message.Choice{
		Index:        0,
		Message:      message.Message{Role: "assistant", Content: []message.Content{message.TextContent(text)}},
		FinishReason: ar.StopReason,
	})

	// Usage
	if ar.Usage != nil {
		resp.Usage = &message.Usage{
			InputTokens:  ar.Usage.InputTokens,
			OutputTokens: ar.Usage.OutputTokens,
			IsAccurate:   true,
		}
	}

	return resp, nil
}

// MessageResponseToAnthropic converts MessageResponse to Anthropic format
func MessageResponseToAnthropic(resp *message.MessageResponse) ([]byte, error) {
	var sb strings.Builder
	finishReason := ""
	if len(resp.Choices) > 0 {
		for _, c := range resp.Choices[0].Message.Content {
			if c.IsText() {
				if text := c.AsText(); text != "" {
					sb.WriteString(text)
				}
			}
		}
		finishReason = resp.Choices[0].FinishReason
	}
	content := sb.String()

	raw := map[string]interface{}{
		"id":    resp.ID,
		"type":  "message",
		"role":  "assistant",
		"model": resp.Model,
		"content": []map[string]interface{}{
			{
				"type": "text",
				"text": content,
			},
		},
		"stop_reason": finishReason,
	}

	if resp.Usage != nil {
		raw["usage"] = map[string]int{
			"input_tokens":  resp.Usage.InputTokens,
			"output_tokens": resp.Usage.OutputTokens,
		}
	}

	return json.Marshal(raw)
}

// AnthropicSSEChunk represents Anthropic SSE event
type AnthropicSSEChunk struct {
	Type    string                 `json:"type"`
	Index   int                    `json:"index,omitempty"`
	Message map[string]interface{} `json:"message,omitempty"`
	Delta   map[string]interface{} `json:"delta,omitempty"`
	Usage   map[string]int         `json:"usage,omitempty"`
}

// OpenAISSEToAnthropicSSE converts OpenAI SSE chunk to Anthropic SSE format
func OpenAISSEToAnthropicSSE(chunk *message.StreamChunk) []string {
	if len(chunk.Choices) == 0 {
		return nil
	}

	choice := chunk.Choices[0]
	events := make([]string, 0)

	// message_start
	if choice.Delta.Role != "" {
		event := AnthropicSSEChunk{
			Type: "message_start",
			Message: map[string]interface{}{
				"id":    chunk.ID,
				"model": chunk.Model,
				"role":  "assistant",
			},
		}
		data, _ := json.Marshal(event)
		events = append(events, "event: message_start\ndata: "+string(data)+"\n\n")
	}

	// content_block_delta
	if len(choice.Delta.Content) > 0 {
		text := ""
		for _, c := range choice.Delta.Content {
			if c.IsText() {
				text = c.AsText()
			}
		}
		if text != "" {
			event := AnthropicSSEChunk{
				Type:  "content_block_delta",
				Index: choice.Index,
				Delta: map[string]interface{}{
					"type": "text_delta",
					"text": text,
				},
			}
			data, _ := json.Marshal(event)
			events = append(events, "event: content_block_delta\ndata: "+string(data)+"\n\n")
		}
	}

	// message_delta
	if choice.FinishReason != nil {
		event := AnthropicSSEChunk{
			Type: "message_delta",
			Delta: map[string]interface{}{
				"stop_reason": *choice.FinishReason,
			},
		}
		if chunk.Usage != nil {
			event.Usage = map[string]int{
				"output_tokens": chunk.Usage.OutputTokens,
			}
		}
		data, _ := json.Marshal(event)
		events = append(events, "event: message_delta\ndata: "+string(data)+"\n\n")
	}

	return events
}

// AnthropicSSEToOpenAISSE converts Anthropic SSE line to OpenAI SSE format
func AnthropicSSEToOpenAISSE(line []byte) []byte {
	var event AnthropicSSEChunk
	if err := json.Unmarshal(line, &event); err != nil {
		return nil
	}

	switch event.Type {
	case "content_block_delta":
		text := ""
		if delta, ok := event.Delta["text"].(string); ok {
			text = delta
		}
		chunk := message.StreamChunk{
			ID:      "",
			Object:  "chat.completion.chunk",
			Model:   "",
			Choices: []message.StreamChoice{
				{
					Index: event.Index,
					Delta: message.Message{
						Content: []message.Content{message.TextContent(text)},
					},
				},
			},
		}
		data, _ := json.Marshal(chunk)
		return []byte("data: " + string(data) + "\n\n")

	case "message_delta":
		stopReason := ""
		if sr, ok := event.Delta["stop_reason"].(string); ok {
			stopReason = sr
		}
		chunk := message.StreamChunk{
			ID:      "",
			Object:  "chat.completion.chunk",
			Choices: []message.StreamChoice{
				{
					Index:        0,
					Delta:        message.Message{},
					FinishReason: &stopReason,
				},
			},
		}
		if event.Usage != nil {
			chunk.Usage = &message.Usage{
				OutputTokens: event.Usage["output_tokens"],
			}
		}
		data, _ := json.Marshal(chunk)
		return []byte("data: " + string(data) + "\n\n")

	case "message_start":
		// Skip, no equivalent in OpenAI SSE
		return nil

	default:
		return nil
	}
}