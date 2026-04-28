// Streaming example: SSE response handling
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/tokzone/fluxcore/endpoint"
	"github.com/tokzone/fluxcore/flux"
	"github.com/tokzone/fluxcore/provider"
)

func main() {
	// 1. Define global provider
	openai := provider.NewProvider(1, provider.SingleBaseURL("https://api.openai.com"))

	// 2. Register endpoint to global registry
	endpoint.RegisterEndpoint(1, openai, "", []provider.Protocol{provider.ProtocolOpenAI})

	// 3. Create APIKey (Provider + Secret)
	key, err := flux.NewAPIKey(openai, "sk-your-api-key")
	if err != nil {
		log.Fatal(err)
	}

	// 4. Create UserEndpoint (Endpoint + APIKey + Priority)
	ue, err := flux.NewUserEndpoint("", key, 1000)
	if err != nil {
		log.Fatal(err)
	}

	// 5. Create client
	client := flux.NewClient([]*flux.UserEndpoint{ue}, flux.WithRetryMax(3))

	// 6. Request body
	rawReq := []byte(`{
		"model": "gpt-4",
		"messages": [{"role": "user", "content": [{"type": "text", "data": "Tell me a joke"}]}],
		"max_tokens": 100
	}`)

	// 7. Send streaming request
	result, err := client.DoStream(context.Background(), rawReq, provider.ProtocolOpenAI)
	if err != nil {
		log.Fatal(err)
	}
	defer result.Close()

	// 8. Read chunks
	for chunk := range result.Ch {
		fmt.Printf("Chunk: %s\n", chunk)
	}

	// 9. Get final usage
	usage := result.Usage()
	fmt.Printf("Tokens: in=%d, out=%d\n", usage.InputTokens, usage.OutputTokens)
}
