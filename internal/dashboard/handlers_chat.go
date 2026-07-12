package dashboard

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/ggai/k8ops/internal/auth"
	"github.com/ggai/k8ops/internal/chat"
	"github.com/ggai/k8ops/internal/resilience"
	"github.com/ggai/k8ops/internal/tools"
	"github.com/ggai/k8ops/internal/tools/host"
	"github.com/ggai/k8ops/internal/tools/k8s"
)

// userRateLimiter provides per-user rate limiting for chat/LLM calls.
// Each user gets an independent token bucket. Idle users' entries are
// periodically cleaned up to prevent unbounded memory growth.
type userRateLimiter struct {
	mu       sync.Mutex
	limiters map[string]*resilience.RateLimiter // keyed by username
	burst    int
	refill   int // tokens per second
}

func newUserRateLimiter(burst, refillPerSec int) *userRateLimiter {
	return &userRateLimiter{
		limiters: make(map[string]*resilience.RateLimiter),
		burst:    burst,
		refill:   refillPerSec,
	}
}

// Allow checks if the given user is allowed to make a request.
// Creates a new limiter on first use for unknown users.
func (u *userRateLimiter) Allow(username string) bool {
	u.mu.Lock()
	rl, ok := u.limiters[username]
	if !ok {
		rl = resilience.NewRateLimiter(u.burst, u.refill)
		u.limiters[username] = rl
	}
	u.mu.Unlock()
	return rl.Allow()
}

// Cleanup removes idle users (not seen for >10 minutes).
// Called periodically to prevent unbounded growth.
func (u *userRateLimiter) Cleanup(maxAge time.Duration) {
	u.mu.Lock()
	defer u.mu.Unlock()
	// If map is small, skip cleanup
	if len(u.limiters) < 100 {
		return
	}
	// Reset by recreating — RateLimiter doesn't track last-access time,
	// so we only clean up when the map gets large
	if len(u.limiters) > 500 {
		u.limiters = make(map[string]*resilience.RateLimiter)
	}
}

// --- Chat SSE endpoint ---

func (s *Server) handleChat(w http.ResponseWriter, r *http.Request) {
	if s.chatEngine == nil {
		writeError(w, 503, "chat engine not initialized")
		return
	}
	// Per-user rate limit: 20 burst, 10/min per user
	if s.chatLimiter == nil {
		s.chatLimiter = newUserRateLimiter(20, 10)
	}
	// Identify user from auth context
	username := "anonymous"
	if user := auth.UserFromRequest(r); user != nil && user.Username != "" {
		username = user.Username
	}
	if !s.chatLimiter.Allow(username) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(429)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"error": "rate limit exceeded",
			"hint":  "too many chat requests, please slow down",
		})
		return
	}
	// Periodic cleanup
	s.chatLimiter.Cleanup(10 * time.Minute)

	if r.Method != "POST" {
		writeError(w, 405, "method not allowed")
		return
	}

	var req struct {
		Message        string `json:"message"`
		ConversationID string `json:"conversationId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, 400, "invalid request body")
		return
	}
	if req.Message == "" {
		writeError(w, 400, "message is required")
		return
	}
	if req.ConversationID == "" {
		req.ConversationID = fmt.Sprintf("conv-%d", time.Now().UnixNano())
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, 500, "streaming not supported")
		return
	}

	ctx := r.Context()

	sendSSE(w, flusher, chat.StreamEvent{
		Type:      "conversation",
		Data:      map[string]string{"conversationId": req.ConversationID},
		Timestamp: time.Now().Format(time.RFC3339),
	})

	done := make(chan error, 1)
	go func() {
		err := s.chatEngine.RunStreamWithRegistry(ctx, req.ConversationID, req.Message, func(event chat.StreamEvent) {
			sendSSE(w, flusher, event)
		}, s.buildImpersonatedRegistry(r))
		done <- err
	}()

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case err := <-done:
			if err != nil {
				sendSSE(w, flusher, chat.StreamEvent{
					Type:      chat.EventError,
					Data:      map[string]string{"message": err.Error()},
					Timestamp: time.Now().Format(time.RFC3339),
				})
			}
			return
		case <-ticker.C:
			if _, err := fmt.Fprintf(w, "event: ping\ndata: {\"type\":\"ping\"}\n\n"); err != nil {
				return
			}
			flusher.Flush()
		case <-ctx.Done():
			return
		}
	}
}

// buildImpersonatedRegistry creates a per-request tool registry backed by
// an impersonated K8s client so chat AI tools respect the user's RBAC permissions.
func (s *Server) buildImpersonatedRegistry(r *http.Request) *tools.Registry {
	kc := s.ImpersonatedKubeClient(r)
	if kc == nil || kc == s.k8sClientTool {
		return nil
	}

	registry := tools.NewRegistry()
	for _, t := range []tools.Tool{
		&k8s.GetResourceTool{Client: kc},
		&k8s.ListResourcesTool{Client: kc},
		&k8s.DescribeResourceTool{Client: kc},
		&k8s.GetPodLogsTool{Client: kc},
		&k8s.GetEventsTool{Client: kc},
		&k8s.GetNamespacesTool{Client: kc},
		&k8s.GetTopTool{Client: kc},
		&k8s.GetPodStatusTool{Client: kc},
		&k8s.GetServicesTool{Client: kc},
		&k8s.GetNodesTool{Client: kc},
		&k8s.GetStorageTool{Client: kc},
		&k8s.GetConfigMapTool{Client: kc},
		&k8s.GetIngressTool{Client: kc},
		&k8s.GetClusterVersionTool{Client: kc},
		&host.HostInfoTool{},
		&host.HostDiskUsageTool{},
		&host.HostNetworkTool{},
		&host.HostProcessTool{},
		&host.HostDmesgTool{},
		// Audit tools — expose all dashboard audit endpoints to the LLM agent
		&k8s.AuditTool{DashboardAddr: "localhost:9090"},
		&k8s.ListAuditsTool{},
	} {
		registry.Register(t)
	}
	return registry
}

func sendSSE(w http.ResponseWriter, flusher http.Flusher, event chat.StreamEvent) {
	data, _ := json.Marshal(event)
	if _, err := fmt.Fprintf(w, "data: %s\n\n", data); err != nil {
		return
	}
	flusher.Flush()
}

// --- Conversation management ---

func (s *Server) handleConversations(w http.ResponseWriter, r *http.Request) {
	if s.chatEngine == nil {
		writeJSON(w, map[string]any{"conversations": []any{}})
		return
	}
	if r.Method == "DELETE" {
		id := r.URL.Query().Get("id")
		if id != "" {
			s.chatEngine.DeleteConversation(id)
		}
		writeJSON(w, map[string]string{"status": "deleted"})
		return
	}
	stats := s.chatEngine.ConversationStats()
	writeJSON(w, map[string]any{"conversations": stats})
}
