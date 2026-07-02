package dashboard

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/ggai/k8ops/internal/metrics"
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
			gz:             gz,
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
	_ = json.NewEncoder(w).Encode(map[string]string{
		"version":   Version,
		"gitCommit": GitCommit,
		"buildDate": BuildDate,
		"name":      "k8ops",
	})
}

// handleQuickExec executes a safe kubectl get/describe/explain command from the NL-to-kubectl feature.
// Only read-only verbs are allowed: get, describe, explain, logs (with limits).
func (s *Server) handleQuickExec(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req struct {
		Command string `json:"command"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	cmd := strings.TrimSpace(req.Command)
	if cmd == "" {
		writeError(w, http.StatusBadRequest, "command is required")
		return
	}

	// Security: only allow kubectl prefix
	if !strings.HasPrefix(cmd, "kubectl ") {
		writeError(w, http.StatusForbidden, "only kubectl commands are allowed")
		return
	}

	// Security: whitelist verbs
	allowed := []string{"kubectl get ", "kubectl describe ", "kubectl explain "}
	matched := false
	for _, a := range allowed {
		if strings.HasPrefix(cmd, a) {
			matched = true
			break
		}
	}
	if !matched {
		writeError(w, http.StatusForbidden, "only read-only kubectl commands (get, describe, explain) are allowed")
		return
	}

	// Execute via nsenter on host kubectl
	parts := strings.Fields(cmd)
	execCmd := exec.Command("nsenter", append([]string{"-t", "1", "-m", "-u", "-i", "-n", "--"}, parts...)...)
	var stdout, stderr bytes.Buffer
	execCmd.Stdout = &stdout
	execCmd.Stderr = &stderr
	execCmd.Env = []string{"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"}

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	execCmd = exec.CommandContext(ctx, execCmd.Path, execCmd.Args[1:]...)
	execCmd.Stdout = &stdout
	execCmd.Stderr = &stderr

	if err := execCmd.Run(); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK) // return 200 with error field for frontend convenience
		_ = json.NewEncoder(w).Encode(map[string]string{
			"output": stdout.String(),
			"error":  err.Error() + ": " + stderr.String(),
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{
		"output": stdout.String(),
	})
}

// --- Request Timing Middleware ---

// statusCapture wraps http.ResponseWriter to capture the status code.
type statusCapture struct {
	http.ResponseWriter
	status int
}

func (sc *statusCapture) WriteHeader(code int) {
	sc.status = code
	sc.ResponseWriter.WriteHeader(code)
}

// timingMiddleware logs request duration and adds X-Response-Time header.
func (s *Server) timingMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sc := &statusCapture{ResponseWriter: w, status: 200}
		next(sc, r)
		elapsed := time.Since(start)
		w.Header().Set("X-Response-Time", fmt.Sprintf("%.3fs", elapsed.Seconds()))
		if elapsed > 500*time.Millisecond {
			s.log.Warn("slow request",
				"method", r.Method, "path", r.URL.Path,
				"duration", elapsed.String(), "status", sc.status,
				"requestId", requestIDFromCtx(r.Context()))
		}
	}
}

// --- Request ID Middleware ---

// requestIDKey is the context key for request IDs.
type requestIDKey struct{}

// generateRequestID creates a short random hex string.
func generateRequestID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// requestIDFromCtx extracts the request ID from context, or returns "unknown".
func requestIDFromCtx(ctx context.Context) string {
	if v, ok := ctx.Value(requestIDKey{}).(string); ok {
		return v
	}
	return "unknown"
}

// requestIDMiddleware injects a unique request ID into the context and response header.
func (s *Server) requestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqID := r.Header.Get("X-Request-ID")
		if reqID == "" {
			reqID = generateRequestID()
		}
		w.Header().Set("X-Request-ID", reqID)
		r = r.WithContext(context.WithValue(r.Context(), requestIDKey{}, reqID))
		next.ServeHTTP(w, r)
	})
}

// --- HTTP Metrics Middleware ---

// metricsStatusCapture wraps ResponseWriter to capture status code for metrics.
type metricsStatusCapture struct {
	http.ResponseWriter
	status int
}

func (m *metricsStatusCapture) WriteHeader(code int) {
	m.status = code
	m.ResponseWriter.WriteHeader(code)
}

// normalizePath converts a URL path to a template for metrics labels.
// e.g. /api/pods/default/nginx-abc/logs -> /api/pods/{ns}/{name}/logs
func normalizePath(path string) string {
	// Remove query string
	if idx := strings.IndexByte(path, '?'); idx >= 0 {
		path = path[:idx]
	}
	parts := strings.Split(path, "/")
	if len(parts) <= 3 {
		return path
	}
	// /api/pods/{ns}/{name}/... -> normalize namespace/name segments
	if len(parts) >= 2 && parts[1] == "api" {
		if len(parts) >= 5 && parts[2] == "pods" {
			// /api/pods/{ns}/{name}/action
			parts[3] = "{ns}"
			parts[4] = "{name}"
			return strings.Join(parts, "/")
		}
		if len(parts) >= 4 && parts[2] == "nodes" {
			// /api/nodes/{node}/pods
			parts[3] = "{node}"
			return strings.Join(parts, "/")
		}
		if len(parts) >= 4 && (parts[2] == "diagnostics" || parts[2] == "remediation") {
			// /api/diagnostics/{name} or /api/remediation/{name}
			parts[3] = "{name}"
			return strings.Join(parts, "/")
		}
		if len(parts) >= 4 && parts[2] == "audit" && parts[3] == "events" {
			return path
		}
	}
	return path
}

// httpMetricsMiddleware records Prometheus metrics for each HTTP request.
func (s *Server) httpMetricsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Skip static assets and SSE streams
		path := r.URL.Path
		if !strings.HasPrefix(path, "/api/") && path != "/healthz" && path != "/readyz" {
			next.ServeHTTP(w, r)
			return
		}
		normPath := normalizePath(path)

		start := time.Now()
		metrics.HTTPRequestsInFlight.Inc()
		defer metrics.HTTPRequestsInFlight.Dec()

		sc := &metricsStatusCapture{ResponseWriter: w, status: 200}
		next.ServeHTTP(sc, r)

		elapsed := time.Since(start).Seconds()
		statusStr := fmt.Sprintf("%d", sc.status)

		metrics.HTTPRequestsTotal.WithLabelValues(r.Method, normPath, statusStr).Inc()
		metrics.HTTPRequestDuration.WithLabelValues(r.Method, normPath).Observe(elapsed)

		// Track API errors (4xx + 5xx)
		if sc.status >= 400 {
			metrics.APIErrorsTotal.WithLabelValues(r.Method, normPath, statusStr).Inc()
		}

		// Record in performance tracker for percentile computation
		if s.perfTracker != nil {
			s.perfTracker.Record(APISample{
				Method:   r.Method,
				Path:     normPath,
				Duration: time.Since(start),
				Status:   sc.status,
			})
		}
	})
}
