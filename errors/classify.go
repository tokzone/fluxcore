package errors

import (
	"context"
	stderrors "errors"
	"fmt"
	"net"
	"strings"
)

// ErrorCode represents classified error types
type ErrorCode string

const (
	// Network errors
	CodeNetworkError ErrorCode = "network_error" // Connection failures
	CodeTimeout      ErrorCode = "timeout"       // Request timeout
	CodeDNSError     ErrorCode = "dns_error"     // DNS resolution failure

	// Server errors
	CodeRateLimit   ErrorCode = "rate_limit"   // 429 Too Many Requests
	CodeServerError ErrorCode = "server_error" // 5xx errors
	CodeModelError  ErrorCode = "model_error"  // Model-specific errors (overloaded, not found)

	// Client errors
	CodeInvalidRequest ErrorCode = "invalid_request" // 400 Bad Request
	CodeAuthError      ErrorCode = "auth_error"      // 401/403 Authentication failures

	// Endpoint errors
	CodeNoEndpoint ErrorCode = "no_endpoint" // No available endpoints
)

// IsRetryable returns true if errors with this code are retryable.
func (c ErrorCode) IsRetryable() bool {
	switch c {
	case CodeTimeout, CodeNetworkError, CodeRateLimit, CodeServerError, CodeModelError:
		return true
	default:
		return false
	}
}

// ClassifiedError wraps error with classification
type ClassifiedError struct {
	Code       ErrorCode
	Message    string
	Original   error
	StatusCode int // HTTP status code if applicable
}

func (e *ClassifiedError) Error() string {
	if e.Original != nil {
		return fmt.Sprintf("[%s] %s: %v", e.Code, e.Message, e.Original)
	}
	return fmt.Sprintf("[%s] %s", e.Code, e.Message)
}

func (e *ClassifiedError) Unwrap() error {
	return e.Original
}

// Wrap wraps an existing error with classification.
func Wrap(code ErrorCode, message string, original error) *ClassifiedError {
	return &ClassifiedError{
		Code:     code,
		Message:  message,
		Original: original,
	}
}

// ClassifyHTTPError classifies HTTP response errors
func ClassifyHTTPError(statusCode int, body string) *ClassifiedError {
	var code ErrorCode

	// Check for model-specific errors first (can appear in various status codes)
	if strings.Contains(body, "model") && strings.Contains(body, "overloaded") {
		code = CodeModelError
	} else {
		switch {
		case statusCode == 429:
			code = CodeRateLimit
		case statusCode >= 500:
			code = CodeServerError
		case statusCode == 401 || statusCode == 403:
			code = CodeAuthError
		case statusCode >= 400 && statusCode < 500:
			// Other 4xx errors (including 400)
			code = CodeInvalidRequest
		default:
			code = CodeServerError
		}
	}

	msg := fmt.Sprintf("HTTP %d", statusCode)
	if body != "" {
		msg = msg + ": " + truncateBody(body, 200)
	}

	return &ClassifiedError{
		Code:       code,
		Message:    msg,
		StatusCode: statusCode,
	}
}

// ClassifyNetError classifies network/transport errors
func ClassifyNetError(err error) *ClassifiedError {
	if err == nil {
		return nil
	}

	// Check for timeout
	if isTimeoutError(err) {
		return Wrap(CodeTimeout, "request timeout", err)
	}

	// Check for DNS error
	if isDNSError(err) {
		return Wrap(CodeDNSError, "DNS resolution failed", err)
	}

	// Check for connection refused
	if isConnectionError(err) {
		return Wrap(CodeNetworkError, "connection failed", err)
	}

	// Generic network error
	return Wrap(CodeNetworkError, "network error", err)
}

// Helper functions for error classification

func isTimeoutError(err error) bool {
	// Check stdlib sentinel error
	if stderrors.Is(err, context.DeadlineExceeded) {
		return true
	}
	// Check net.Error timeout interface
	var netErr net.Error
	if stderrors.As(err, &netErr) && netErr.Timeout() {
		return true
	}
	// Fallback for non-stdlib errors
	msg := err.Error()
	return strings.Contains(msg, "timeout") ||
		strings.Contains(msg, "deadline exceeded") ||
		strings.Contains(msg, "context deadline")
}

func isDNSError(err error) bool {
	// net.DNSError check
	var dnsErr *net.DNSError
	if stderrors.As(err, &dnsErr) {
		return true
	}
	msg := err.Error()
	return strings.Contains(msg, "no such host") ||
		strings.Contains(msg, "DNS")
}

func isConnectionError(err error) bool {
	// Check net.OpError for connection errors
	var opErr *net.OpError
	if stderrors.As(err, &opErr) && opErr.Op == "dial" {
		return true
	}
	// Fallback for other connection errors
	msg := err.Error()
	return strings.Contains(msg, "connection refused") ||
		strings.Contains(msg, "connection reset") ||
		strings.Contains(msg, "network is unreachable")
}

func truncateBody(body string, maxLen int) string {
	if len(body) <= maxLen {
		return body
	}
	return body[:maxLen] + "..."
}

// IsRetryable checks if an error is retryable
func IsRetryable(err error) bool {
	var classified *ClassifiedError
	if stderrors.As(err, &classified) {
		return classified.Code.IsRetryable()
	}
	// Unknown errors - be conservative
	return false
}
