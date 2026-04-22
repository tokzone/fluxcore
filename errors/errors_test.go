package errors

import (
	"context"
	stderrors "errors"
	"testing"
)

func TestClassifiedErrorError(t *testing.T) {
	// Test with original error
	origErr := stderrors.New("original")
	classified := Wrap(CodeNetworkError, "connection failed", origErr)
	expected := "[network_error] connection failed: original"
	if classified.Error() != expected {
		t.Errorf("expected '%s', got '%s'", expected, classified.Error())
	}

	// Test without original error
	classified2 := New(CodeTimeout, "request timeout")
	expected2 := "[timeout] request timeout"
	if classified2.Error() != expected2 {
		t.Errorf("expected '%s', got '%s'", expected2, classified2.Error())
	}
}

func TestClassifiedErrorUnwrap(t *testing.T) {
	origErr := stderrors.New("original")
	classified := Wrap(CodeNetworkError, "wrapped", origErr)

	unwrapped := classified.Unwrap()
	if unwrapped != origErr {
		t.Errorf("expected original error, got %v", unwrapped)
	}
}

func TestClassifyHTTPErrorAllStatusCodes(t *testing.T) {
	tests := []struct {
		statusCode  int
		expectedCode ErrorCode
		retryable    bool
	}{
		{400, CodeInvalidRequest, false},
		{401, CodeAuthError, false},
		{403, CodeAuthError, false},
		{404, CodeInvalidRequest, false},
		{429, CodeRateLimit, true},
		{500, CodeServerError, true},
		{502, CodeServerError, true},
		{503, CodeServerError, true},
		{504, CodeServerError, true},
	}

	for _, tt := range tests {
		err := ClassifyHTTPError(tt.statusCode, "")
		if err.Code != tt.expectedCode {
			t.Errorf("HTTP %d: expected code %s, got %s", tt.statusCode, tt.expectedCode, err.Code)
		}
		if err.Code.IsRetryable() != tt.retryable {
			t.Errorf("HTTP %d: expected retryable %v, got %v", tt.statusCode, tt.retryable, err.Code.IsRetryable())
		}
		if err.StatusCode != tt.statusCode {
			t.Errorf("HTTP %d: expected StatusCode %d, got %d", tt.statusCode, tt.statusCode, err.StatusCode)
		}
	}
}

func TestClassifyHTTPErrorWithModelError(t *testing.T) {
	// Model overloaded error should be retryable
	err := ClassifyHTTPError(400, "model is overloaded")
	if err.Code != CodeModelError {
		t.Errorf("expected CodeModelError, got %s", err.Code)
	}
	if !err.Code.IsRetryable() {
		t.Error("model overloaded error should be retryable")
	}
}

func TestClassifyNetErrorTimeout(t *testing.T) {
	// Test context timeout
	err := ClassifyNetError(context.DeadlineExceeded)
	if err.Code != CodeTimeout {
		t.Errorf("expected CodeTimeout, got %s", err.Code)
	}
	if !err.Code.IsRetryable() {
		t.Error("timeout should be retryable")
	}
}

func TestIsRetryableWithNil(t *testing.T) {
	if IsRetryable(nil) {
		t.Error("nil error should not be retryable")
	}
}

func TestGetCodeWithNil(t *testing.T) {
	if GetCode(nil) != CodeNetworkError {
		t.Errorf("nil error should return CodeNetworkError, got %s", GetCode(nil))
	}
}

func TestTruncateBody(t *testing.T) {
	// Short string - no truncation
	short := "short"
	if truncateBody(short, 10) != short {
		t.Error("short string should not be truncated")
	}

	// Long string - truncation
	long := "this is a very long string that should be truncated"
	result := truncateBody(long, 10)
	if len(result) != 13 { // 10 + "..."
		t.Errorf("expected length 13, got %d", len(result))
	}
	if result != "this is a ..." {
		t.Errorf("expected 'this is a ...', got '%s'", result)
	}
}