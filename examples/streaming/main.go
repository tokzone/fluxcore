// Streaming example: SSE response handling
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/tokzone/fluxcore/call"
	"github.com/tokzone/fluxcore/routing"
)

func main() {
	// Define API key
	key := &routing.Key{
		BaseURL:  "https://api.openai.com/v1",
		APIKey:   "sk-your-api-key",
		Protocol: routing.ProtocolOpenAI,
	}

	// Create endpoint and pool
	ep := routing.NewEndpoint(1, key, "", 0.01, 0.03)
	pool := routing.NewEndpointPool([]*routing.Endpoint{ep}, 3)

	// Request body
	rawReq := []byte(`{
		"model": "gpt-4",
		"messages": [{"role": "user", "content": [{"type": "text", "data": "Tell me a joke"}]}],
		"max_tokens": 100
	}`)

	// Send streaming request
	result, err := call.RequestStream(context.Background(), pool, rawReq, routing.ProtocolOpenAI)
	if err != nil {
		log.Fatal(err)
	}
	defer result.Close()

	// Read chunks
	for chunk := range result.Ch {
		fmt.Printf("Chunk: %s\n", chunk)
	}

	// Get final usage
	usage := result.Usage()
	fmt.Printf("Tokens: in=%d, out=%d\n", usage.InputTokens, usage.OutputTokens)
}