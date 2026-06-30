package resilience

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"sync/atomic"
	"testing"
	"time"
)

func testLog() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}

// --- Retry tests ---

func TestRetry_SuccessOnFirstTry(t *testing.T) {
	calls := 0
	err := Retry(context.Background(), DefaultRetryConfig(), testLog(), func() error {
		calls++
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls != 1 {
		t.Errorf("expected 1 call, got %d", calls)
	}
}

func TestRetry_SuccessOnRetry(t *testing.T) {
	cfg := RetryConfig{MaxAttempts: 3, InitialDelay: 1 * time.Millisecond, MaxDelay: 5 * time.Millisecond, Multiplier: 2}
	var calls int32
	err := Retry(context.Background(), cfg, testLog(), func() error {
		n := atomic.AddInt32(&calls, 1)
		if n < 3 {
			return errors.New("transient error")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls != 3 {
		t.Errorf("expected 3 calls, got %d", calls)
	}
}

func TestRetry_AllFail(t *testing.T) {
	cfg := RetryConfig{MaxAttempts: 3, InitialDelay: 1 * time.Millisecond, MaxDelay: 5 * time.Millisecond, Multiplier: 2}
	err := Retry(context.Background(), cfg, testLog(), func() error {
		return errors.New("permanent error")
	})
	if err == nil {
		t.Error("expected error")
	}
}

func TestRetry_ContextCancel(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
	defer cancel()

	cfg := RetryConfig{MaxAttempts: 10, InitialDelay: 10 * time.Millisecond, MaxDelay: 100 * time.Millisecond, Multiplier: 2}
	err := Retry(ctx, cfg, testLog(), func() error {
		return errors.New("fail")
	})
	if err == nil {
		t.Error("expected context cancellation error")
	}
}

// --- Circuit Breaker tests ---

func TestCircuitBreaker_Closed(t *testing.T) {
	cb := NewCircuitBreaker(3, 1*time.Second)
	if !cb.Allow() {
		t.Error("should allow in closed state")
	}
	if cb.State() != StateClosed {
		t.Error("expected closed state")
	}
}

func TestCircuitBreaker_OpenAfterFailures(t *testing.T) {
	cb := NewCircuitBreaker(3, 1*time.Second)

	cb.RecordFailure()
	cb.RecordFailure()
	if !cb.Allow() {
		t.Error("should still allow with 2 failures")
	}

	cb.RecordFailure()
	if cb.Allow() {
		t.Error("should block after 3 failures")
	}
	if cb.State() != StateOpen {
		t.Error("expected open state")
	}
}

func TestCircuitBreaker_HalfOpen(t *testing.T) {
	cb := NewCircuitBreaker(1, 50*time.Millisecond)

	cb.RecordFailure()
	if cb.Allow() {
		t.Error("should block when open")
	}

	time.Sleep(60 * time.Millisecond)
	if !cb.Allow() {
		t.Error("should allow in half-open after timeout")
	}
	if cb.State() != StateHalfOpen {
		t.Error("expected half-open state")
	}

	cb.RecordSuccess()
	if cb.State() != StateClosed {
		t.Error("expected closed state after successful half-open")
	}
}

func TestCircuitBreaker_HalfOpenFailure(t *testing.T) {
	cb := NewCircuitBreaker(1, 50*time.Millisecond)

	cb.RecordFailure()
	time.Sleep(60 * time.Millisecond)

	cb.Allow() // triggers half-open
	cb.RecordFailure()

	if cb.State() != StateOpen {
		t.Error("expected back to open after half-open failure")
	}
}

// --- Rate Limiter tests ---

func TestRateLimiter_AllowWithinLimit(t *testing.T) {
	rl := NewRateLimiter(5, 1)
	for i := 0; i < 5; i++ {
		if !rl.Allow() {
			t.Errorf("expected allow for call %d", i)
		}
	}
}

func TestRateLimiter_ExceedLimit(t *testing.T) {
	rl := NewRateLimiter(2, 1)
	rl.Allow()
	rl.Allow()
	if rl.Allow() {
		t.Error("should block when tokens exhausted")
	}
}

func TestRateLimiter_Refill(t *testing.T) {
	rl := NewRateLimiter(1, 100) // 100 tokens/sec
	rl.Allow()
	if rl.Allow() {
		t.Error("should block after exhausting 1 token")
	}
	time.Sleep(20 * time.Millisecond) // should refill ~2 tokens
	if !rl.Allow() {
		t.Error("should allow after refill")
	}
}

func TestCircuitState_String(t *testing.T) {
	tests := []struct {
		state CircuitState
		want  string
	}{
		{StateClosed, "closed"},
		{StateOpen, "open"},
		{StateHalfOpen, "half-open"},
	}
	for _, tt := range tests {
		if tt.state.String() != tt.want {
			t.Errorf("expected '%s', got '%s'", tt.want, tt.state.String())
		}
	}
}
