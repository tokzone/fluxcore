package call

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/tokzone/fluxcore/errors"
	"github.com/tokzone/fluxcore/message"
	"github.com/tokzone/fluxcore/routing"
	"github.com/tokzone/fluxcore/internal/translate"
)

// atomicUsage provides thread-safe access to usage statistics.
type atomicUsage struct {
	inputTokens  atomic.Int64
	outputTokens atomic.Int64
	latencyMs    atomic.Int64
	isAccurate   atomic.Bool
}

// Get returns a snapshot of the current usage values.
func (u *atomicUsage) Get() *message.Usage {
	return &message.Usage{
		InputTokens:  int(u.inputTokens.Load()),
		OutputTokens: int(u.outputTokens.Load()),
		LatencyMs:    int(u.latencyMs.Load()),
		IsAccurate:   u.isAccurate.Load(),
	}
}

// Set atomically updates all usage fields.
func (u *atomicUsage) Set(usage *message.Usage) {
	u.inputTokens.Store(int64(usage.InputTokens))
	u.outputTokens.Store(int64(usage.OutputTokens))
	u.latencyMs.Store(int64(usage.LatencyMs))
	u.isAccurate.Store(usage.IsAccurate)
}

// StreamResult holds the result of a streaming request.
// Thread-safe: Ch can be read concurrently, Usage() and Close() are safe to call from any goroutine.
type StreamResult struct {
	Ch     chan []byte
	Usage  func() *message.Usage
	cancel context.CancelFunc
}

// RequestStream sends a streaming chat completion request with automatic retry and failover.
// Returns a StreamResult with a channel for SSE chunks and a Usage function for final stats.
// Caller must call StreamResult.Close() to release resources when done.
func RequestStream(ctx context.Context, pool *routing.EndpointPool, rawReq []byte, inputProtocol routing.Protocol) (*StreamResult, error) {
	req, err := parseRequest(rawReq, inputProtocol)
	if err != nil {
		return nil, fmt.Errorf("parse request: %w", err)
	}
	req = req.WithStream(true)

	ep, err := selectEndpoint(pool)
	if err != nil {
		return nil, err
	}

	var lastErr error
	for retry := 0; retry <= pool.RetryMax(); retry++ {
		if retry > 0 {
			ep, err = selectEndpoint(pool)
			if err != nil {
				break
			}

			backoff := backoffWithJitter(retry)
			select {
			case <-time.After(backoff):
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}

		result, err := callStreamWithParsedRequest(ctx, ep, req, inputProtocol)
		if err == nil {
			wrappedCh := make(chan []byte, 100)
			go func() {
				defer close(wrappedCh)
				defer result.Close()
				for {
					select {
					case chunk, ok := <-result.Ch:
						if !ok {
							ep.MarkSuccess()
							return
						}
						wrappedCh <- chunk
					case <-ctx.Done():
						return
					}
				}
			}()

			return &StreamResult{
				Ch:     wrappedCh,
				Usage:  result.Usage,
				cancel: result.cancel,
			}, nil
		}

		lastErr = err
		pool.MarkFail(ep)

		// Don't retry non-retryable errors
		if !errors.IsRetryable(err) {
			break
		}
	}

	return nil, lastErr
}

// callStreamWithParsedRequest executes a stream request with pre-parsed MessageRequest
func callStreamWithParsedRequest(ctx context.Context, ep *routing.Endpoint, req *message.MessageRequest, inputProtocol routing.Protocol) (*StreamResult, error) {
	start := time.Now()

	reqBody, err := translateRequest(req, ep.Key.Protocol)
	if err != nil {
		return nil, fmt.Errorf("convert request: %w", err)
	}

	respBody, cancel, err := streamTransport(ctx, ep, reqBody)
	if err != nil {
		return nil, err
	}

	ch := make(chan []byte, translate.SSEChannelBuffer)
	usageData := &atomicUsage{}

	eventCh := translate.ParseSSEStream(ctx, respBody, ep.Key.Protocol.String(), start)

	go func() {
		defer close(ch)
		for result := range eventCh {
			if result.Error != nil {
				continue
			}

			if result.Usage != nil {
				usageData.Set(result.Usage)
			}

			if result.Event.Type == translate.SSETypeDone {
				continue
			}

			if ep.Key.Protocol != inputProtocol {
				converted := translate.ConvertSSEEvent(result.Event, ep.Key.Protocol.String(), inputProtocol.String())
				if converted != nil {
					ch <- converted
				}
			} else {
				output := translate.FormatSSEOutput(result.Event, inputProtocol.String())
				if output != nil {
					ch <- output
				}
			}
		}
	}()

	return &StreamResult{
		Ch:     ch,
		Usage:  usageData.Get,
		cancel: cancel,
	}, nil
}

func streamTransport(ctx context.Context, ep *routing.Endpoint, body []byte) (io.ReadCloser, context.CancelFunc, error) {
	ctx, cancel := context.WithTimeout(ctx, DefaultTimeout)

	req, err := http.NewRequestWithContext(ctx, "POST", buildURL(ep, true), bytes.NewReader(body))
	if err != nil {
		cancel()
		return nil, nil, err
	}
	setHeaders(req, ep, true)

	resp, err := sharedClient.Do(req)
	if err != nil {
		cancel()
		return nil, nil, errors.ClassifyNetError(err)
	}

	if resp.StatusCode >= 400 {
		cancel()
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		resp.Body.Close()
		return nil, nil, errors.ClassifyHTTPError(resp.StatusCode, string(respBody))
	}

	return resp.Body, cancel, nil
}

func (s *StreamResult) Close() {
	if s.cancel != nil {
		s.cancel()
	}
}