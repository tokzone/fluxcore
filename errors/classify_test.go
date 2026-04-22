package errors

import (
	"context"
	"errors"
	"net"
	"testing"
)

func TestClassifyNetError(t *testing.T) {
	t.Run("nil_error", func(t *testing.T) {
		result := ClassifyNetError(nil)
		if result != nil {
			t.Errorf("expected nil for nil error, got %+v", result)
		}
	})

	t.Run("context_deadline_exceeded", func(t *testing.T) {
		err := context.DeadlineExceeded
		result := ClassifyNetError(err)
		if result == nil {
			t.Fatal("expected classified error")
		}
		if result.Code != CodeTimeout {
			t.Errorf("expected CodeTimeout, got %s", result.Code)
		}
		if !result.Code.IsRetryable() {
			t.Error("timeout should be retryable")
		}
	})

	t.Run("timeout_string", func(t *testing.T) {
		err := errors.New("operation timeout")
		result := ClassifyNetError(err)
		if result == nil {
			t.Fatal("expected classified error")
		}
		if result.Code != CodeTimeout {
			t.Errorf("expected CodeTimeout for timeout string, got %s", result.Code)
		}
	})

	t.Run("deadline_exceeded_string", func(t *testing.T) {
		err := errors.New("context deadline exceeded")
		result := ClassifyNetError(err)
		if result == nil {
			t.Fatal("expected classified error")
		}
		if result.Code != CodeTimeout {
			t.Errorf("expected CodeTimeout, got %s", result.Code)
		}
	})

	t.Run("dns_error", func(t *testing.T) {
		dnsErr := &net.DNSError{
			Err:  "no such host",
			Name: "api.invalid-tld",
		}
		result := ClassifyNetError(dnsErr)
		if result == nil {
			t.Fatal("expected classified error")
		}
		if result.Code != CodeDNSError {
			t.Errorf("expected CodeDNSError, got %s", result.Code)
		}
		if result.Code.IsRetryable() {
			t.Error("DNS error should not be retryable")
		}
	})

	t.Run("no_such_host_string", func(t *testing.T) {
		err := errors.New("no such host")
		result := ClassifyNetError(err)
		if result == nil {
			t.Fatal("expected classified error")
		}
		if result.Code != CodeDNSError {
			t.Errorf("expected CodeDNSError, got %s", result.Code)
		}
	})

	t.Run("connection_refused", func(t *testing.T) {
		err := errors.New("connection refused")
		result := ClassifyNetError(err)
		if result == nil {
			t.Fatal("expected classified error")
		}
		if result.Code != CodeNetworkError {
			t.Errorf("expected CodeNetworkError, got %s", result.Code)
		}
		if !result.Code.IsRetryable() {
			t.Error("connection refused should be retryable")
		}
	})

	t.Run("connection_reset", func(t *testing.T) {
		err := errors.New("connection reset by peer")
		result := ClassifyNetError(err)
		if result == nil {
			t.Fatal("expected classified error")
		}
		if result.Code != CodeNetworkError {
			t.Errorf("expected CodeNetworkError, got %s", result.Code)
		}
	})

	t.Run("network_unreachable", func(t *testing.T) {
		err := errors.New("network is unreachable")
		result := ClassifyNetError(err)
		if result == nil {
			t.Fatal("expected classified error")
		}
		if result.Code != CodeNetworkError {
			t.Errorf("expected CodeNetworkError, got %s", result.Code)
		}
	})

	t.Run("generic_error", func(t *testing.T) {
		err := errors.New("some random error")
		result := ClassifyNetError(err)
		if result == nil {
			t.Fatal("expected classified error")
		}
		if result.Code != CodeNetworkError {
			t.Errorf("expected CodeNetworkError for generic, got %s", result.Code)
		}
		if !result.Code.IsRetryable() {
			t.Error("generic network error should be retryable")
		}
	})
}

func TestClassifyHTTPErrorEdgeCases(t *testing.T) {
	t.Run("model_overloaded_in_400", func(t *testing.T) {
		result := ClassifyHTTPError(400, `{"error": "model is overloaded"}`)
		if result.Code != CodeModelError {
			t.Errorf("expected CodeModelError, got %s", result.Code)
		}
		if !result.Code.IsRetryable() {
			t.Error("model overloaded should be retryable")
		}
	})

	t.Run("model_overloaded_in_503", func(t *testing.T) {
		result := ClassifyHTTPError(503, `{"error": "model is overloaded, please retry"}`)
		if result.Code != CodeModelError {
			t.Errorf("expected CodeModelError, got %s", result.Code)
		}
	})

	t.Run("empty_body", func(t *testing.T) {
		result := ClassifyHTTPError(500, "")
		if result.Code != CodeServerError {
			t.Errorf("expected CodeServerError, got %s", result.Code)
		}
	})

	t.Run("large_body_truncated", func(t *testing.T) {
		largeBody := make([]byte, 500)
		for i := range largeBody {
			largeBody[i] = 'a'
		}
		result := ClassifyHTTPError(500, string(largeBody))
		if result.Code != CodeServerError {
			t.Errorf("expected CodeServerError, got %s", result.Code)
		}
		// Message should be truncated
		if len(result.Message) > 300 {
			t.Errorf("message should be truncated, got length %d", len(result.Message))
		}
	})
}