package call

import (
	"testing"
	"time"
)

func TestRetryConfig(t *testing.T) {
	// Save original config
	original := GetRetryConfig()
	defer SetRetryConfig(&original) // Restore after test

	// Test SetRetryConfig
	SetRetryConfig(&RetryConfig{
		BaseBackoff: 200 * time.Millisecond,
		MaxBackoff:  10 * time.Second,
	})

	cfg := GetRetryConfig()
	if cfg.BaseBackoff != 200*time.Millisecond {
		t.Errorf("expected BaseBackoff 200ms, got %v", cfg.BaseBackoff)
	}
	if cfg.MaxBackoff != 10*time.Second {
		t.Errorf("expected MaxBackoff 10s, got %v", cfg.MaxBackoff)
	}
}

func TestSetRetryConfigNil(t *testing.T) {
	original := GetRetryConfig()
	defer SetRetryConfig(&original)

	SetRetryConfig(nil) // Should be no-op
	cfg := GetRetryConfig()
	if cfg.BaseBackoff != original.BaseBackoff {
		t.Error("nil config should not change values")
	}
}

func TestSetRetryConfigZeroValues(t *testing.T) {
	original := GetRetryConfig()
	defer SetRetryConfig(&original)

	// Zero values should be ignored
	SetRetryConfig(&RetryConfig{BaseBackoff: 0, MaxBackoff: 0})
	cfg := GetRetryConfig()
	if cfg.BaseBackoff != original.BaseBackoff {
		t.Error("zero values should be ignored")
	}
}

func TestBackoffWithJitterZeroAttemptConfig(t *testing.T) {
	if backoffWithJitter(0) != 0 {
		t.Error("attempt 0 should return 0 backoff")
	}
	if backoffWithJitter(-1) != 0 {
		t.Error("negative attempt should return 0 backoff")
	}
}

func TestBackoffWithJitterBounds(t *testing.T) {
	original := GetRetryConfig()
	defer SetRetryConfig(&original)

	SetRetryConfig(&RetryConfig{
		BaseBackoff: 100 * time.Millisecond,
		MaxBackoff:  1 * time.Second,
	})

	// Test that backoff is within expected bounds
	for attempt := 1; attempt <= 10; attempt++ {
		backoff := backoffWithJitter(attempt)
		// With full jitter, backoff should be between 0 and maxBackoff
		if backoff < 0 {
			t.Errorf("attempt %d: negative backoff %v", attempt, backoff)
		}
		if backoff > 1*time.Second {
			t.Errorf("attempt %d: backoff %v exceeds max", attempt, backoff)
		}
	}
}