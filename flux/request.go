package flux

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/tokzone/fluxcore/errors"
	"github.com/tokzone/fluxcore/internal/translate"
	"github.com/tokzone/fluxcore/message"
	"github.com/tokzone/fluxcore/provider"
)

// Do sends a chat completion request with automatic retry and failover.
// It handles endpoint selection, health checking, and protocol translation.
// Returns the response body in the input protocol format, usage statistics, or an error.
func (c *Client) Do(ctx context.Context, rawReq []byte, inputProtocol provider.Protocol) ([]byte, *message.Usage, error) {
	req, err := parseRequest(rawReq, inputProtocol)
	if err != nil {
		return nil, nil, fmt.Errorf("parse request: %w", err)
	}

	var lastErr error
	retryMax := c.RetryMax()

	for retry := 0; retry <= retryMax; retry++ {
		e := c.Next()
		if e == nil {
			break
		}

		if retry > 0 {
			backoff := backoffWithJitter(retry)
			select {
			case <-time.After(backoff):
			case <-ctx.Done():
				return nil, nil, ctx.Err()
			}
		}

		start := time.Now()
		resp, usage, err := c.doWithParsedRequest(ctx, e, req, inputProtocol)
		latencyMs := int(time.Since(start).Milliseconds())

		if err == nil {
			c.Feedback(e, nil, latencyMs)
			return resp, usage, nil
		}

		lastErr = err
		c.Feedback(e, err, 0)

		// Don't retry non-retryable errors (auth, invalid request, etc.)
		if !errors.IsRetryable(err) {
			break
		}
	}

	if lastErr != nil {
		return nil, nil, lastErr
	}
	return nil, nil, fmt.Errorf("no available endpoints")
}

func (c *Client) doWithParsedRequest(ctx context.Context, ue *UserEndpoint, req *message.MessageRequest, inputProtocol provider.Protocol) ([]byte, *message.Usage, error) {
	targetProtocol := ue.Protocol()
	reqBody, err := translateRequest(req, targetProtocol)
	if err != nil {
		return nil, nil, fmt.Errorf("convert request: %w", err)
	}

	respBody, err := transport(ctx, ue, reqBody)
	if err != nil {
		return nil, nil, err
	}

	resp, err := translateResponse(respBody, targetProtocol)
	if err != nil {
		return nil, nil, fmt.Errorf("convert response: %w", err)
	}

	usage := &message.Usage{
		IsAccurate:   resp.Usage != nil && resp.Usage.IsAccurate,
		InputTokens:  resp.Usage.InputTokens,
		OutputTokens: resp.Usage.OutputTokens,
	}

	output, err := translateOutput(resp, inputProtocol)
	if err != nil {
		return nil, nil, fmt.Errorf("convert output: %w", err)
	}

	return output, usage, nil
}

const defaultWrappedChannelBuffer = 100

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
// Thread-safe: Ch can be read concurrently, Usage() and Error() and Close() are safe to call from any goroutine.
type StreamResult struct {
	Ch     chan []byte
	Usage  func() *message.Usage
	Error  func() error // Returns the first error encountered during streaming
	cancel context.CancelFunc
}

// DoStream sends a streaming chat completion request with automatic retry and failover.
// Returns a StreamResult with a channel for SSE chunks and a Usage function for final stats.
// Caller must call StreamResult.Close() to release resources when done.
func (c *Client) DoStream(ctx context.Context, rawReq []byte, inputProtocol provider.Protocol) (*StreamResult, error) {
	req, err := parseRequest(rawReq, inputProtocol)
	if err != nil {
		return nil, fmt.Errorf("parse request: %w", err)
	}
	req = req.WithStream(true)

	var lastErr error
	retryMax := c.RetryMax()

	for retry := 0; retry <= retryMax; retry++ {
		e := c.Next()
		if e == nil {
			break
		}

		if retry > 0 {
			backoff := backoffWithJitter(retry)
			select {
			case <-time.After(backoff):
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}

		start := time.Now()
		result, err := c.doStreamWithParsedRequest(ctx, e, req, inputProtocol)

		if err == nil {
			// Wrap the result channel to track success on completion
			wrappedCh := make(chan []byte, defaultWrappedChannelBuffer)
			wrappedResult := &StreamResult{
				Ch:     wrappedCh,
				Usage:  result.Usage,
				Error:  result.Error,
				cancel: result.cancel,
			}

			go func() {
				defer func() {
					if r := recover(); r != nil {
						log.Printf("[fluxcore] stream wrapper panic recovered: %v", r)
					}
				}()
				defer close(wrappedCh)
				defer result.Close()

				for {
					select {
					case chunk, ok := <-result.Ch:
						if !ok {
							// Stream completed - check if there was an error
							latencyMs := int(time.Since(start).Milliseconds())
							if result.Error() == nil {
								c.Feedback(e, nil, latencyMs)
							} else {
								c.Feedback(e, result.Error(), 0)
							}
							return
						}
						// Non-blocking send to avoid goroutine leak
						select {
						case wrappedCh <- chunk:
						case <-ctx.Done():
							c.Feedback(e, ctx.Err(), 0)
							return
						}
					case <-ctx.Done():
						c.Feedback(e, ctx.Err(), 0)
						return
					}
				}
			}()

			return wrappedResult, nil
		}

		lastErr = err
		c.Feedback(e, err, 0)

		// Don't retry non-retryable errors
		if !errors.IsRetryable(err) {
			break
		}
	}

	if lastErr != nil {
		return nil, lastErr
	}
	return nil, fmt.Errorf("no available targets")
}

func (c *Client) doStreamWithParsedRequest(ctx context.Context, ue *UserEndpoint, req *message.MessageRequest, inputProtocol provider.Protocol) (*StreamResult, error) {
	start := time.Now()

	targetProtocol := ue.Protocol()
	reqBody, err := translateRequest(req, targetProtocol)
	if err != nil {
		return nil, fmt.Errorf("convert request: %w", err)
	}

	respBody, cancel, err := streamTransport(ctx, ue, reqBody)
	if err != nil {
		return nil, err
	}

	ch := make(chan []byte, translate.GetSSEConfig().ChannelBuffer)
	usageData := &atomicUsage{}
	var firstError atomic.Pointer[error]

	eventCh := translate.ParseSSEStream(ctx, respBody, targetProtocol.String(), start)

	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[fluxcore] SSE processor panic recovered: %v", r)
			}
		}()
		defer close(ch)
		for result := range eventCh {
			if result.Error != nil {
				// Store first error only (atomic CAS)
				firstError.CompareAndSwap(nil, &result.Error)
				continue
			}

			if result.Usage != nil {
				usageData.Set(result.Usage)
			}

			if result.Event.Type == translate.SSETypeDone {
				continue
			}

			if targetProtocol != inputProtocol {
				converted := translate.ConvertSSEEvent(result.Event, targetProtocol.String(), inputProtocol.String())
				if converted != nil {
					// Non-blocking send to avoid goroutine leak
					select {
					case ch <- converted:
					case <-ctx.Done():
						return
					}
				}
			} else {
				output := translate.FormatSSEOutput(result.Event, inputProtocol.String())
				if output != nil {
					// Non-blocking send to avoid goroutine leak
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

func streamTransport(ctx context.Context, ue *UserEndpoint, body []byte) (io.ReadCloser, context.CancelFunc, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)

	req, err := http.NewRequestWithContext(ctx, "POST", buildURL(ue, true), bytes.NewReader(body))
	if err != nil {
		cancel()
		return nil, nil, err
	}
	setHeaders(req, ue, true)

	resp, err := sharedClient.Do(req)
	if err != nil {
		cancel()
		return nil, nil, errors.ClassifyNetError(err)
	}

	if resp.StatusCode >= 400 {
		cancel()
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, defaultErrorBodyLimit))
		resp.Body.Close()
		return nil, nil, errors.ClassifyHTTPError(resp.StatusCode, string(respBody))
	}

	return resp.Body, cancel, nil
}

// Close releases resources for the StreamResult.
func (s *StreamResult) Close() {
	if s.cancel != nil {
		s.cancel()
	}
}
