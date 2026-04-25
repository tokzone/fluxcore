// Package errors provides error classification for HTTP and network errors.
//
// The errors package handles:
//   - HTTP status code classification (retryable vs non-retryable)
//   - Network error detection (timeout, connection refused)
//   - Structured error types with context
//
// Main functions:
//   - ClassifyHTTPError: Classify HTTP status codes
//   - ClassifyNetError: Classify network errors
//   - IsRetryable: Check if error should trigger retry
//
// Error classification:
//   - Retryable: 429 (rate limit), 500, 502, 503, 504, network timeouts
//   - Non-retryable: 400, 401, 403, 404, invalid requests
//
// Example usage:
//
//	err := errors.ClassifyHTTPError(429, "rate limited")
//	if errors.IsRetryable(err) {
//	    // retry the request
//	}
package errors
