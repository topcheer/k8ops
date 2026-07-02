package dashboard

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/ggai/k8ops/internal/audit"
)

// handlePodDelete deletes a single pod. Useful for forcing recreation of crash-looping pods.
// POST /api/pod/delete  body: {"namespace":"default","name":"nginx-abc123"}
func (s *Server) handlePodDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req struct {
		Namespace string `json:"namespace"`
		Name      string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	req.Namespace = strings.TrimSpace(req.Namespace)
	req.Name = strings.TrimSpace(req.Name)
	if req.Namespace == "" || req.Name == "" {
		writeError(w, http.StatusBadRequest, "namespace and name are required")
		return
	}

	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	err := rc.clientset.CoreV1().Pods(req.Namespace).Delete(r.Context(), req.Name, metav1.DeleteOptions{})
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed to delete pod: %v", err))
		return
	}

	s.log.Info("deleted pod", "namespace", req.Namespace, "name", req.Name)
	if s.auditLog != nil {
		s.auditLog.Log(r.Context(), audit.Event{Type: audit.EventTypeUserAction, Severity: audit.SeverityCritical, Actor: "dashboard-user", Action: "pod_delete", Target: "pod/" + req.Namespace + "/" + req.Name, Namespace: req.Namespace, Success: true})
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success":   true,
		"namespace": req.Namespace,
		"name":      req.Name,
		"message":   "pod deleted, controller will recreate it if managed",
	})
}

// handleRolloutRestart triggers a rolling restart by patching the rollout annotation.
// POST /api/rollout/restart  body: {"namespace":"default","kind":"deployment","name":"nginx"}
func (s *Server) handleRolloutRestart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req struct {
		Namespace string `json:"namespace"`
		Kind      string `json:"kind"`
		Name      string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	req.Namespace = strings.TrimSpace(req.Namespace)
	req.Name = strings.TrimSpace(req.Name)
	req.Kind = strings.ToLower(strings.TrimSpace(req.Kind))
	if req.Namespace == "" || req.Name == "" {
		writeError(w, http.StatusBadRequest, "namespace and name are required")
		return
	}

	allowedKinds := map[string]bool{"deployment": true, "deployments": true, "statefulset": true, "statefulsets": true, "daemonset": true, "daemonsets": true}
	if !allowedKinds[req.Kind] {
		writeError(w, http.StatusBadRequest, "only deployment, statefulset, and daemonset support rollout restart")
		return
	}
	if strings.HasSuffix(req.Kind, "s") {
		req.Kind = req.Kind[:len(req.Kind)-1]
	}

	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	// Patch the template annotation to trigger rollout
	timestamp := metav1.Now().Format("2006-01-02T15:04:05Z")
	annotations := map[string]string{
		"kubectl.kubernetes.io/restartedAt": timestamp,
	}

	switch req.Kind {
	case "deployment":
		dep, err := rc.clientset.AppsV1().Deployments(req.Namespace).Get(r.Context(), req.Name, metav1.GetOptions{})
		if err != nil {
			writeError(w, http.StatusNotFound, fmt.Sprintf("deployment not found: %v", err))
			return
		}
		if dep.Spec.Template.Annotations == nil {
			dep.Spec.Template.Annotations = make(map[string]string)
		}
		for k, v := range annotations {
			dep.Spec.Template.Annotations[k] = v
		}
		if _, err := rc.clientset.AppsV1().Deployments(req.Namespace).Update(r.Context(), dep, metav1.UpdateOptions{}); err != nil {
			writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed to restart deployment: %v", err))
			return
		}
		s.log.Info("rollout restart deployment", "namespace", req.Namespace, "name", req.Name)
		if s.auditLog != nil {
			s.auditLog.Log(r.Context(), audit.Event{Type: audit.EventTypeUserAction, Severity: audit.SeverityWarning, Actor: "dashboard-user", Action: "rollout_restart", Target: "deployment/" + req.Namespace + "/" + req.Name, Namespace: req.Namespace, Success: true})
		}

	case "statefulset":
		sts, err := rc.clientset.AppsV1().StatefulSets(req.Namespace).Get(r.Context(), req.Name, metav1.GetOptions{})
		if err != nil {
			writeError(w, http.StatusNotFound, fmt.Sprintf("statefulset not found: %v", err))
			return
		}
		if sts.Spec.Template.Annotations == nil {
			sts.Spec.Template.Annotations = make(map[string]string)
		}
		for k, v := range annotations {
			sts.Spec.Template.Annotations[k] = v
		}
		if _, err := rc.clientset.AppsV1().StatefulSets(req.Namespace).Update(r.Context(), sts, metav1.UpdateOptions{}); err != nil {
			writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed to restart statefulset: %v", err))
			return
		}
		s.log.Info("rollout restart statefulset", "namespace", req.Namespace, "name", req.Name)
		if s.auditLog != nil {
			s.auditLog.Log(r.Context(), audit.Event{Type: audit.EventTypeUserAction, Severity: audit.SeverityWarning, Actor: "dashboard-user", Action: "rollout_restart", Target: "statefulset/" + req.Namespace + "/" + req.Name, Namespace: req.Namespace, Success: true})
		}

	case "daemonset":
		ds, err := rc.clientset.AppsV1().DaemonSets(req.Namespace).Get(r.Context(), req.Name, metav1.GetOptions{})
		if err != nil {
			writeError(w, http.StatusNotFound, fmt.Sprintf("daemonset not found: %v", err))
			return
		}
		if ds.Spec.Template.Annotations == nil {
			ds.Spec.Template.Annotations = make(map[string]string)
		}
		for k, v := range annotations {
			ds.Spec.Template.Annotations[k] = v
		}
		if _, err := rc.clientset.AppsV1().DaemonSets(req.Namespace).Update(r.Context(), ds, metav1.UpdateOptions{}); err != nil {
			writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed to restart daemonset: %v", err))
			return
		}
		s.log.Info("rollout restart daemonset", "namespace", req.Namespace, "name", req.Name)
		if s.auditLog != nil {
			s.auditLog.Log(r.Context(), audit.Event{Type: audit.EventTypeUserAction, Severity: audit.SeverityWarning, Actor: "dashboard-user", Action: "rollout_restart", Target: "daemonset/" + req.Namespace + "/" + req.Name, Namespace: req.Namespace, Success: true})
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success":   true,
		"kind":      req.Kind,
		"namespace": req.Namespace,
		"name":      req.Name,
		"message":   "rollout restart triggered",
	})
}
