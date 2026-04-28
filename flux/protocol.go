package flux

import (
	"bytes"
	"encoding/json"
	"net/http"

	"github.com/tokzone/fluxcore/internal/translate"
	"github.com/tokzone/fluxcore/message"
	"github.com/tokzone/fluxcore/provider"
)

// buildURL constructs the API URL for a user endpoint and target protocol.
func buildURL(ue *UserEndpoint, targetProtocol provider.Protocol, stream bool) string {
	if ue == nil {
		return ""
	}
	var path string
	switch targetProtocol {
	case provider.ProtocolGemini:
		if stream {
			path = "/v1/models/" + ue.Model() + ":streamGenerateContent?alt=sse"
		} else {
			path = "/v1/models/" + ue.Model() + ":generateContent"
		}
	case provider.ProtocolAnthropic:
		path = "/v1/messages"
	case provider.ProtocolCohere:
		path = "/v1/chat"
	default:
		path = "/v1/chat/completions"
	}
	return ue.BaseURL(targetProtocol) + path
}

// setHeaders sets the required headers for an API request
func setHeaders(req *http.Request, ue *UserEndpoint, targetProtocol provider.Protocol, stream bool) {
	if ue == nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	if stream {
		req.Header.Set("Accept", "text/event-stream")
	}

	switch targetProtocol {
	case provider.ProtocolGemini:
		req.Header.Set("x-goog-api-key", ue.Secret())
	case provider.ProtocolAnthropic:
		req.Header.Set("anthropic-version", "2023-06-01")
		req.Header.Set("x-api-key", ue.Secret())
	case provider.ProtocolCohere:
		req.Header.Set("Authorization", "Bearer "+ue.Secret())
	default:
		req.Header.Set("Authorization", "Bearer "+ue.Secret())
	}
}

// translateRequest converts MessageRequest to protocol-specific format
func translateRequest(req *message.MessageRequest, protocol provider.Protocol) ([]byte, error) {
	switch protocol {
	case provider.ProtocolAnthropic:
		return translate.MessageRequestToAnthropic(req)
	case provider.ProtocolGemini:
		return translate.MessageRequestToGemini(req)
	case provider.ProtocolCohere:
		return translate.MessageRequestToCohere(req)
	default:
		return json.Marshal(req)
	}
}

// translateResponse converts protocol-specific response to MessageResponse
func translateResponse(respBody []byte, protocol provider.Protocol) (*message.MessageResponse, error) {
	switch protocol {
	case provider.ProtocolAnthropic:
		return translate.AnthropicResponseToMessageResponse(respBody)
	case provider.ProtocolGemini:
		return translate.GeminiResponseToMessageResponse(respBody)
	case provider.ProtocolCohere:
		return translate.CohereResponseToMessageResponse(respBody)
	default:
		return message.ParseResponse(respBody)
	}
}

// translateOutput converts MessageResponse to protocol-specific output format
func translateOutput(resp *message.MessageResponse, protocol provider.Protocol) ([]byte, error) {
	switch protocol {
	case provider.ProtocolAnthropic:
		return translate.MessageResponseToAnthropic(resp)
	case provider.ProtocolGemini:
		return translate.MessageResponseToGemini(resp)
	case provider.ProtocolCohere:
		return translate.MessageResponseToCohere(resp)
	default:
		return json.Marshal(resp)
	}
}

// parseRequest parses raw request bytes to MessageRequest based on protocol
func parseRequest(rawReq []byte, protocol provider.Protocol) (*message.MessageRequest, error) {
	switch protocol {
	case provider.ProtocolAnthropic:
		return translate.AnthropicToMessageRequest(bytes.NewReader(rawReq))
	case provider.ProtocolGemini:
		return translate.GeminiToMessageRequest(bytes.NewReader(rawReq))
	case provider.ProtocolCohere:
		return translate.CohereToMessageRequest(bytes.NewReader(rawReq))
	default:
		return message.ParseRequest(rawReq)
	}
}
