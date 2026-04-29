package translate

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log"
	"sync"
	"time"

	"github.com/tokzone/fluxcore/message"
)

// chunkParser parses SSE chunk data to StreamChunk
type chunkParser func(data []byte) (*message.StreamChunk, error)

var chunkParsers = make(map[string]chunkParser)

// registerChunkParser registers a parser for a format
func registerChunkParser(format string, parser chunkParser) {
	chunkParsers[format] = parser
}

// getChunkParser returns the parser for a format, or nil for OpenAI format
func getChunkParser(format string) chunkParser {
	return chunkParsers[format]
}

// SSE event type constants
const (
	sseTypeData  = "data"
	sseTypeEvent = "event"
	SSETypeDone  = "done"
)

// SSEConfig holds SSE configuration.
type SSEConfig struct {
	BufferSize         int // Read buffer size (default: 4096)
	ChannelBuffer      int // Event channel buffer size (default: 100)
	MaxAccumulatedSize int // Max accumulated data size (default: 1MB, prevents unbounded growth)
}

var sseConfig = SSEConfig{
	BufferSize:         4096,
	ChannelBuffer:      100,
	MaxAccumulatedSize: 1024 * 1024, // 1MB
}

var sseConfigMu sync.RWMutex

func GetSSEConfig() SSEConfig {
	sseConfigMu.RLock()
	defer sseConfigMu.RUnlock()
	return sseConfig
}

// SSE byte prefixes (package-level constants, avoid allocation per call)
var (
	sseDataPrefix  = []byte("data: ")
	sseEventPrefix = []byte("event: ")
	sseDoneMarker  = []byte("[DONE]")
	sseSuffix      = []byte("\n\n")
)

type sseEvent struct {
	Type   string               // sseTypeData, sseTypeEvent, SSETypeDone
	Data   []byte               // Raw data payload
	Chunk  *message.StreamChunk // Parsed chunk (for data events)
	Format string               // Source format
}

type sseParseResult struct {
	Event sseEvent
	Usage *message.Usage
	Error error
}

func ParseSSEStream(ctx context.Context, reader io.ReadCloser, format string, startTime time.Time) chan sseParseResult {
	sseConfigMu.RLock()
	bufSize := sseConfig.BufferSize
	chBuf := sseConfig.ChannelBuffer
	maxAccum := sseConfig.MaxAccumulatedSize
	sseConfigMu.RUnlock()

	ch := make(chan sseParseResult, chBuf)

	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[fluxcore] SSE parser panic recovered: %v", r)
			}
		}()
		defer close(ch)
		defer reader.Close()

		buf := make([]byte, bufSize)
		accumulated := &bytes.Buffer{}
		usageData := &message.Usage{}

		for {
			select {
			case <-ctx.Done():
				usageData.LatencyMs = int(time.Since(startTime).Milliseconds())
				ch <- sseParseResult{Usage: usageData, Error: ctx.Err()}
				return
			default:
			}

			n, readErr := reader.Read(buf)
			if n > 0 {
				accumulated.Write(buf[:n])

				// Prevent unbounded growth: process data before resetting
				if accumulated.Len() > maxAccum {
					log.Printf("[fluxcore] SSE accumulated data exceeds limit")
					data := accumulated.Bytes()
					lines := bytes.Split(data, sseSuffix)
					for i := 0; i < len(lines)-1; i++ {
						result := parseSSELine(lines[i], format, startTime, usageData)
						if result.Event.Type != "" {
							ch <- result
						}
					}
					accumulated.Reset()
					if len(lines) > 0 {
						accumulated.Write(lines[len(lines)-1])
					}
					continue
				}

				data := accumulated.Bytes()

				lines := bytes.Split(data, sseSuffix)
				accumulated.Reset()
				accumulated.Write(lines[len(lines)-1])

				for i := 0; i < len(lines)-1; i++ {
					result := parseSSELine(lines[i], format, startTime, usageData)
					if result.Event.Type != "" {
						ch <- result
					}
				}
			}

			if readErr != nil {
				if readErr != io.EOF {
					log.Printf("[fluxcore] stream read error: %v", readErr)
					ch <- sseParseResult{Error: readErr}
				}
				usageData.LatencyMs = int(time.Since(startTime).Milliseconds())
				ch <- sseParseResult{Usage: usageData}
				return
			}
		}
	}()

	return ch
}

func parseSSELine(line []byte, format string, startTime time.Time, usageData *message.Usage) sseParseResult {
	result := sseParseResult{}

	if bytes.HasPrefix(line, sseDataPrefix) {
		data := bytes.TrimPrefix(line, sseDataPrefix)

		if bytes.Equal(data, sseDoneMarker) {
			result.Event = sseEvent{
				Type: SSETypeDone,
				Data: sseDoneMarker,
			}
			usageData.LatencyMs = int(time.Since(startTime).Milliseconds())
			return result
		}

		parser := getChunkParser(format)
		if parser != nil {
			chunk, err := parser(data)
			if err != nil {
				log.Printf("[fluxcore] malformed %s SSE chunk: %v", format, err)
				result.Error = err
				return result
			}
			if chunk == nil {
				return result
			}
			result.Event = sseEvent{
				Type:   sseTypeData,
				Data:   data,
				Chunk:  chunk,
				Format: format,
			}
			updateUsage(usageData, chunk.Usage)
		} else {
			var chunk message.StreamChunk
			if err := json.Unmarshal(data, &chunk); err != nil {
				log.Printf("[fluxcore] malformed SSE chunk: %v", err)
				result.Error = err
				return result
			}
			result.Event = sseEvent{
				Type:   sseTypeData,
				Data:   data,
				Chunk:  &chunk,
				Format: format,
			}
			updateUsage(usageData, chunk.Usage)
		}
	} else if bytes.HasPrefix(line, sseEventPrefix) {
		result.Event = sseEvent{
			Type: sseTypeEvent,
			Data: line,
		}
	}

	return result
}

func updateUsage(usageData *message.Usage, usage *message.Usage) {
	if usage != nil {
		usageData.InputTokens = usage.InputTokens
		usageData.OutputTokens = usage.OutputTokens
		usageData.IsAccurate = true
	}
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

func ConvertSSEEvent(event sseEvent, fromFormat, toFormat string) []byte {
	if fromFormat == toFormat {
		return FormatSSEOutput(event, toFormat)
	}

	if event.Type != sseTypeData {
		return nil
	}

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
	var buf bytes.Buffer
	for _, e := range events {
		buf.WriteString(e)
	}
	return buf.Bytes()
}

func convertViaOpenAI(inputData []byte, toOpenAI func([]byte) []byte, fromOpenAI func(*message.StreamChunk) []byte) []byte {
	openaiData := toOpenAI(inputData)
	if openaiData == nil {
		return nil
	}

	data := bytes.TrimPrefix(openaiData, sseDataPrefix)
	data = bytes.TrimSuffix(data, sseSuffix)

	var chunk message.StreamChunk
	if err := json.Unmarshal(data, &chunk); err != nil {
		return nil
	}

	return fromOpenAI(&chunk)
}

func FormatSSEOutput(event sseEvent, targetFormat string) []byte {
	switch event.Type {
	case SSETypeDone:
		buf := make([]byte, 0, len(sseDataPrefix)+len(sseDoneMarker)+len(sseSuffix))
		buf = append(buf, sseDataPrefix...)
		buf = append(buf, sseDoneMarker...)
		buf = append(buf, sseSuffix...)
		return buf
	case sseTypeEvent:
		buf := make([]byte, 0, len(event.Data)+len(sseSuffix))
		buf = append(buf, event.Data...)
		buf = append(buf, sseSuffix...)
		return buf
	case sseTypeData:
		buf := make([]byte, 0, len(sseDataPrefix)+len(event.Data)+len(sseSuffix))
		buf = append(buf, sseDataPrefix...)
		buf = append(buf, event.Data...)
		buf = append(buf, sseSuffix...)
		return buf
	}
	return nil
}
