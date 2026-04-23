package translate

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/tokzone/fluxcore/message"
)

// ChunkParser parses SSE chunk data to StreamChunk
type ChunkParser func(data []byte) (*message.StreamChunk, error)

var chunkParsers = make(map[string]ChunkParser)

// RegisterChunkParser registers a parser for a format
func RegisterChunkParser(format string, parser ChunkParser) {
	chunkParsers[format] = parser
}

// getChunkParser returns the parser for a format, or nil for OpenAI format
func getChunkParser(format string) ChunkParser {
	return chunkParsers[format]
}

// SSE event type constants
const (
	SSETypeData  = "data"
	SSETypeEvent = "event"
	SSETypeDone  = "done"
	SSETypeError = "error"
)

// SSEConfig holds SSE configuration.
type SSEConfig struct {
	BufferSize    int // Read buffer size (default: 4096)
	ChannelBuffer int // Event channel buffer size (default: 100)
}

var sseConfig = SSEConfig{
	BufferSize:    4096,
	ChannelBuffer: 100,
}

var sseConfigMu sync.RWMutex

// SetSSEConfig updates the SSE configuration.
// Zero values are ignored (keep current defaults).
func SetSSEConfig(cfg *SSEConfig) {
	if cfg == nil {
		return
	}
	sseConfigMu.Lock()
	defer sseConfigMu.Unlock()
	if cfg.BufferSize > 0 {
		sseConfig.BufferSize = cfg.BufferSize
	}
	if cfg.ChannelBuffer > 0 {
		sseConfig.ChannelBuffer = cfg.ChannelBuffer
	}
}

// GetSSEConfig returns the current SSE configuration.
func GetSSEConfig() SSEConfig {
	sseConfigMu.RLock()
	defer sseConfigMu.RUnlock()
	return sseConfig
}

// SSEBufferSize returns the current buffer size.
// Deprecated: Use GetSSEConfig().BufferSize instead.
func SSEBufferSize() int {
	sseConfigMu.RLock()
	defer sseConfigMu.RUnlock()
	return sseConfig.BufferSize
}

// SSEChannelBuffer returns the current channel buffer size.
// Deprecated: Use GetSSEConfig().ChannelBuffer instead.
func SSEChannelBuffer() int {
	sseConfigMu.RLock()
	defer sseConfigMu.RUnlock()
	return sseConfig.ChannelBuffer
}

type SSEEvent struct {
	Type   string            // SSETypeData, SSETypeEvent, SSETypeDone, SSETypeError
	Data   []byte            // Raw data payload
	Chunk  *message.StreamChunk // Parsed chunk (for data events)
	Format string            // Source format
}

type SSEParseResult struct {
	Event SSEEvent
	Usage *message.Usage
	Error error
}

func ParseSSEStream(ctx context.Context, reader io.ReadCloser, format string, startTime time.Time) chan SSEParseResult {
	sseConfigMu.RLock()
	bufSize := sseConfig.BufferSize
	chBuf := sseConfig.ChannelBuffer
	sseConfigMu.RUnlock()

	ch := make(chan SSEParseResult, chBuf)

	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[fluxcore] SSE parser panic recovered: %v", r)
			}
		}()
		defer close(ch)
		defer reader.Close()

		buf := make([]byte, bufSize)
		accumulated := &strings.Builder{}
		usageData := &message.Usage{}

		for {
			select {
			case <-ctx.Done():
				usageData.LatencyMs = int(time.Since(startTime).Milliseconds())
				ch <- SSEParseResult{Usage: usageData, Error: ctx.Err()}
				return
			default:
			}

			n, readErr := reader.Read(buf)
			if n > 0 {
				accumulated.Write(buf[:n])
				data := accumulated.String()

				lines := strings.Split(data, "\n\n")
				accumulated.Reset()
				accumulated.WriteString(lines[len(lines)-1])

				for i := 0; i < len(lines)-1; i++ {
					line := lines[i]
					result := parseSSELine(line, format, startTime, usageData)
					if result.Event.Type != "" {
						ch <- result
					}
				}
			}

			if readErr != nil {
				if readErr != io.EOF {
					log.Printf("[fluxcore] stream read error: %v", readErr)
					ch <- SSEParseResult{Error: readErr}
				}
				usageData.LatencyMs = int(time.Since(startTime).Milliseconds())
				ch <- SSEParseResult{Usage: usageData}
				return
			}
		}
	}()

	return ch
}

func parseSSELine(line, format string, startTime time.Time, usageData *message.Usage) SSEParseResult {
	result := SSEParseResult{}

	if strings.HasPrefix(line, "data: ") {
		dataStr := strings.TrimPrefix(line, "data: ")

		if dataStr == "[DONE]" {
			result.Event = SSEEvent{
				Type: SSETypeDone,
				Data: []byte("[DONE]"),
			}
			usageData.LatencyMs = int(time.Since(startTime).Milliseconds())
			return result
		}

		parser := getChunkParser(format)
		if parser != nil {
			chunk, err := parser([]byte(dataStr))
			if err != nil {
				log.Printf("[fluxcore] malformed %s SSE chunk: %v", format, err)
				result.Error = err
				return result
			}
			if chunk == nil {
				return result // skip non-text events
			}
			result.Event = SSEEvent{
				Type:   SSETypeData,
				Data:   []byte(dataStr),
				Chunk:  chunk,
				Format: format,
			}
			if chunk.Usage != nil {
				usageData.InputTokens = chunk.Usage.InputTokens
				usageData.OutputTokens = chunk.Usage.OutputTokens
				usageData.IsAccurate = true
			}
		} else {
			// OpenAI/Anthropic format (default)
			var chunk message.StreamChunk
			if err := json.Unmarshal([]byte(dataStr), &chunk); err != nil {
				log.Printf("[fluxcore] malformed SSE chunk: %v", err)
				result.Error = err
				return result
			}
			result.Event = SSEEvent{
				Type:   SSETypeData,
				Data:   []byte(dataStr),
				Chunk:  &chunk,
				Format: format,
			}
			if chunk.Usage != nil {
				usageData.InputTokens = chunk.Usage.InputTokens
				usageData.OutputTokens = chunk.Usage.OutputTokens
				usageData.IsAccurate = true
			}
		}
	} else if strings.HasPrefix(line, "event: ") {
		result.Event = SSEEvent{
			Type: SSETypeEvent,
			Data: []byte(line),
		}
	}

	return result
}

// SSE conversion function types
type dataConverter func([]byte) []byte
type chunkConverter func(*message.StreamChunk) []byte

// Conversion registries
var (
	// toOpenAI converts format-specific SSE data to OpenAI format
	toOpenAI = map[string]dataConverter{
		"anthropic": AnthropicSSEToOpenAISSE,
		"gemini":    GeminiSSEToOpenAISSE,
		"cohere":    CohereSSEToOpenAISSE,
	}

	// fromOpenAI converts OpenAI StreamChunk to format-specific SSE
	fromOpenAI = map[string]chunkConverter{
		"anthropic": func(c *message.StreamChunk) []byte { return joinAnthropicEvents(OpenAISSEToAnthropicSSE(c)) },
		"gemini":    OpenAISSEToGeminiSSE,
		"cohere":    OpenAISSEToCohereSSE,
	}
)

func ConvertSSEEvent(event SSEEvent, fromFormat, toFormat string) []byte {
	if fromFormat == toFormat {
		if event.Type == SSETypeData {
			return []byte("data: " + string(event.Data) + "\n\n")
		} else if event.Type == SSETypeEvent {
			return []byte(string(event.Data) + "\n\n")
		}
		return nil
	}

	if event.Type != SSETypeData {
		return nil
	}

	// Direct conversion to OpenAI
	if toFormat == "openai" {
		if conv, ok := toOpenAI[fromFormat]; ok && event.Data != nil {
			return conv(event.Data)
		}
		return nil
	}

	// Direct conversion from OpenAI
	if fromFormat == "openai" {
		if conv, ok := fromOpenAI[toFormat]; ok && event.Chunk != nil {
			return conv(event.Chunk)
		}
		return nil
	}

	// Indirect conversion via OpenAI
	toOpenAIConv, hasToOpenAI := toOpenAI[fromFormat]
	fromOpenAIConv, hasFromOpenAI := fromOpenAI[toFormat]

	if !hasToOpenAI || !hasFromOpenAI {
		return nil
	}

	// If we have a parsed chunk, convert directly
	if event.Chunk != nil {
		return fromOpenAIConv(event.Chunk)
	}

	// Otherwise, convert via OpenAI intermediate
	if event.Data != nil {
		return convertViaOpenAI(event.Data, toOpenAIConv, func(c *message.StreamChunk) []byte {
			// For anthropic target, need special handling
			if toFormat == "anthropic" {
				return joinAnthropicEvents(OpenAISSEToAnthropicSSE(c))
			}
			return fromOpenAI[toFormat](c)
		})
	}

	return nil
}

// joinAnthropicEvents concatenates multiple Anthropic SSE events
func joinAnthropicEvents(events []string) []byte {
	if len(events) == 0 {
		return nil
	}
	var sb strings.Builder
	for _, e := range events {
		sb.WriteString(e)
	}
	return []byte(sb.String())
}

// convertViaOpenAI converts SSE format via OpenAI intermediate
func convertViaOpenAI(data []byte, toOpenAI func([]byte) []byte, fromOpenAI func(*message.StreamChunk) []byte) []byte {
	openaiData := toOpenAI(data)
	if openaiData == nil {
		return nil
	}

	dataStr := strings.TrimPrefix(string(openaiData), "data: ")
	dataStr = strings.TrimSuffix(dataStr, "\n\n")

	var chunk message.StreamChunk
	if err := json.Unmarshal([]byte(dataStr), &chunk); err != nil {
		return nil
	}

	return fromOpenAI(&chunk)
}

// convertToAnthropic converts SSE format to Anthropic via OpenAI intermediate
func convertToAnthropic(data []byte, toOpenAI func([]byte) []byte) []byte {
	openaiData := toOpenAI(data)
	if openaiData == nil {
		return nil
	}

	dataStr := strings.TrimPrefix(string(openaiData), "data: ")
	dataStr = strings.TrimSuffix(dataStr, "\n\n")

	var chunk message.StreamChunk
	if err := json.Unmarshal([]byte(dataStr), &chunk); err != nil {
		return nil
	}

	return joinAnthropicEvents(OpenAISSEToAnthropicSSE(&chunk))
}

func FormatSSEOutput(event SSEEvent, targetFormat string) []byte {
	switch event.Type {
	case SSETypeDone:
		return []byte("data: [DONE]\n\n")
	case SSETypeEvent:
		return []byte(string(event.Data) + "\n\n")
	case SSETypeData:
		return []byte("data: " + string(event.Data) + "\n\n")
	}
	return nil
}