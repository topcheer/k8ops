package dashboard

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/ggai/k8ops/internal/audit"
)

// handleNodeCordon cordons or uncordons a node.
// POST /api/node/cordon  body: {"name":"node1","unschedulable":true}
func (s *Server) handleNodeCordon(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req struct {
		Name          string `json:"name"`
		Unschedulable *bool  `json:"unschedulable"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "node name is required")
		return
	}
	if req.Unschedulable == nil {
		writeError(w, http.StatusBadRequest, "unschedulable field is required (true=cordon, false=uncordon)")
		return
	}

	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	node, err := rc.clientset.CoreV1().Nodes().Get(r.Context(), req.Name, metav1.GetOptions{})
	if err != nil {
		writeError(w, http.StatusNotFound, fmt.Sprintf("node not found: %v", err))
		return
	}

	node.Spec.Unschedulable = *req.Unschedulable
	if _, err := rc.clientset.CoreV1().Nodes().Update(r.Context(), node, metav1.UpdateOptions{}); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed to update node: %v", err))
		return
	}

	action := "uncordoned"
	if *req.Unschedulable {
		action = "cordoned"
	}
	s.log.Info("node "+action, "node", req.Name)
	if s.auditLog != nil {
		sev := audit.SeverityWarning
		if !*req.Unschedulable {
			sev = audit.SeverityInfo
		}
		s.auditLog.Log(r.Context(), audit.Event{Type: audit.EventTypeUserAction, Severity: sev, Actor: "dashboard-user", Action: action, Target: "node/" + req.Name, Success: true, Detail: map[string]any{"unschedulable": *req.Unschedulable}})
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success":       true,
		"name":          req.Name,
		"unschedulable": *req.Unschedulable,
		"message":       fmt.Sprintf("node %s %s", req.Name, action),
	})
}
