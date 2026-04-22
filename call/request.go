package call

import (
	"bytes"
	"encoding/json"
	"net/http"

	"github.com/tokzone/fluxcore/message"
	"github.com/tokzone/fluxcore/routing"
	"github.com/tokzone/fluxcore/internal/translate"
)

// buildURL constructs the API URL for an endpoint
func buildURL(ep *routing.Endpoint, stream bool) string {
	var path string
	switch ep.Key.Protocol {
	case routing.ProtocolGemini:
		if stream {
			path = "/v1/models/" + ep.Model + ":streamGenerateContent?alt=sse"
		} else {
			path = "/v1/models/" + ep.Model + ":generateContent"
		}
	case routing.ProtocolAnthropic:
		path = "/v1/messages"
	case routing.ProtocolCohere:
		path = "/v1/chat"
	default:
		path = "/v1/chat/completions"
	}
	return ep.Key.BaseURL + path
}

// setHeaders sets the required headers for an API request
func setHeaders(req *http.Request, ep *routing.Endpoint, stream bool) {
	req.Header.Set("Content-Type", "application/json")
	if stream {
		req.Header.Set("Accept", "text/event-stream")
	}

	switch ep.Key.Protocol {
	case routing.ProtocolGemini:
		req.Header.Set("x-goog-api-key", ep.Key.APIKey)
	case routing.ProtocolAnthropic:
		req.Header.Set("anthropic-version", "2023-06-01")
		req.Header.Set("x-api-key", ep.Key.APIKey)
	case routing.ProtocolCohere:
		req.Header.Set("Authorization", "Bearer "+ep.Key.APIKey)
	default:
		req.Header.Set("Authorization", "Bearer "+ep.Key.APIKey)
	}
}

// selectEndpoint selects an available endpoint from the pool
func selectEndpoint(pool *routing.EndpointPool) (*routing.Endpoint, error) {
	ep := pool.CurrentEp()
	if ep == nil || ep.IsCircuitBreakerOpen() {
		ep = pool.SelectBest()
	}
	if ep == nil {
		return nil, routing.ErrNoEndpoints
	}
	return ep, nil
}

// translateRequest converts MessageRequest to protocol-specific format
func translateRequest(req *message.MessageRequest, protocol routing.Protocol) ([]byte, error) {
	switch protocol {
	case routing.ProtocolAnthropic:
		return translate.MessageRequestToAnthropic(req)
	case routing.ProtocolGemini:
		return translate.MessageRequestToGemini(req)
	case routing.ProtocolCohere:
		return translate.MessageRequestToCohere(req)
	default:
		return json.Marshal(req)
	}
}

// translateResponse converts protocol-specific response to MessageResponse
func translateResponse(respBody []byte, protocol routing.Protocol) (*message.MessageResponse, error) {
	switch protocol {
	case routing.ProtocolAnthropic:
		return translate.AnthropicResponseToMessageResponse(respBody)
	case routing.ProtocolGemini:
		return translate.GeminiResponseToMessageResponse(respBody)
	case routing.ProtocolCohere:
		return translate.CohereResponseToMessageResponse(respBody)
	default:
		return message.ParseResponse(respBody)
	}
}

// translateOutput converts MessageResponse to protocol-specific output format
func translateOutput(resp *message.MessageResponse, protocol routing.Protocol) ([]byte, error) {
	switch protocol {
	case routing.ProtocolAnthropic:
		return translate.MessageResponseToAnthropic(resp)
	case routing.ProtocolGemini:
		return translate.MessageResponseToGemini(resp)
	case routing.ProtocolCohere:
		return translate.MessageResponseToCohere(resp)
	default:
		return json.Marshal(resp)
	}
}

// parseRequest parses raw request bytes to MessageRequest based on protocol
func parseRequest(rawReq []byte, protocol routing.Protocol) (*message.MessageRequest, error) {
	switch protocol {
	case routing.ProtocolAnthropic:
		return translate.AnthropicToMessageRequest(bytes.NewReader(rawReq))
	case routing.ProtocolGemini:
		return translate.GeminiToMessageRequest(bytes.NewReader(rawReq))
	case routing.ProtocolCohere:
		return translate.CohereToMessageRequest(bytes.NewReader(rawReq))
	default:
		return message.ParseRequest(rawReq)
	}
}