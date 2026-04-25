package translate

import (
	"encoding/json"
	"io"
	"strings"

	"github.com/tokzone/fluxcore/message"
)

func init() {
	registerChunkParser("gemini", parseGeminiChunk)
}

// GeminiRequest represents Gemini API request structure
type GeminiRequest struct {
	Contents          []GeminiContent         `json:"contents"`
	SystemInstruction *GeminiContent          `json:"systemInstruction,omitempty"`
	GenerationConfig  *GeminiGenerationConfig `json:"generationConfig,omitempty"`
	SafetySettings    []GeminiSafetySetting   `json:"safetySettings,omitempty"`
}

// GeminiContent represents a content block in Gemini
type GeminiContent struct {
	Role  string       `json:"role,omitempty"` // "user" or "model"
	Parts []GeminiPart `json:"parts"`
}

// GeminiPart represents a part in content
type GeminiPart struct {
	Text string `json:"text,omitempty"`
	// Additional fields for multimodal: inlineData, functionCall, functionResponse
}

// GeminiGenerationConfig represents generation parameters
type GeminiGenerationConfig struct {
	MaxOutputTokens int     `json:"maxOutputTokens,omitempty"`
	Temperature     float64 `json:"temperature,omitempty"`
	TopP            float64 `json:"topP,omitempty"`
	TopK            int     `json:"topK,omitempty"`
}

// GeminiSafetySetting represents safety configuration
type GeminiSafetySetting struct {
	Category  string `json:"category"`
	Threshold string `json:"threshold"`
}

// GeminiResponse represents Gemini API response structure
type GeminiResponse struct {
	Candidates    []GeminiCandidate    `json:"candidates"`
	UsageMetadata *GeminiUsageMetadata `json:"usageMetadata,omitempty"`
}

// GeminiCandidate represents a response candidate
type GeminiCandidate struct {
	Content      GeminiContent `json:"content"`
	FinishReason string        `json:"finishReason,omitempty"`
	Index        int           `json:"index,omitempty"`
}

// GeminiUsageMetadata represents token usage
type GeminiUsageMetadata struct {
	PromptTokenCount     int `json:"promptTokenCount"`
	CandidatesTokenCount int `json:"candidatesTokenCount"`
	TotalTokenCount      int `json:"totalTokenCount"`
}

// GeminiStreamChunk represents Gemini streaming response
type GeminiStreamChunk struct {
	Candidates    []GeminiCandidate    `json:"candidates,omitempty"`
	UsageMetadata *GeminiUsageMetadata `json:"usageMetadata,omitempty"`
}

// GeminiToMessageRequest converts Gemini format to MessageRequest
func GeminiToMessageRequest(body io.Reader) (*message.MessageRequest, error) {
	var gr GeminiRequest
	if err := json.NewDecoder(body).Decode(&gr); err != nil {
		return nil, err
	}

	req := &message.MessageRequest{}

	// Generation config -> params
	if gr.GenerationConfig != nil {
		req.MaxTokens = gr.GenerationConfig.MaxOutputTokens
		req.Temperature = gr.GenerationConfig.Temperature
		req.TopP = gr.GenerationConfig.TopP
	}

	// System instruction -> first system message
	if gr.SystemInstruction != nil {
		var sb strings.Builder
		for _, part := range gr.SystemInstruction.Parts {
			sb.WriteString(part.Text)
		}
		systemText := sb.String()
		if systemText != "" {
			req.Messages = append(req.Messages, message.Message{
				Role:    "system",
				Content: []message.Content{message.TextContent(systemText)},
			})
		}
	}

	// Contents -> messages
	for _, c := range gr.Contents {
		role := c.Role
		// Convert "model" role to "assistant"
		if role == "model" {
			role = "assistant"
		}

		var contents []message.Content
		for _, part := range c.Parts {
			if part.Text != "" {
				contents = append(contents, message.TextContent(part.Text))
			}
		}

		req.Messages = append(req.Messages, message.Message{
			Role:    role,
			Content: contents,
		})
	}

	return req, nil
}

// MessageRequestToGemini converts MessageRequest to Gemini format
func MessageRequestToGemini(req *message.MessageRequest) ([]byte, error) {
	raw := map[string]interface{}{
		"contents": []interface{}{},
	}

	// Generation config
	gc := map[string]interface{}{}
	if req.MaxTokens > 0 {
		gc["maxOutputTokens"] = req.MaxTokens
	}
	if req.Temperature > 0 {
		gc["temperature"] = req.Temperature
	}
	if req.TopP > 0 {
		gc["topP"] = req.TopP
	}
	if len(gc) > 0 {
		raw["generationConfig"] = gc
	}

	// System instruction (extract from system messages)
	for _, msg := range req.Messages {
		if msg.Role == "system" && len(msg.Content) > 0 {
			parts := []interface{}{}
			for _, c := range msg.Content {
				if c.IsText() {
					if text := c.AsText(); text != "" {
						parts = append(parts, map[string]interface{}{"text": text})
					}
				}
			}
			if len(parts) > 0 {
				raw["systemInstruction"] = map[string]interface{}{"parts": parts}
			}
			break // Only first system message
		}
	}

	// Contents (skip system messages)
	contents := []interface{}{}
	for _, msg := range req.Messages {
		if msg.Role == "system" {
			continue
		}

		role := msg.Role
		if role == "assistant" {
			role = "model" // Gemini uses "model" for assistant
		}

		parts := []interface{}{}
		for _, c := range msg.Content {
			if c.IsText() {
				if text := c.AsText(); text != "" {
					parts = append(parts, map[string]interface{}{"text": text})
				}
			}
		}

		if len(parts) > 0 {
			contents = append(contents, map[string]interface{}{
				"role":  role,
				"parts": parts,
			})
		}
	}
	raw["contents"] = contents

	return json.Marshal(raw)
}

// GeminiResponseToMessageResponse converts Gemini response to MessageResponse
func GeminiResponseToMessageResponse(body []byte) (*message.MessageResponse, error) {
	var gr GeminiResponse
	if err := json.Unmarshal(body, &gr); err != nil {
		return nil, err
	}

	resp := &message.MessageResponse{}

	// Candidates -> choices
	for i, c := range gr.Candidates {
		var sb strings.Builder
		for _, part := range c.Content.Parts {
			sb.WriteString(part.Text)
		}
		textContent := sb.String()

		resp.Choices = append(resp.Choices, message.Choice{
			Index: i,
			Message: message.Message{
				Role:    "assistant",
				Content: []message.Content{message.TextContent(textContent)},
			},
			FinishReason: c.FinishReason,
		})
	}

	// Usage metadata
	if gr.UsageMetadata != nil {
		resp.Usage = &message.Usage{
			InputTokens:  gr.UsageMetadata.PromptTokenCount,
			OutputTokens: gr.UsageMetadata.CandidatesTokenCount,
			IsAccurate:   true,
		}
	}

	return resp, nil
}

// MessageResponseToGemini converts MessageResponse to Gemini format
func MessageResponseToGemini(resp *message.MessageResponse) ([]byte, error) {
	candidates := []interface{}{}

	for i, choice := range resp.Choices {
		parts := []interface{}{}
		for _, c := range choice.Message.Content {
			if c.IsText() {
				if text := c.AsText(); text != "" {
					parts = append(parts, map[string]interface{}{"text": text})
				}
			}
		}

		candidates = append(candidates, map[string]interface{}{
			"index": i,
			"content": map[string]interface{}{
				"role":  "model",
				"parts": parts,
			},
			"finishReason": choice.FinishReason,
		})
	}

	raw := map[string]interface{}{
		"candidates": candidates,
	}

	if resp.Usage != nil {
		raw["usageMetadata"] = map[string]interface{}{
			"promptTokenCount":     resp.Usage.InputTokens,
			"candidatesTokenCount": resp.Usage.OutputTokens,
			"totalTokenCount":      resp.Usage.InputTokens + resp.Usage.OutputTokens,
		}
	}

	return json.Marshal(raw)
}

// GeminiSSEToOpenAISSE converts Gemini SSE line to OpenAI SSE format
func GeminiSSEToOpenAISSE(line []byte) []byte {
	var geminiChunk GeminiStreamChunk
	if err := json.Unmarshal(line, &geminiChunk); err != nil {
		return nil
	}

	if len(geminiChunk.Candidates) == 0 {
		return nil
	}

	candidate := geminiChunk.Candidates[0]

	// Build content from parts
	var content []message.Content
	for _, part := range candidate.Content.Parts {
		if part.Text != "" {
			content = append(content, message.TextContent(part.Text))
		}
	}

	chunk := message.StreamChunk{
		ID:     "",
		Object: "chat.completion.chunk",
		Model:  "",
		Choices: []message.StreamChoice{
			{
				Index: candidate.Index,
				Delta: message.Message{
					Role:    "assistant",
					Content: content,
				},
			},
		},
	}

	if candidate.FinishReason != "" {
		chunk.Choices[0].FinishReason = &candidate.FinishReason
	}

	if geminiChunk.UsageMetadata != nil {
		chunk.Usage = &message.Usage{
			InputTokens:  geminiChunk.UsageMetadata.PromptTokenCount,
			OutputTokens: geminiChunk.UsageMetadata.CandidatesTokenCount,
		}
	}

	data, _ := json.Marshal(chunk)
	return []byte("data: " + string(data) + "\n\n")
}

// OpenAISSEToGeminiSSE converts OpenAI SSE chunk to Gemini SSE format
func OpenAISSEToGeminiSSE(chunk *message.StreamChunk) []byte {
	if len(chunk.Choices) == 0 {
		return nil
	}

	choice := chunk.Choices[0]

	// Build parts from content
	parts := []interface{}{}
	for _, c := range choice.Delta.Content {
		if c.IsText() {
			if text := c.AsText(); text != "" {
				parts = append(parts, map[string]interface{}{"text": text})
			}
		}
	}

	geminiChunk := map[string]interface{}{
		"candidates": []interface{}{
			map[string]interface{}{
				"index": choice.Index,
				"content": map[string]interface{}{
					"role":  "model",
					"parts": parts,
				},
			},
		},
	}

	if choice.FinishReason != nil {
		candidates := geminiChunk["candidates"].([]interface{})
		candidates[0].(map[string]interface{})["finishReason"] = *choice.FinishReason
	}

	if chunk.Usage != nil {
		geminiChunk["usageMetadata"] = map[string]interface{}{
			"promptTokenCount":     chunk.Usage.InputTokens,
			"candidatesTokenCount": chunk.Usage.OutputTokens,
		}
	}

	data, _ := json.Marshal(geminiChunk)
	return []byte("data: " + string(data) + "\n\n")
}

// parseGeminiChunk parses Gemini streaming response to StreamChunk
func parseGeminiChunk(data []byte) (*message.StreamChunk, error) {
	var gemini GeminiStreamChunk
	if err := json.Unmarshal(data, &gemini); err != nil {
		return nil, err
	}

	chunk := &message.StreamChunk{
		Object: "chat.completion.chunk",
	}

	if len(gemini.Candidates) > 0 {
		c := gemini.Candidates[0]
		var content []message.Content
		for _, part := range c.Content.Parts {
			if part.Text != "" {
				content = append(content, message.TextContent(part.Text))
			}
		}
		chunk.Choices = []message.StreamChoice{
			{
				Index: c.Index,
				Delta: message.Message{
					Role:    "assistant",
					Content: content,
				},
			},
		}
		if c.FinishReason != "" {
			chunk.Choices[0].FinishReason = &c.FinishReason
		}
	}

	if gemini.UsageMetadata != nil {
		chunk.Usage = &message.Usage{
			InputTokens:  gemini.UsageMetadata.PromptTokenCount,
			OutputTokens: gemini.UsageMetadata.CandidatesTokenCount,
		}
	}

	return chunk, nil
}
