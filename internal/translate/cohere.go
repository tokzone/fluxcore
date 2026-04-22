package translate

import (
	"encoding/json"
	"io"
	"strings"

	"github.com/tokzone/fluxcore/message"
)

func init() {
	RegisterChunkParser("cohere", parseCohereChunk)
}

// CohereRequest represents Cohere API request structure
type CohereRequest struct {
	Message      string          `json:"message"`
	ChatHistory  []CohereMessage `json:"chat_history,omitempty"`
	Preamble     string          `json:"preamble,omitempty"`
	MaxTokens    int             `json:"max_tokens,omitempty"`
	Temperature  float64         `json:"temperature,omitempty"`
	P            float64         `json:"p,omitempty"`
	K            int             `json:"k,omitempty"`
	Stream       bool            `json:"stream,omitempty"`
	Connectors   []interface{}   `json:"connectors,omitempty"`
}

// CohereMessage represents a message in chat history
type CohereMessage struct {
	Role    string `json:"role"`    // USER or CHATBOT
	Message string `json:"message"`
}

// CohereResponse represents Cohere API response structure
type CohereResponse struct {
	Text         string            `json:"text"`
	IsFinished   bool              `json:"is_finished"`
	FinishReason string            `json:"finish_reason"`
	Meta         *CohereMeta       `json:"meta,omitempty"`
	TokenCount   *CohereTokenCount `json:"token_count,omitempty"`
}

// CohereMeta contains metadata about the response
type CohereMeta struct {
	APIVersion struct {
		Version string `json:"version"`
	} `json:"api_version"`
	BilledUnits *CohereBilledUnits `json:"billed_units,omitempty"`
}

// CohereBilledUnits contains billing information
type CohereBilledUnits struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// CohereTokenCount contains token usage
type CohereTokenCount struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// CohereStreamChunk represents Cohere streaming response
type CohereStreamChunk struct {
	Text         string            `json:"text,omitempty"`
	IsFinished   bool              `json:"is_finished,omitempty"`
	FinishReason string            `json:"finish_reason,omitempty"`
	TokenCount   *CohereTokenCount `json:"token_count,omitempty"`
	EventType    string            `json:"event_type,omitempty"` // text-generation, stream-end
}

// CohereToMessageRequest converts Cohere format to MessageRequest
func CohereToMessageRequest(body io.Reader) (*message.MessageRequest, error) {
	var cr CohereRequest
	if err := json.NewDecoder(body).Decode(&cr); err != nil {
		return nil, err
	}

	req := &message.MessageRequest{
		MaxTokens:   cr.MaxTokens,
		Temperature: cr.Temperature,
		TopP:        cr.P,
		Stream:      cr.Stream,
	}

	// Preamble -> system message
	if cr.Preamble != "" {
		req.Messages = append(req.Messages, message.Message{
			Role:    "system",
			Content: []message.Content{message.TextContent(cr.Preamble)},
		})
	}

	// Chat history -> messages
	for _, m := range cr.ChatHistory {
		role := m.Role
		// Convert USER/CHATBOT to user/assistant
		if role == "USER" {
			role = "user"
		} else if role == "CHATBOT" {
			role = "assistant"
		}

		if m.Message != "" {
			req.Messages = append(req.Messages, message.Message{
				Role:    role,
				Content: []message.Content{message.TextContent(m.Message)},
			})
		}
	}

	// Current message -> last user message
	if cr.Message != "" {
		req.Messages = append(req.Messages, message.Message{
			Role:    "user",
			Content: []message.Content{message.TextContent(cr.Message)},
		})
	}

	return req, nil
}

// MessageRequestToCohere converts MessageRequest to Cohere format
func MessageRequestToCohere(req *message.MessageRequest) ([]byte, error) {
	raw := map[string]interface{}{
		"message": "",
	}

	// Basic fields
	if req.MaxTokens > 0 {
		raw["max_tokens"] = req.MaxTokens
	}
	if req.Temperature > 0 {
		raw["temperature"] = req.Temperature
	}
	if req.TopP > 0 {
		raw["p"] = req.TopP
	}
	if req.Stream {
		raw["stream"] = true
	}

	// Extract system message -> preamble
	for _, msg := range req.Messages {
		if msg.Role == "system" && len(msg.Content) > 0 {
			if len(msg.Content) == 1 && msg.Content[0].Type == "text" {
				if text := msg.Content[0].AsText(); text != "" {
					raw["preamble"] = text
				}
			}
			break
		}
	}

	// Messages -> chat_history + message
	chatHistory := []interface{}{}
	lastUserMessage := ""

	for i, msg := range req.Messages {
		if msg.Role == "system" {
			continue // already in preamble
		}

		role := msg.Role
		if role == "assistant" {
			role = "CHATBOT"
		} else if role == "user" {
			role = "USER"
		}

		// Get text content
		var sb strings.Builder
		for _, c := range msg.Content {
			if c.IsText() {
				if t := c.AsText(); t != "" {
					sb.WriteString(t)
				}
			}
		}
		text := sb.String()

		if text == "" {
			continue
		}

		// Last user message goes to message field
		if msg.Role == "user" && i == len(req.Messages)-1 {
			lastUserMessage = text
		} else {
			chatHistory = append(chatHistory, map[string]interface{}{
				"role":    role,
				"message": text,
			})
		}
	}

	raw["message"] = lastUserMessage
	if len(chatHistory) > 0 {
		raw["chat_history"] = chatHistory
	}

	return json.Marshal(raw)
}

// CohereResponseToMessageResponse converts Cohere response to MessageResponse
func CohereResponseToMessageResponse(body []byte) (*message.MessageResponse, error) {
	var cr CohereResponse
	if err := json.Unmarshal(body, &cr); err != nil {
		return nil, err
	}

	resp := &message.MessageResponse{}

	// Text -> content
	if cr.Text != "" {
		resp.Choices = append(resp.Choices, message.Choice{
			Index: 0,
			Message: message.Message{
				Role:    "assistant",
				Content: []message.Content{message.TextContent(cr.Text)},
			},
			FinishReason: cr.FinishReason,
		})
	}

	// Usage from meta.billed_units
	if cr.Meta != nil && cr.Meta.BilledUnits != nil {
		resp.Usage = &message.Usage{
			InputTokens:  cr.Meta.BilledUnits.InputTokens,
			OutputTokens: cr.Meta.BilledUnits.OutputTokens,
			IsAccurate:   true,
		}
	}

	// Alternative: token_count field
	if cr.TokenCount != nil {
		if resp.Usage == nil {
			resp.Usage = &message.Usage{}
		}
		resp.Usage.InputTokens = cr.TokenCount.InputTokens
		resp.Usage.OutputTokens = cr.TokenCount.OutputTokens
		resp.Usage.IsAccurate = true
	}

	return resp, nil
}

// MessageResponseToCohere converts MessageResponse to Cohere format
func MessageResponseToCohere(resp *message.MessageResponse) ([]byte, error) {
	var sb strings.Builder
	finishReason := ""

	if len(resp.Choices) > 0 {
		for _, c := range resp.Choices[0].Message.Content {
			if c.IsText() {
				if t := c.AsText(); t != "" {
					sb.WriteString(t)
				}
			}
		}
		finishReason = resp.Choices[0].FinishReason
	}
	text := sb.String()

	raw := map[string]interface{}{
		"text":         text,
		"is_finished":  true,
		"finish_reason": finishReason,
	}

	if resp.Usage != nil {
		raw["token_count"] = map[string]int{
			"input_tokens":  resp.Usage.InputTokens,
			"output_tokens": resp.Usage.OutputTokens,
		}
	}

	return json.Marshal(raw)
}

// CohereSSEToOpenAISSE converts Cohere SSE line to OpenAI SSE format
func CohereSSEToOpenAISSE(line []byte) []byte {
	var cohereChunk CohereStreamChunk
	if err := json.Unmarshal(line, &cohereChunk); err != nil {
		return nil
	}

	// Only process text-generation and stream-end events
	if cohereChunk.EventType != "text-generation" && cohereChunk.EventType != "stream-end" {
		return nil
	}

	chunk := message.StreamChunk{
		ID:     "",
		Object: "chat.completion.chunk",
		Model:  "",
		Choices: []message.StreamChoice{
			{
				Index: 0,
				Delta: message.Message{
					Role:    "assistant",
					Content: []message.Content{message.TextContent(cohereChunk.Text)},
				},
			},
		},
	}

	if cohereChunk.IsFinished {
		finishReason := cohereChunk.FinishReason
		if finishReason == "" {
			finishReason = "complete"
		}
		chunk.Choices[0].FinishReason = &finishReason
	}

	if cohereChunk.TokenCount != nil {
		chunk.Usage = &message.Usage{
			InputTokens:  cohereChunk.TokenCount.InputTokens,
			OutputTokens: cohereChunk.TokenCount.OutputTokens,
		}
	}

	data, _ := json.Marshal(chunk)
	return []byte("data: " + string(data) + "\n\n")
}

// OpenAISSEToCohereSSE converts OpenAI SSE chunk to Cohere SSE format
func OpenAISSEToCohereSSE(chunk *message.StreamChunk) []byte {
	if len(chunk.Choices) == 0 {
		return nil
	}

	choice := chunk.Choices[0]

	// Get text content
	var sb strings.Builder
	for _, c := range choice.Delta.Content {
		if c.IsText() {
			if t := c.AsText(); t != "" {
				sb.WriteString(t)
			}
		}
	}
	text := sb.String()

	cohereChunk := map[string]interface{}{
		"event_type": "text-generation",
		"text":       text,
	}

	if choice.FinishReason != nil {
		cohereChunk["is_finished"] = true
		cohereChunk["finish_reason"] = *choice.FinishReason
		cohereChunk["event_type"] = "stream-end"
	}

	if chunk.Usage != nil {
		cohereChunk["token_count"] = map[string]int{
			"input_tokens":  chunk.Usage.InputTokens,
			"output_tokens": chunk.Usage.OutputTokens,
		}
	}

	data, _ := json.Marshal(cohereChunk)
	return []byte("data: " + string(data) + "\n\n")
}

// parseCohereChunk parses Cohere streaming response to StreamChunk
func parseCohereChunk(data []byte) (*message.StreamChunk, error) {
	var cohere CohereStreamChunk
	if err := json.Unmarshal(data, &cohere); err != nil {
		return nil, err
	}

	// Only process text-generation and stream-end events
	if cohere.EventType != "text-generation" && cohere.EventType != "stream-end" {
		return nil, nil
	}

	chunk := &message.StreamChunk{
		Object: "chat.completion.chunk",
	}

	if cohere.Text != "" {
		chunk.Choices = []message.StreamChoice{
			{
				Index: 0,
				Delta: message.Message{
					Role:    "assistant",
					Content: []message.Content{message.TextContent(cohere.Text)},
				},
			},
		}
	}

	if cohere.IsFinished {
		finishReason := cohere.FinishReason
		if finishReason == "" {
			finishReason = "complete"
		}
		if len(chunk.Choices) == 0 {
			chunk.Choices = []message.StreamChoice{{Index: 0}}
		}
		chunk.Choices[0].FinishReason = &finishReason
	}

	if cohere.TokenCount != nil {
		chunk.Usage = &message.Usage{
			InputTokens:  cohere.TokenCount.InputTokens,
			OutputTokens: cohere.TokenCount.OutputTokens,
		}
	}

	return chunk, nil
}