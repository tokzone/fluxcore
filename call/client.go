package call

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/tokzone/fluxcore/errors"
	"github.com/tokzone/fluxcore/message"
	"github.com/tokzone/fluxcore/routing"
)

const defaultTimeout = 30 * time.Second

const (
	defaultMaxIdleConns        = 100
	defaultMaxIdleConnsPerHost = 10
	defaultIdleConnTimeout     = 90 * time.Second
	defaultErrorBodyLimit      = 4096
)

var sharedClient = &http.Client{
	Timeout: defaultTimeout,
	Transport: &http.Transport{
		MaxIdleConns:        defaultMaxIdleConns,
		MaxIdleConnsPerHost: defaultMaxIdleConnsPerHost,
		IdleConnTimeout:     defaultIdleConnTimeout,
	},
}


// Request sends a chat completion request with automatic retry and failover.
// It handles endpoint selection, health checking, and protocol translation.
// Returns the response body in the input protocol format, usage statistics, or an error.
func Request(ctx context.Context, pool *routing.EndpointPool, rawReq []byte, inputProtocol routing.Protocol) ([]byte, *message.Usage, error) {
	req, err := parseRequest(rawReq, inputProtocol)
	if err != nil {
		return nil, nil, fmt.Errorf("parse request: %w", err)
	}

	ep, err := selectEndpoint(pool)
	if err != nil {
		return nil, nil, err
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
				return nil, nil, ctx.Err()
			}
		}

		resp, usage, err := callWithParsedRequest(ctx, ep, req, inputProtocol)
		if err == nil {
			ep.MarkSuccess()
			return resp, usage, nil
		}

		lastErr = err
		pool.MarkFail(ep)

		// Don't retry non-retryable errors (auth, invalid request, etc.)
		if !errors.IsRetryable(err) {
			break
		}
	}

	return nil, nil, lastErr
}

func transport(ctx context.Context, ep *routing.Endpoint, body []byte) ([]byte, error) {
	// Ensure requests have a deadline for timeout control
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, defaultTimeout)
		defer cancel()
	}

	req, err := http.NewRequestWithContext(ctx, "POST", buildURL(ep, false), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	setHeaders(req, ep, false)

	resp, err := sharedClient.Do(req)
	if err != nil {
		return nil, errors.ClassifyNetError(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, defaultErrorBodyLimit))
		return nil, errors.ClassifyHTTPError(resp.StatusCode, string(respBody))
	}

	return io.ReadAll(resp.Body)
}

// callWithParsedRequest executes a request with pre-parsed MessageRequest
func callWithParsedRequest(ctx context.Context, ep *routing.Endpoint, req *message.MessageRequest, inputProtocol routing.Protocol) ([]byte, *message.Usage, error) {
	start := time.Now()

	reqBody, err := translateRequest(req, ep.Key.Protocol)
	if err != nil {
		return nil, nil, fmt.Errorf("convert request: %w", err)
	}

	respBody, err := transport(ctx, ep, reqBody)
	if err != nil {
		return nil, nil, err
	}

	latency := int(time.Since(start).Milliseconds())

	resp, err := translateResponse(respBody, ep.Key.Protocol)
	if err != nil {
		return nil, nil, fmt.Errorf("convert response: %w", err)
	}

	usage := &message.Usage{
		LatencyMs:    latency,
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