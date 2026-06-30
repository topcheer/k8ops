package dashboard

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/ggai/k8ops/internal/provider"
	"github.com/ggai/k8ops/internal/tools"
	"github.com/ggai/k8ops/internal/tools/host"
	"github.com/ggai/k8ops/internal/tools/k8s"
)

// --- Provider status & management ---

func (s *Server) handleProviderStatus(w http.ResponseWriter, r *http.Request) {
	if s.providerMgr == nil {
		writeJSON(w, map[string]any{"active": false})
		return
	}
	writeJSON(w, s.providerMgr.Status())
}

func (s *Server) handleProviderUpdate(w http.ResponseWriter, r *http.Request) {
	if s.providerMgr == nil {
		writeError(w, 503, "provider manager not initialized")
		return
	}
	if r.Method != "POST" {
		writeError(w, 405, "method not allowed")
		return
	}

	var req struct {
		Type        string  `json:"type"`
		Model       string  `json:"model"`
		APIKey      string  `json:"apiKey"`
		Endpoint    string  `json:"endpoint"`
		MaxTokens   int     `json:"maxTokens"`
		Temperature float64 `json:"temperature"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, 400, "invalid request body")
		return
	}

	if req.Type == "" || req.Model == "" || req.APIKey == "" {
		writeError(w, 400, "type, model, and apiKey are required")
		return
	}

	cfg := providerConfigFromRequest(req.Type, req.Model, req.APIKey, req.Endpoint, req.MaxTokens, req.Temperature)
	if err := s.providerMgr.ReloadFromDirect(r.Context(), cfg); err != nil {
		writeK8sError(w, err)
		return
	}

	writeJSON(w, map[string]any{
		"status":  "updated",
		"message": fmt.Sprintf("Provider switched to %s/%s (hot-reloaded)", req.Type, req.Model),
	})
}

func (s *Server) handleProviderReload(w http.ResponseWriter, r *http.Request) {
	if s.providerMgr == nil {
		writeError(w, 503, "provider manager not initialized")
		return
	}

	if err := s.providerMgr.Reload(r.Context()); err != nil {
		writeK8sError(w, err)
		return
	}

	writeJSON(w, s.providerMgr.Status())
}

// --- Tool listing ---

func (s *Server) handleToolList(w http.ResponseWriter, r *http.Request) {
	registry := tools.NewRegistry()
	if s.k8sClientTool != nil {
		registry.Register(&k8s.GetResourceTool{Client: s.k8sClientTool})
		registry.Register(&k8s.GetEventsTool{Client: s.k8sClientTool})
		registry.Register(&k8s.GetPodStatusTool{Client: s.k8sClientTool})
		registry.Register(&k8s.GetServicesTool{Client: s.k8sClientTool})
		registry.Register(&k8s.GetNodesTool{Client: s.k8sClientTool})
		registry.Register(&k8s.GetTopTool{Client: s.k8sClientTool})
		registry.Register(&k8s.GetStorageTool{Client: s.k8sClientTool})
	}
	registry.Register(&host.HostInfoTool{})
	registry.Register(&host.HostDiskUsageTool{})

	type toolInfo struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	toolList := make([]toolInfo, 0)
	for _, def := range registry.Definitions() {
		toolList = append(toolList, toolInfo{
			Name: def.Function.Name, Description: def.Function.Description,
		})
	}
	writeJSON(w, map[string]any{"count": len(toolList), "tools": toolList})
}

func providerConfigFromRequest(pType, model, apiKey, endpoint string, maxTokens int, temp float64) provider.ProviderConfig {
	if maxTokens == 0 {
		maxTokens = 4096
	}
	return provider.ProviderConfig{
		Type:        pType,
		Model:       model,
		APIKey:      apiKey,
		Endpoint:    endpoint,
		MaxTokens:   maxTokens,
		Temperature: temp,
	}
}
