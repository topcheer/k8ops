package dashboard

import (
	"compress/gzip"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
)

// --- Gzip Compression Middleware ---

// gzipResponseWriter wraps http.ResponseWriter with gzip compression.
// SSE responses (text/event-stream) are never compressed.
type gzipResponseWriter struct {
	http.ResponseWriter
	gz            *gzip.Writer
	statusWritten bool
	gzClosed      bool
	mu            sync.Mutex
}

func (g *gzipResponseWriter) WriteHeader(code int) {
	if g.statusWritten {
		return
	}
	g.statusWritten = true
	ct := g.Header().Get("Content-Type")

	// Don't compress SSE streams
	if strings.Contains(ct, "text/event-stream") {
		g.gz.Close()
		g.gzClosed = true
		g.ResponseWriter.WriteHeader(code)
		return
	}

	g.Header().Set("Content-Encoding", "gzip")
	g.Header().Del("Content-Length")
	g.ResponseWriter.WriteHeader(code)
}

func (g *gzipResponseWriter) Write(b []byte) (int, error) {
	if !g.statusWritten {
		g.WriteHeader(200)
	}
	if g.gzClosed {
		return g.ResponseWriter.Write(b)
	}
	if g.Header().Get("Content-Encoding") == "gzip" {
		return g.gz.Write(b)
	}
	return g.ResponseWriter.Write(b)
}

func (g *gzipResponseWriter) Flush() {
	if !g.gzClosed {
		g.gz.Flush()
	}
	if flusher, ok := g.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

// gzipMiddleware compresses JSON/text responses for clients that accept gzip.
func (s *Server) gzipMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			next.ServeHTTP(w, r)
			return
		}
		// Only compress API responses (static files are small, already embedded)
		if !strings.HasPrefix(r.URL.Path, "/api/") {
			next.ServeHTTP(w, r)
			return
		}

		gz := gzip.NewWriter(w)
		defer gz.Close()

		gzw := &gzipResponseWriter{
			ResponseWriter: w,
			gz:            gz,
		}
		next.ServeHTTP(gzw, r)
	})
}

// --- Security Headers Middleware ---

func (s *Server) securityHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("X-XSS-Protection", "1; mode=block")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		h.Set("Permissions-Policy", "geolocation=(), microphone=(), camera=()")
		if s.IsTLS() {
			h.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}
		h.Set("Content-Security-Policy", "default-src 'self'; img-src 'self' data:; style-src 'self' 'unsafe-inline'; script-src 'self' 'unsafe-inline'; connect-src 'self'; font-src 'self'; frame-ancestors 'none'")
		next.ServeHTTP(w, r)
	})
}

// --- /api/version Endpoint ---

var (
	Version   = "dev"
	GitCommit = "unknown"
	BuildDate = "unknown"
)

func (s *Server) handleVersion(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"version":   Version,
		"gitCommit": GitCommit,
		"buildDate": BuildDate,
		"name":      "k8ops",
	})
}
