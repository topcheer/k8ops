package auth

import (
	"net/http"
	"strings"
	"sync"
	"time"
)

// --- Per-IP Rate Limiting for Login ---

// ipRateLimiter provides per-IP token-bucket rate limiting for login attempts.
type ipRateLimiter struct {
	mu         sync.Mutex
	visitors   map[string]*visitorEntry
	maxTokens  int
	refillRate int // tokens per second
	ttl        time.Duration
	stopCh     chan struct{}
	stopOnce   sync.Once
}

type visitorEntry struct {
	tokens   int
	lastSeen time.Time
}

func newIPRateLimiter(maxTokens, refillRate int) *ipRateLimiter {
	rl := &ipRateLimiter{
		visitors:   make(map[string]*visitorEntry),
		maxTokens:  maxTokens,
		refillRate: refillRate,
		ttl:        15 * time.Minute,
		stopCh:     make(chan struct{}),
	}
	go rl.cleanup()
	return rl
}

// Allow checks if the given IP is allowed to make a request.
func (rl *ipRateLimiter) Allow(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	v, exists := rl.visitors[ip]
	if !exists {
		rl.visitors[ip] = &visitorEntry{
			tokens:   rl.maxTokens - 1,
			lastSeen: now,
		}
		return true
	}

	// Refill tokens based on elapsed time.
	elapsed := now.Sub(v.lastSeen).Seconds()
	v.tokens += int(elapsed * float64(rl.refillRate))
	if v.tokens > rl.maxTokens {
		v.tokens = rl.maxTokens
	}
	v.lastSeen = now

	if v.tokens > 0 {
		v.tokens--
		return true
	}
	return false
}

// Stop signals the cleanup goroutine to exit. Safe to call multiple times.
func (rl *ipRateLimiter) Stop() {
	rl.stopOnce.Do(func() { close(rl.stopCh) })
}

func (rl *ipRateLimiter) cleanup() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-rl.stopCh:
			return
		case <-ticker.C:
			rl.mu.Lock()
			now := time.Now()
			for ip, v := range rl.visitors {
				if now.Sub(v.lastSeen) > rl.ttl {
					delete(rl.visitors, ip)
				}
			}
			rl.mu.Unlock()
		}
	}
}

// loginRateLimitMiddleware wraps the given handler with per-IP rate limiting.
// Returns 429 Too Many Requests if the IP has exceeded the rate limit.
func (a *Authenticator) loginRateLimitMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if a.loginLimiter == nil {
			next(w, r) // rate limiting disabled if limiter not initialized
			return
		}
		ip := getClientIP(r)
		if !a.loginLimiter.Allow(ip) {
			w.Header().Set("Retry-After", "60")
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusTooManyRequests)
			w.Write([]byte(`{"error":"rate_limited","message":"Too many login attempts. Please try again later."}`))
			return
		}
		next(w, r)
	}
}

// getClientIP extracts the client IP address from the request.
func getClientIP(r *http.Request) string {
	// Check X-Forwarded-For (first IP in the chain)
	xff := r.Header.Get("X-Forwarded-For")
	if xff != "" {
		ips := strings.Split(xff, ",")
		return strings.TrimSpace(ips[0])
	}
	// Check X-Real-IP
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return strings.TrimSpace(xri)
	}
	// Fall back to RemoteAddr (strip port)
	if idx := strings.LastIndex(r.RemoteAddr, ":"); idx > 0 {
		return r.RemoteAddr[:idx]
	}
	return r.RemoteAddr
}
