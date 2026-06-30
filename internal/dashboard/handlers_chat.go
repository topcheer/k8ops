package dashboard

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/ggai/k8ops/internal/chat"
	"github.com/ggai/k8ops/internal/tools"
	"github.com/ggai/k8ops/internal/tools/host"
	"github.com/ggai/k8ops/internal/tools/k8s"
)

// --- Chat SSE endpoint ---

func (s *Server) handleChat(w http.ResponseWriter, r *http.Request) {
	if s.chatEngine == nil {
		writeError(w, 503, "chat engine not initialized")
		return
	}
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
