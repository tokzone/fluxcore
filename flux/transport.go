package flux

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"time"

	"github.com/tokzone/fluxcore/errors"
)

const defaultTimeout = 30 * time.Second

const (
	defaultMaxIdleConns        = 100
	defaultMaxIdleConnsPerHost = 10
	defaultIdleConnTimeout     = 90 * time.Second
	defaultErrorBodyLimit      = 4096
	defaultResponseBodyLimit   = 10 * 1024 * 1024 // 10MB
)

var sharedClient = &http.Client{
	Timeout: defaultTimeout,
	Transport: &http.Transport{
		MaxIdleConns:        defaultMaxIdleConns,
		MaxIdleConnsPerHost: defaultMaxIdleConnsPerHost,
		IdleConnTimeout:     defaultIdleConnTimeout,
	},
}

func transport(ctx context.Context, ue *UserEndpoint, body []byte) ([]byte, error) {
	// Ensure requests have a deadline for timeout control
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, defaultTimeout)
		defer cancel()
	}

	req, err := http.NewRequestWithContext(ctx, "POST", buildURL(ue, false), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	setHeaders(req, ue, false)

	resp, err := sharedClient.Do(req)
	if err != nil {
		return nil, errors.ClassifyNetError(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, defaultErrorBodyLimit))
		return nil, errors.ClassifyHTTPError(resp.StatusCode, string(respBody))
	}

	return io.ReadAll(io.LimitReader(resp.Body, defaultResponseBodyLimit))
}
