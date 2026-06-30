package auth

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// --- IPRateLimiter Unit Tests ---

func TestIPRateLimiter_Allow_NewIP(t *testing.T) {
	rl := newIPRateLimiter(5, 1)
	defer rl.Stop()

	if !rl.Allow("192.168.1.1") {
		t.Error("first request from new IP should be allowed")
	}
}

func TestIPRateLimiter_Allow_BurstExhaustion(t *testing.T) {
	rl := newIPRateLimiter(3, 1)
	defer rl.Stop()

	// First 3 requests exhaust the burst
	for i := 0; i < 3; i++ {
		if !rl.Allow("10.0.0.1") {
			t.Fatalf("request %d should be allowed (burst not exhausted yet)", i+1)
		}
	}
	// 4th request should be denied — burst exhausted
	if rl.Allow("10.0.0.1") {
		t.Error("4th request should be denied (burst exhausted)")
	}
}

func TestIPRateLimiter_Allow_DifferentIPsIndependent(t *testing.T) {
	rl := newIPRateLimiter(2, 1)
	defer rl.Stop()

	// Exhaust IP A
	rl.Allow("10.0.0.1")
	rl.Allow("10.0.0.1")

	// IP B should still be allowed
	if !rl.Allow("10.0.0.2") {
		t.Error("different IP should still be allowed after exhausting another")
	}
}

func TestIPRateLimiter_Allow_TokenRefill(t *testing.T) {
	// refillRate = 100 tokens/sec means ~1 token per 10ms
	rl := newIPRateLimiter(1, 100)
	defer rl.Stop()

	// Exhaust the single token
	if !rl.Allow("172.16.0.1") {
		t.Fatal("first request should be allowed")
	}
	// Should be denied immediately
	if rl.Allow("172.16.0.1") {
		t.Error("second immediate request should be denied")
	}
	// Wait for refill (>10ms for 1 token at 100/sec)
	time.Sleep(50 * time.Millisecond)
	// Should be allowed again after refill
	if !rl.Allow("172.16.0.1") {
		t.Error("request should be allowed after token refill")
	}
}

func TestIPRateLimiter_Allow_TokenCapAtMax(t *testing.T) {
	rl := newIPRateLimiter(3, 1)
	defer rl.Stop()

	// Consume 1 token, leaving 2
	rl.Allow("10.0.0.1")

	// Wait a bit so tokens could refill
	time.Sleep(100 * time.Millisecond)

	// After refill, tokens should be capped at maxTokens (3)
	rl.mu.Lock()
	v := rl.visitors["10.0.0.1"]
	if v.tokens > rl.maxTokens {
		rl.mu.Unlock()
		t.Fatalf("tokens %d exceeded max %d", v.tokens, rl.maxTokens)
	}
	rl.mu.Unlock()
}

func TestIPRateLimiter_Stop_TerminatesGoroutine(t *testing.T) {
	rl := newIPRateLimiter(5, 1)
	rl.Stop()

	// Calling Stop again should panic (double close) — verify it doesn't deadlock
	// (can't easily test double-close panic, but at least verify no deadlock)
}

func TestIPRateLimiter_Allow_ConcurrentAccess(t *testing.T) {
	rl := newIPRateLimiter(1000, 100)
	defer rl.Stop()

	var wg sync.WaitGroup
	allowed := int64(0)
	denied := int64(0)
	var mu sync.Mutex

	// 10 goroutines, each making 100 requests from the same IP
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				mu.Lock()
				if rl.Allow("192.168.1.100") {
					allowed++
				} else {
					denied++
				}
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	total := allowed + denied
	if total != 1000 {
		t.Errorf("total requests = %d, want 1000", total)
	}
	// With 1000 max tokens, all should be allowed on first visit
	// (first call creates entry with 999 tokens, subsequent consume)
	if allowed == 0 {
		t.Error("expected some requests to be allowed")
	}
}

// --- getClientIP Tests ---

func TestGetClientIP_XForwardedFor(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Forwarded-For", "203.0.113.5, 10.0.0.1, 10.0.0.2")

	ip := getClientIP(req)
	if ip != "203.0.113.5" {
		t.Errorf("getClientIP() = %q, want %q", ip, "203.0.113.5")
	}
}

func TestGetClientIP_XForwardedForSingleIP(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Forwarded-For", "198.51.100.10")

	ip := getClientIP(req)
	if ip != "198.51.100.10" {
		t.Errorf("getClientIP() = %q, want %q", ip, "198.51.100.10")
	}
}

func TestGetClientIP_XRealIP(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Real-IP", "192.0.2.50")

	ip := getClientIP(req)
	if ip != "192.0.2.50" {
		t.Errorf("getClientIP() = %q, want %q", ip, "192.0.2.50")
	}
}

func TestGetClientIP_RemoteAddrFallback(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.0.0.5:54321"

	ip := getClientIP(req)
	if ip != "10.0.0.5" {
		t.Errorf("getClientIP() = %q, want %q", ip, "10.0.0.5")
	}
}

func TestGetClientIP_RemoteAddrNoPort(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.0.0.5"

	ip := getClientIP(req)
	if ip != "10.0.0.5" {
		t.Errorf("getClientIP() = %q, want %q", ip, "10.0.0.5")
	}
}

func TestGetClientIP_EmptyHeaders(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "127.0.0.1:8080"

	ip := getClientIP(req)
	if ip != "127.0.0.1" {
		t.Errorf("getClientIP() = %q, want %q", ip, "127.0.0.1")
	}
}

// --- loginRateLimitMiddleware Tests ---

func TestLoginRateLimitMiddleware_AllowsUnderLimit(t *testing.T) {
	a := testAuth(t)
	// Note: testAuth cleanup via a.Close() already calls loginLimiter.Stop()

	called := false
	h := a.loginRateLimitMiddleware(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", nil)
	req.RemoteAddr = "192.168.1.1:12345"

	h.ServeHTTP(rec, req)

	if !called {
		t.Error("handler should be called when under rate limit")
	}
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestLoginRateLimitMiddleware_BlocksOverLimit(t *testing.T) {
	// Create authenticator with a low rate limit
	a := testAuth(t)
	// Replace with a fresh limiter that has only 2 burst tokens
	// (old limiter is stopped by testAuth cleanup via a.Close())
	a.loginLimiter = newIPRateLimiter(2, 1)

	called := false
	h := a.loginRateLimitMiddleware(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	// Exhaust the 2-token burst
	for i := 0; i < 2; i++ {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/auth/login", nil)
		req.RemoteAddr = "10.0.0.1:11111"
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("request %d: status = %d, want %d", i+1, rec.Code, http.StatusOK)
		}
	}

	// 3rd request should be 429
	called = false
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", nil)
	req.RemoteAddr = "10.0.0.1:11111"
	h.ServeHTTP(rec, req)

	if called {
		t.Error("handler should NOT be called when rate limited")
	}
	if rec.Code != http.StatusTooManyRequests {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusTooManyRequests)
	}
	if rec.Header().Get("Retry-After") != "60" {
		t.Errorf("Retry-After = %q, want %q", rec.Header().Get("Retry-After"), "60")
	}
	if rec.Header().Get("Content-Type") != "application/json" {
		t.Errorf("Content-Type = %q, want %q", rec.Header().Get("Content-Type"), "application/json")
	}
}

func TestLoginRateLimitMiddleware_NilLimiterPassThrough(t *testing.T) {
	a := testAuth(t)
	a.loginLimiter.Stop()
	a.loginLimiter = nil

	called := false
	h := a.loginRateLimitMiddleware(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", nil)

	h.ServeHTTP(rec, req)

	if !called {
		t.Error("handler should be called when limiter is nil (disabled)")
	}
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestLoginRateLimitMiddleware_PerIPIsolation(t *testing.T) {
	a := testAuth(t)
	a.loginLimiter = newIPRateLimiter(1, 1)

	h := a.loginRateLimitMiddleware(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// Exhaust IP A
	recA := httptest.NewRecorder()
	reqA := httptest.NewRequest(http.MethodPost, "/api/auth/login", nil)
	reqA.RemoteAddr = "10.0.0.1:11111"
	h.ServeHTTP(recA, reqA)
	if recA.Code != http.StatusOK {
		t.Errorf("IP A first: status = %d, want 200", recA.Code)
	}

	// IP A second = blocked
	recA2 := httptest.NewRecorder()
	reqA2 := httptest.NewRequest(http.MethodPost, "/api/auth/login", nil)
	reqA2.RemoteAddr = "10.0.0.1:11111"
	h.ServeHTTP(recA2, reqA2)
	if recA2.Code != http.StatusTooManyRequests {
		t.Errorf("IP A second: status = %d, want 429", recA2.Code)
	}

	// IP B = still allowed
	recB := httptest.NewRecorder()
	reqB := httptest.NewRequest(http.MethodPost, "/api/auth/login", nil)
	reqB.RemoteAddr = "10.0.0.2:22222"
	h.ServeHTTP(recB, reqB)
	if recB.Code != http.StatusOK {
		t.Errorf("IP B first: status = %d, want 200", recB.Code)
	}
}
