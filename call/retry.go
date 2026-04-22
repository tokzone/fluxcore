package call

import (
	"math/rand"
	"time"
)

// retry configuration constants (internal)
const (
	retryBackoffBase = 100 * time.Millisecond
	retryBackoffMax  = 5 * time.Second
)

// backoffWithJitter calculates exponential backoff with full jitter.
// Go 1.20+ rand.Float64 is concurrent-safe.
func backoffWithJitter(attempt int) time.Duration {
	if attempt <= 0 {
		return 0
	}
	// Exponential backoff: base * 2^(attempt-1)
	backoff := retryBackoffBase << uint(attempt-1)
	if backoff > retryBackoffMax {
		backoff = retryBackoffMax
	}
	// Full jitter: random value between 0 and backoff
	// Avoids thundering herd problem when multiple clients retry simultaneously
	return time.Duration(rand.Float64() * float64(backoff))
}