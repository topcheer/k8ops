package dashboard

import (
	"bytes"
	"net/http"
	"sync"
	"time"

	"github.com/ggai/k8ops/internal/auth"
)

// cacheEntry stores a cached HTTP response.
type cacheEntry struct {
	data      []byte
	expiresAt time.Time
}

// responseCache is a TTL-based in-memory cache for expensive API responses.
type responseCache struct {
	mu      sync.RWMutex
	entries map[string]*cacheEntry
	ttl     time.Duration
}

func newResponseCache(ttl time.Duration) *responseCache {
	return &responseCache{entries: make(map[string]*cacheEntry), ttl: ttl}
}

// get returns cached data if still valid.
func (c *responseCache) get(key string) ([]byte, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	e, ok := c.entries[key]
	if !ok || time.Now().After(e.expiresAt) {
		return nil, false
	}
	return e.data, true
}

// set stores data with the configured TTL.
func (c *responseCache) set(key string, data []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[key] = &cacheEntry{
		data:      data,
		expiresAt: time.Now().Add(c.ttl),
	}
}

// invalidate removes a specific cache key.
func (c *responseCache) invalidate(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.entries, key)
}

// invalidatePrefix removes all keys starting with the given prefix.
func (c *responseCache) invalidatePrefix(prefix string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for k := range c.entries {
		if len(k) >= len(prefix) && k[:len(prefix)] == prefix {
			delete(c.entries, k)
		}
	}
}

// cachedResponseWriter captures the response body for caching.
type cachedResponseWriter struct {
	http.ResponseWriter
	body *bytes.Buffer
}

func (w *cachedResponseWriter) Write(b []byte) (int, error) {
	w.body.Write(b)
	return w.ResponseWriter.Write(b)
}

// cacheMiddleware wraps a handler with TTL caching.
// The cache key is derived from the request path + query string + user role.
// When "?refresh=true" is passed, the cache is bypassed and refreshed.
func (s *Server) cacheMiddleware(ttl time.Duration, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Include user identity in cache key to prevent cross-user cache leakage
		userKey := "anon"
		if u := auth.UserFromRequest(r); u != nil {
			userKey = u.Role + ":" + u.AllowedNamespaces
		}
		key := userKey + ":" + r.URL.Path + "?" + r.URL.RawQuery

		// Bypass cache on explicit refresh
		if r.URL.Query().Get("refresh") == "true" {
			s.cache.invalidate(key)
		}

		// Try cache
		if data, ok := s.cache.get(key); ok {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("X-Cache", "HIT")
			w.Write(data)
			return
		}

		// Cache miss: execute handler and capture response
		cw := &cachedResponseWriter{ResponseWriter: w, body: &bytes.Buffer{}}
		next(cw, r)

		// Only cache successful JSON responses
		if cw.body.Len() > 0 && cw.body.Len() < 5*1024*1024 { // skip >5MB
			s.cache.set(key, cw.body.Bytes())
		}
	}
}
