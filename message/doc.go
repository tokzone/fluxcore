// Package message defines the intermediate representation (IR) for LLM requests and responses.
//
// The message package provides:
//   - Protocol-agnostic request/response structures
//   - Content types (text, image, audio)
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
// Content construction:
//   - TextContent(text): Create text content
//
// Example usage:
//
//	req := &message.MessageRequest{
//	    Model: "gpt-4",
//	    Messages: []message.Message{
//	        {Role: "user", Content: []message.Content{
//	            message.TextContent("Hello"),
//	        }},
//	    },
//	    MaxTokens: 100,
//	}
//
// Stream mode:
//
//	req = req.WithStream(true)
package message
