// Package message defines the intermediate representation (IR) for LLM requests and responses.
//
// The message package provides:
//   - Protocol-agnostic request/response structures
//   - Multimodal content support (text, image, audio)
//   - Token usage tracking
//   - JSON serialization with custom handling
//
// Core types:
//   - MessageRequest: Chat completion request
//   - MessageResponse: Chat completion response
//   - Message: Single message with role and content
//   - Content: Multimodal content item
//   - Usage: Token usage statistics
//
// Content types:
//   - TextContent: Plain text content
//   - ImageContent: Image content (URL or base64)
//   - AudioContent: Audio content (URL or base64)
//
// Example usage:
//
//	req := &message.MessageRequest{
//	    Model: "gpt-4",
//	    Messages: []message.Message{
//	        {Role: "user", Content: []message.Content{
//	            message.TextContent("Hello"),
//	            message.ImageContent("https://example.com/img.png", "image/png", ""),
//	        }},
//	    },
//	    MaxTokens: 100,
//	}
//
// Builder methods:
//
//	req = req.WithStream(true)
//	req = req.WithModel("gpt-3.5-turbo")
//	cloned := req.Clone()
package message