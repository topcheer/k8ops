// Package resilience provides retry, circuit breaker, and rate limiting
// for LLM provider calls and tool executions.
package resilience

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// --- Retry ---

type RetryConfig struct {
	MaxAttempts  int
	InitialDelay time.Duration
	MaxDelay     time.Duration
	Multiplier   float64
}

func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxAttempts:  3,
		InitialDelay: 500 * time.Millisecond,
		MaxDelay:     10 * time.Second,
		Multiplier:   2.0,
	}
}

// Retry executes fn with exponential backoff retry.
func Retry(ctx context.Context, cfg RetryConfig, log *slog.Logger, fn func() error) error {
	var lastErr error
	delay := cfg.InitialDelay

	for attempt := 1; attempt <= cfg.MaxAttempts; attempt++ {
		if err := fn(); err != nil {
			lastErr = err
			if attempt == cfg.MaxAttempts {
				break
			}
			if log != nil {
				log.Debug("retrying", "attempt", attempt, "delay", delay, "error", err)
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(delay):
			}
			delay = time.Duration(float64(delay) * cfg.Multiplier)
			if delay > cfg.MaxDelay {
				delay = cfg.MaxDelay
			}
		} else {
			return nil
		}
	}
	return fmt.Errorf("after %d attempts: %w", cfg.MaxAttempts, lastErr)
}

// --- Circuit Breaker ---

type CircuitState int

const (
	StateClosed   CircuitState = iota // normal operation
	StateOpen                         // failing, reject all
	StateHalfOpen                     // testing if recovered
)

func (s CircuitState) String() string {
	switch s {
	case StateClosed:
		return "closed"
	case StateOpen:
		return "open"
	case StateHalfOpen:
		return "half-open"
	}
	return "unknown"
}

type CircuitBreaker struct {
	mu               sync.Mutex
	state            CircuitState
	failureCount     int
	failureThreshold int
	resetTimeout     time.Duration
	lastFailureTime  time.Time
	successCount     int
	halfOpenMax      int
}

func NewCircuitBreaker(threshold int, resetTimeout time.Duration) *CircuitBreaker {
	return &CircuitBreaker{
		state:            StateClosed,
		failureThreshold: threshold,
		resetTimeout:     resetTimeout,
		halfOpenMax:      1,
	}
}

// Allow returns true if a request should be allowed through.
func (cb *CircuitBreaker) Allow() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case StateClosed:
		return true
	case StateOpen:
		if time.Since(cb.lastFailureTime) > cb.resetTimeout {
			cb.state = StateHalfOpen
			cb.successCount = 0
			return true
		}
		return false
	case StateHalfOpen:
		return cb.successCount < cb.halfOpenMax
	}
	return false
}

// RecordSuccess records a successful call.
func (cb *CircuitBreaker) RecordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	if cb.state == StateHalfOpen {
		cb.successCount++
		if cb.successCount >= cb.halfOpenMax {
			cb.state = StateClosed
			cb.failureCount = 0
		}
	} else if cb.state == StateClosed {
		cb.failureCount = 0
	}
}

// RecordFailure records a failed call.
func (cb *CircuitBreaker) RecordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.failureCount++
	cb.lastFailureTime = time.Now()

	if cb.state == StateHalfOpen {
		cb.state = StateOpen
	} else if cb.state == StateClosed && cb.failureCount >= cb.failureThreshold {
		cb.state = StateOpen
	}
}

// State returns the current circuit breaker state.
func (cb *CircuitBreaker) State() CircuitState {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	return cb.state
}

// --- Rate Limiter (token bucket) ---

type RateLimiter struct {
	mu         sync.Mutex
	tokens     int
	maxTokens  int
	refillRate int // tokens per second
	lastRefill time.Time
}

func NewRateLimiter(maxTokens, refillRate int) *RateLimiter {
	return &RateLimiter{
		tokens:     maxTokens,
		maxTokens:  maxTokens,
		refillRate: refillRate,
		lastRefill: time.Now(),
	}
}

// Allow tries to consume one token. Returns true if allowed.
func (rl *RateLimiter) Allow() bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(rl.lastRefill).Seconds()
	rl.tokens += int(elapsed * float64(rl.refillRate))
	if rl.tokens > rl.maxTokens {
		rl.tokens = rl.maxTokens
	}
	rl.lastRefill = now

	if rl.tokens > 0 {
		rl.tokens--
		return true
	}
	return false
}

// Tokens returns the current number of available tokens.
func (rl *RateLimiter) Tokens() int {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	return rl.tokens
}
