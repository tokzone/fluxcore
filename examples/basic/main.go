// Basic example: non-streaming request
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/tokzone/fluxcore/call"
	"github.com/tokzone/fluxcore/message"
	"github.com/tokzone/fluxcore/routing"
)

func main() {
	// Define API key
	key := &routing.Key{
		BaseURL:  "https://api.openai.com/v1",
		APIKey:   "sk-your-api-key",
		Protocol: routing.ProtocolOpenAI,
	}

	// Create endpoint with pricing
	ep := routing.NewEndpoint(1, key, "", 0.01, 0.03)

	// Create pool
	pool := routing.NewEndpointPool([]*routing.Endpoint{ep}, 3)

	// Build request
	req := &message.MessageRequest{
		Model: "gpt-4",
		Messages: []message.Message{
			{Role: "user", Content: []message.Content{message.TextContent("Hello!")}},
		},
		MaxTokens: 100,
	}

	// Send request
	rawReq, _ := message.ParseRequest([]byte(`{"model":"gpt-4","messages":[{"role":"user","content":[{"type":"text","data":"Hello!"}]}],"max_tokens":100}`))
	_ = rawReq // Use in real code

	resp, usage, err := call.Request(context.Background(), pool, mustMarshal(req), routing.ProtocolOpenAI)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Response: %s\n", resp)
	fmt.Printf("Tokens: in=%d, out=%d\n", usage.InputTokens, usage.OutputTokens)
}

func mustMarshal(req *message.MessageRequest) []byte {
	// Simplified for example
	return []byte(`{"model":"gpt-4","messages":[{"role":"user","content":[{"type":"text","data":"Hello!"}]}],"max_tokens":100}`)
}