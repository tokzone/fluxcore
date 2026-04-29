package fluxcore

import (
	"bytes"
	"context"
	"encoding/json"
	stderrors "errors"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/tokzone/fluxcore/errors"
	"github.com/tokzone/fluxcore/internal/translate"
	"github.com/tokzone/fluxcore/message"
)

// ──── Router ────

// Router is a domain service that executes requests against Route tables.
// It handles protocol translation, HTTP transport, retry with backoff,
// and two-layer health feedback (network → ServiceEndpoint, model → Route).
type Router struct {
	inputProto Protocol
	httpClient *http.Client
}

// RouterOption configures a Router.
type RouterOption func(*Router)

// WithHTTPClient sets a custom HTTP client.
func WithHTTPClient(client *http.Client) RouterOption {
	return func(r *Router) {
		if client != nil {
			r.httpClient = client
		}
	}
}

// NewRouter creates a new Router for the given input protocol.
func NewRouter(inputProto Protocol, opts ...RouterOption) *Router {
	r := &Router{
		inputProto: inputProto,
		httpClient: sharedHTTPClient,
	}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

// ──── Do — single request ────

// Do executes a single request through a Route.
// When input and target protocols match, the raw body is passed through directly.
func (r *Router) Do(ctx context.Context, route *Route, targetProto Protocol, rawReq []byte) ([]byte, *message.Usage, error) {
	start := time.Now()

	se := route.SvcEP()
	baseURL := se.Service().BaseURLFor(targetProto)
	credential := route.Desc().Credential
	model := string(route.Desc().Model)

	// Request direction: passthrough when same protocol
	passthrough := r.inputProto == targetProto
	var reqBody []byte
	if passthrough {
		reqBody = rawReq
	} else {
		req, err := parseRequestBody(rawReq, r.inputProto)
		if err != nil {
			return nil, nil, fmt.Errorf("parse request: %w", err)
		}
		reqBody, err = translateToProtocol(req, targetProto)
		if err != nil {
			return nil, nil, fmt.Errorf("translate request: %w", err)
		}
	}

	// HTTP transport
	respBody, err := r.httpDo(ctx, targetProto, baseURL, model, credential, reqBody)
	latencyMs := int(time.Since(start).Milliseconds())

	if err != nil {
		r.feedbackFailure(route, err)
		return nil, nil, err
	}

	r.feedbackSuccess(route, latencyMs)

	// Response direction: passthrough when same protocol
	if passthrough {
		return respBody, extractUsage(respBody, targetProto), nil
	}

	resp, err := translateFromProtocol(respBody, targetProto)
	if err != nil {
		return nil, nil, fmt.Errorf("translate response: %w", err)
	}

	usage := &message.Usage{
		InputTokens:  resp.Usage.InputTokens,
		OutputTokens: resp.Usage.OutputTokens,
		IsAccurate:   resp.Usage != nil && resp.Usage.IsAccurate,
	}

	output, err := translateOutputToProtocol(resp, r.inputProto)
	if err != nil {
		return nil, nil, fmt.Errorf("translate output: %w", err)
	}

	return output, usage, nil
}

// ──── Stream — single stream request ────

// StreamResult holds the result of a streaming request.
type StreamResult struct {
	Ch     chan []byte
	Usage  func() *message.Usage
	Error  func() error
	cancel context.CancelFunc
}

// Close releases resources held by the StreamResult.
func (s *StreamResult) Close() {
	if s.cancel != nil {
		s.cancel()
	}
}

// Stream executes a single streaming request through a Route.
// The returned error is only for pre-stream failures; mid-stream errors
// are reported via StreamResult.Error().
func (r *Router) Stream(ctx context.Context, route *Route, targetProto Protocol, rawReq []byte) (*StreamResult, error) {
	start := time.Now()

	se := route.SvcEP()
	baseURL := se.Service().BaseURLFor(targetProto)
	credential := route.Desc().Credential
	model := string(route.Desc().Model)

	var reqBody []byte
	if r.inputProto == targetProto {
		reqBody = rawReq
	} else {
		req, err := parseRequestBody(rawReq, r.inputProto)
		if err != nil {
			return nil, fmt.Errorf("parse request: %w", err)
		}
		req.Stream = true
		reqBody, err = translateToProtocol(req, targetProto)
		if err != nil {
			return nil, fmt.Errorf("translate request: %w", err)
		}
	}

	respBody, cancel, err := r.httpStream(ctx, targetProto, baseURL, model, credential, reqBody)
	if err != nil {
		r.feedbackFailure(route, err)
		return nil, err
	}

	ch := make(chan []byte, translate.GetSSEConfig().ChannelBuffer)
	usageData := &atomicUsage{}
	var firstError atomic.Pointer[error]

	eventCh := translate.ParseSSEStream(ctx, respBody, targetProto.String(), start)

	go func() {
		defer func() {
			if rc := recover(); rc != nil {
				log.Printf("[fluxcore] SSE processor panic recovered: %v", rc)
			}
		}()
		defer close(ch)
		for result := range eventCh {
			if result.Error != nil {
				firstError.CompareAndSwap(nil, &result.Error)
				continue
			}
			if result.Usage != nil {
				usageData.Set(result.Usage)
			}
			if result.Event.Type == translate.SSETypeDone {
				continue
			}
			if targetProto != r.inputProto {
				converted := translate.ConvertSSEEvent(result.Event, targetProto.String(), r.inputProto.String())
				if converted != nil {
					select {
					case ch <- converted:
					case <-ctx.Done():
						return
					}
				}
			} else {
				output := translate.FormatSSEOutput(result.Event, r.inputProto.String())
				if output != nil {
					select {
					case ch <- output:
					case <-ctx.Done():
						return
					}
				}
			}
		}
	}()

	return &StreamResult{
		Ch:    ch,
		Usage: usageData.Get,
		Error: func() error {
			if p := firstError.Load(); p != nil {
				return *p
			}
			return nil
		},
		cancel: cancel,
	}, nil
}

// ──── Execute — retry + failover ────

// Execute retries with failover through the RouteTable.
// Returns the selected Route, response body, usage, and any error.
func (r *Router) Execute(ctx context.Context, table *RouteTable, body []byte, maxRetry int) (*Route, []byte, *message.Usage, error) {

	var lastErr error
	for attempt := 0; attempt <= maxRetry; attempt++ {
		route, targetProto := table.Select()
		if route == nil {
			break
		}

		if attempt > 0 {
			backoff := backoffWithJitter(attempt)
			select {
			case <-time.After(backoff):
			case <-ctx.Done():
				return nil, nil, nil, ctx.Err()
			}
		}

		resp, usage, err := r.Do(ctx, route, targetProto, body)
		if err == nil {
			return route, resp, usage, nil
		}

		lastErr = err
		if !errors.IsRetryable(err) {
			break
		}
	}

	if lastErr != nil {
		return nil, nil, nil, lastErr
	}
	return nil, nil, nil, errors.Wrap(errors.CodeNoEndpoint, "no available route", nil)
}

// ──── ExecuteStream — retry + failover for streams ────

// ExecuteStream retries streaming with failover through the RouteTable.
func (r *Router) ExecuteStream(ctx context.Context, table *RouteTable, body []byte, maxRetry int) (*Route, *StreamResult, error) {
	var lastErr error
	for attempt := 0; attempt <= maxRetry; attempt++ {
		route, targetProto := table.Select()
		if route == nil {
			break
		}

		if attempt > 0 {
			backoff := backoffWithJitter(attempt)
			select {
			case <-time.After(backoff):
			case <-ctx.Done():
				return nil, nil, ctx.Err()
			}
		}

		result, err := r.Stream(ctx, route, targetProto, body)
		if err == nil {
			// Wrap to track health feedback on stream completion
			wrapped := r.wrapStreamResult(ctx, route, result)
			return route, wrapped, nil
		}

		lastErr = err
		if !errors.IsRetryable(err) {
			break
		}
	}

	if lastErr != nil {
		return nil, nil, lastErr
	}
	return nil, nil, errors.Wrap(errors.CodeNoEndpoint, "no available route", nil)
}

// wrapStreamResult wraps a raw StreamResult to provide health feedback on completion.
func (r *Router) wrapStreamResult(ctx context.Context, route *Route, raw *StreamResult) *StreamResult {
	start := time.Now()
	wrappedCh := make(chan []byte, streamChannelBuffer)

	go func() {
		defer func() {
			if rc := recover(); rc != nil {
				log.Printf("[fluxcore] stream wrapper panic recovered: %v", rc)
			}
		}()
		defer close(wrappedCh)
		defer raw.Close()

		for {
			select {
			case chunk, ok := <-raw.Ch:
				if !ok {
					latencyMs := int(time.Since(start).Milliseconds())
					if raw.Error() == nil {
						r.feedbackSuccess(route, latencyMs)
					} else {
						r.feedbackFailure(route, raw.Error())
					}
					return
				}
				select {
				case wrappedCh <- chunk:
				case <-ctx.Done():
					r.feedbackFailure(route, ctx.Err())
					return
				}
			case <-ctx.Done():
				r.feedbackFailure(route, ctx.Err())
				return
			}
		}
	}()

	return &StreamResult{
		Ch:     wrappedCh,
		Usage:  raw.Usage,
		Error:  raw.Error,
		cancel: raw.cancel,
	}
}

// ──── Health feedback ────

func (r *Router) feedbackSuccess(route *Route, latencyMs int) {
	route.MarkSuccess()
	route.SvcEP().MarkSuccess()
	route.UpdateLatency(latencyMs)
	route.SvcEP().UpdateLatency(latencyMs)
}

func (r *Router) feedbackFailure(route *Route, err error) {
	if isNetworkError(err) {
		route.SvcEP().MarkNetworkFailure()
	} else if isModelError(err) {
		route.MarkModelFailure()
	}
	// 4xx non-429 errors do not trip any circuit breaker
}

func isNetworkError(err error) bool {
	var classified *errors.ClassifiedError
	if stderrors.As(err, &classified) {
		switch classified.Code {
		case errors.CodeNetworkError, errors.CodeDNSError, errors.CodeTimeout:
			return true
		}
	}
	return false
}

func isModelError(err error) bool {
	var classified *errors.ClassifiedError
	if stderrors.As(err, &classified) {
		switch classified.Code {
		case errors.CodeRateLimit, errors.CodeServerError, errors.CodeModelError:
			return true
		}
	}
	return false
}

// ──── HTTP transport ────

const (
	defaultHTTPTimeout   = 30 * time.Second
	maxIdleConns         = 100
	maxIdleConnsPerHost  = 10
	idleConnTimeout      = 90 * time.Second
	errorBodyLimit       = 4096
	responseBodyLimit    = 10 * 1024 * 1024
	streamChannelBuffer  = 100
)

var sharedHTTPClient = &http.Client{
	Timeout: defaultHTTPTimeout,
	Transport: &http.Transport{
		MaxIdleConns:        maxIdleConns,
		MaxIdleConnsPerHost: maxIdleConnsPerHost,
		IdleConnTimeout:     idleConnTimeout,
	},
}

func (r *Router) httpDo(ctx context.Context, targetProto Protocol, baseURL, model, credential string, body []byte) ([]byte, error) {
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, defaultHTTPTimeout)
		defer cancel()
	}

	url := buildRequestURL(targetProto, baseURL, model, false)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	setRequestHeaders(req, targetProto, credential, false)

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return nil, errors.ClassifyNetError(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, errorBodyLimit))
		return nil, errors.ClassifyHTTPError(resp.StatusCode, string(respBody))
	}

	return io.ReadAll(io.LimitReader(resp.Body, responseBodyLimit))
}

func (r *Router) httpStream(ctx context.Context, targetProto Protocol, baseURL, model, credential string, body []byte) (io.ReadCloser, context.CancelFunc, error) {
	cancel := func() {}

	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancelFn context.CancelFunc
		ctx, cancelFn = context.WithTimeout(ctx, defaultHTTPTimeout)
		cancel = cancelFn
	}

	url := buildRequestURL(targetProto, baseURL, model, true)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		cancel()
		return nil, nil, err
	}
	setRequestHeaders(req, targetProto, credential, true)

	// Bypass Client.Timeout for streaming — context deadline controls the lifecycle.
	streamClient := *r.httpClient
	streamClient.Timeout = 0
	resp, err := streamClient.Do(req)
	if err != nil {
		cancel()
		return nil, nil, errors.ClassifyNetError(err)
	}

	if resp.StatusCode >= 400 {
		cancel()
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, errorBodyLimit))
		resp.Body.Close()
		return nil, nil, errors.ClassifyHTTPError(resp.StatusCode, string(respBody))
	}

	return resp.Body, cancel, nil
}

func buildRequestURL(targetProto Protocol, baseURL, model string, stream bool) string {
	switch targetProto {
	case ProtocolGemini:
		if stream {
			return baseURL + "/v1/models/" + model + ":streamGenerateContent?alt=sse"
		}
		return baseURL + "/v1/models/" + model + ":generateContent"
	case ProtocolAnthropic:
		return baseURL + "/v1/messages"
	case ProtocolCohere:
		return baseURL + "/v1/chat"
	default:
		return baseURL + "/v1/chat/completions"
	}
}

func setRequestHeaders(req *http.Request, targetProto Protocol, credential string, stream bool) {
	req.Header.Set("Content-Type", "application/json")
	if stream {
		req.Header.Set("Accept", "text/event-stream")
	}

	switch targetProto {
	case ProtocolGemini:
		req.Header.Set("x-goog-api-key", credential)
	case ProtocolAnthropic:
		req.Header.Set("anthropic-version", "2023-06-01")
		req.Header.Set("x-api-key", credential)
	case ProtocolCohere:
		req.Header.Set("Authorization", "Bearer "+credential)
	default:
		req.Header.Set("Authorization", "Bearer "+credential)
	}
}

// ──── Protocol translation ────

func translateToProtocol(req *message.MessageRequest, targetProto Protocol) ([]byte, error) {
	switch targetProto {
	case ProtocolAnthropic:
		return translate.MessageRequestToAnthropic(req)
	case ProtocolGemini:
		return translate.MessageRequestToGemini(req)
	case ProtocolCohere:
		return translate.MessageRequestToCohere(req)
	default:
		return json.Marshal(req)
	}
}

func translateFromProtocol(respBody []byte, proto Protocol) (*message.MessageResponse, error) {
	switch proto {
	case ProtocolAnthropic:
		return translate.AnthropicResponseToMessageResponse(respBody)
	case ProtocolGemini:
		return translate.GeminiResponseToMessageResponse(respBody)
	case ProtocolCohere:
		return translate.CohereResponseToMessageResponse(respBody)
	default:
		return message.ParseResponse(respBody)
	}
}

func translateOutputToProtocol(resp *message.MessageResponse, proto Protocol) ([]byte, error) {
	switch proto {
	case ProtocolAnthropic:
		return translate.MessageResponseToAnthropic(resp)
	case ProtocolGemini:
		return translate.MessageResponseToGemini(resp)
	case ProtocolCohere:
		return translate.MessageResponseToCohere(resp)
	default:
		return json.Marshal(resp)
	}
}

func parseRequestBody(rawReq []byte, proto Protocol) (*message.MessageRequest, error) {
	switch proto {
	case ProtocolAnthropic:
		return translate.AnthropicToMessageRequest(bytes.NewReader(rawReq))
	case ProtocolGemini:
		return translate.GeminiToMessageRequest(bytes.NewReader(rawReq))
	case ProtocolCohere:
		return translate.CohereToMessageRequest(bytes.NewReader(rawReq))
	default:
		return message.ParseRequest(rawReq)
	}
}

// ──── Backoff ────

const (
	defaultBaseBackoff = 100 * time.Millisecond
	defaultMaxBackoff  = 5 * time.Second
)

func backoffWithJitter(attempt int) time.Duration {
	backoff := defaultBaseBackoff << uint(attempt-1)
	if backoff > defaultMaxBackoff {
		backoff = defaultMaxBackoff
	}
	return time.Duration(rand.Float64() * float64(backoff))
}

// ──── Usage extraction ────

// extractUsage extracts token usage from a raw response body without full protocol translation.
func extractUsage(body []byte, proto Protocol) *message.Usage {
	switch proto {
	case ProtocolAnthropic:
		var v struct {
			Usage struct {
				InputTokens  int `json:"input_tokens"`
				OutputTokens int `json:"output_tokens"`
			} `json:"usage"`
		}
		if json.Unmarshal(body, &v) == nil && v.Usage.InputTokens+v.Usage.OutputTokens > 0 {
			return &message.Usage{InputTokens: v.Usage.InputTokens, OutputTokens: v.Usage.OutputTokens, IsAccurate: true}
		}
	case ProtocolGemini:
		var v struct {
			UsageMetadata struct {
				PromptTokenCount     int `json:"promptTokenCount"`
				CandidatesTokenCount int `json:"candidatesTokenCount"`
			} `json:"usageMetadata"`
		}
		if json.Unmarshal(body, &v) == nil && v.UsageMetadata.PromptTokenCount+v.UsageMetadata.CandidatesTokenCount > 0 {
			return &message.Usage{InputTokens: v.UsageMetadata.PromptTokenCount, OutputTokens: v.UsageMetadata.CandidatesTokenCount, IsAccurate: true}
		}
	case ProtocolCohere:
		var v struct {
			Meta struct {
				BilledUnits struct {
					InputTokens  int `json:"input_tokens"`
					OutputTokens int `json:"output_tokens"`
				} `json:"billed_units"`
			} `json:"meta"`
		}
		if json.Unmarshal(body, &v) == nil && v.Meta.BilledUnits.InputTokens+v.Meta.BilledUnits.OutputTokens > 0 {
			return &message.Usage{InputTokens: v.Meta.BilledUnits.InputTokens, OutputTokens: v.Meta.BilledUnits.OutputTokens, IsAccurate: true}
		}
		var v2 struct {
			TokenCount struct {
				InputTokens  int `json:"input_tokens"`
				OutputTokens int `json:"output_tokens"`
			} `json:"token_count"`
		}
		if json.Unmarshal(body, &v2) == nil && v2.TokenCount.InputTokens+v2.TokenCount.OutputTokens > 0 {
			return &message.Usage{InputTokens: v2.TokenCount.InputTokens, OutputTokens: v2.TokenCount.OutputTokens, IsAccurate: true}
		}
	default: // OpenAI: try real field names first, then IR-compatible names
		var v struct {
			Usage struct {
				PromptTokens     int `json:"prompt_tokens"`
				CompletionTokens int `json:"completion_tokens"`
			} `json:"usage"`
		}
		if json.Unmarshal(body, &v) == nil && v.Usage.PromptTokens+v.Usage.CompletionTokens > 0 {
			return &message.Usage{InputTokens: v.Usage.PromptTokens, OutputTokens: v.Usage.CompletionTokens, IsAccurate: true}
		}
		var vIR struct {
			Usage struct {
				InputTokens  int `json:"input_tokens"`
				OutputTokens int `json:"output_tokens"`
			} `json:"usage"`
		}
		if json.Unmarshal(body, &vIR) == nil && vIR.Usage.InputTokens+vIR.Usage.OutputTokens > 0 {
			return &message.Usage{InputTokens: vIR.Usage.InputTokens, OutputTokens: vIR.Usage.OutputTokens, IsAccurate: true}
		}
	}
	return nil
}

// ──── Atomic usage ────

type atomicUsage struct {
	inputTokens  atomic.Int64
	outputTokens atomic.Int64
	isAccurate   atomic.Bool
}

func (u *atomicUsage) Get() *message.Usage {
	return &message.Usage{
		InputTokens:  int(u.inputTokens.Load()),
		OutputTokens: int(u.outputTokens.Load()),
		IsAccurate:   u.isAccurate.Load(),
	}
}

func (u *atomicUsage) Set(usage *message.Usage) {
	u.inputTokens.Store(int64(usage.InputTokens))
	u.outputTokens.Store(int64(usage.OutputTokens))
	u.isAccurate.Store(usage.IsAccurate)
}
