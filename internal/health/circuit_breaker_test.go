package health

import (
	"testing"
	"time"
)

func TestCircuitBreaker_ClosedToOpen(t *testing.T) {
	cb := New(Config{Threshold: 3, Recovery: 60 * time.Second})

	if cb.IsOpen() {
		t.Error("new circuit breaker should be closed")
	}

	cb.MarkFailure()
	cb.MarkFailure()
	// Still closed at 2 failures with threshold 3
	if cb.IsOpen() {
		t.Error("should still be closed after 2 failures (threshold=3)")
	}

	tripped := cb.MarkFailure() // 3rd failure
	if !tripped {
		t.Error("3rd failure should trip circuit")
	}
	if !cb.IsOpen() {
		t.Error("should be open after threshold reached")
	}
	if cb.FailCount() != 3 {
		t.Errorf("fail count = %d, want 3", cb.FailCount())
	}
}

func TestCircuitBreaker_OpenToHalfOpen(t *testing.T) {
	cb := New(Config{Threshold: 1, Recovery: 10 * time.Millisecond})

	cb.MarkFailure()
	if !cb.IsOpen() {
		t.Fatal("should be open after failure")
	}

	time.Sleep(15 * time.Millisecond)
	if cb.IsOpen() {
		t.Error("should transition to half-open after recovery timeout")
	}
}

func TestCircuitBreaker_HalfOpenSuccess(t *testing.T) {
	cb := New(Config{Threshold: 1, Recovery: 10 * time.Millisecond})

	cb.MarkFailure()
	time.Sleep(15 * time.Millisecond)
	_ = cb.IsOpen() // triggers half-open transition

	cb.MarkSuccess()
	if cb.IsOpen() {
		t.Error("should be closed after success in half-open")
	}
	if cb.FailCount() != 0 {
		t.Errorf("fail count should reset, got %d", cb.FailCount())
	}
}

func TestCircuitBreaker_HalfOpenFailure(t *testing.T) {
	cb := New(Config{Threshold: 1, Recovery: 10 * time.Millisecond})

	cb.MarkFailure()
	time.Sleep(15 * time.Millisecond)
	_ = cb.IsOpen() // triggers half-open transition

	cb.MarkFailure()
	if !cb.IsOpen() {
		t.Error("should be open again after failure in half-open")
	}
}

func TestCircuitBreaker_MarkSuccessResets(t *testing.T) {
	cb := New(Config{Threshold: 3, Recovery: 60 * time.Second})

	cb.MarkFailure()
	cb.MarkFailure()
	cb.MarkSuccess()

	if cb.FailCount() != 0 {
		t.Errorf("fail count = %d, want 0 after success", cb.FailCount())
	}
	if cb.IsOpen() {
		t.Error("should be closed after success")
	}

	// Failures after reset should count from 0
	cb.MarkFailure()
	cb.MarkFailure()
	if cb.IsOpen() {
		t.Error("should still be closed (need 3 failures)")
	}
}

func TestCircuitBreaker_LatencyEWMA(t *testing.T) {
	cb := New(Config{Threshold: 3, Recovery: 60 * time.Second})

	if l := cb.LatencyEWMA(); l != 0 {
		t.Errorf("initial latency = %d, want 0", l)
	}

	cb.UpdateLatency(100)
	if l := cb.LatencyEWMA(); l != 100 {
		t.Errorf("first latency = %d, want 100", l)
	}

	cb.UpdateLatency(200)
	l := cb.LatencyEWMA()
	if l <= 100 || l >= 200 {
		t.Errorf("EWMA should be between 100 and 200, got %d", l)
	}
}

func TestCircuitBreaker_Concurrent(t *testing.T) {
	cb := New(Config{Threshold: 10, Recovery: 5 * time.Second})

	done := make(chan struct{})
	for i := 0; i < 5; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				cb.MarkFailure()
				cb.MarkSuccess()
				cb.UpdateLatency(50)
				_ = cb.IsOpen()
				_ = cb.FailCount()
				_ = cb.LatencyEWMA()
			}
			done <- struct{}{}
		}()
	}

	for i := 0; i < 5; i++ {
		<-done
	}
}
