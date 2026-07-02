package dashboard

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// handleScale scales a deployment or statefulset to the requested replica count.
// POST /api/scale  body: {"namespace":"default","kind":"deployment","name":"nginx","replicas":3}
func (s *Server) handleScale(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req struct {
		Namespace string `json:"namespace"`
		Kind      string `json:"kind"`
		Name      string `json:"name"`
		Replicas  int32  `json:"replicas"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Validate
	req.Namespace = strings.TrimSpace(req.Namespace)
	req.Name = strings.TrimSpace(req.Name)
	req.Kind = strings.ToLower(strings.TrimSpace(req.Kind))
	if req.Namespace == "" || req.Name == "" {
		writeError(w, http.StatusBadRequest, "namespace and name are required")
		return
	}
	if req.Replicas < 0 || req.Replicas > 1000 {
		writeError(w, http.StatusBadRequest, "replicas must be between 0 and 1000")
		return
	}

	allowedKinds := map[string]bool{"deployment": true, "deployments": true, "statefulset": true, "statefulsets": true}
	if !allowedKinds[req.Kind] {
		writeError(w, http.StatusBadRequest, "only deployment and statefulset can be scaled")
		return
	}
	// Normalize kind
	if strings.HasSuffix(req.Kind, "s") {
		req.Kind = req.Kind[:len(req.Kind)-1]
	}

	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	var newReplicas int32
	switch req.Kind {
	case "deployment":
		dep, err := rc.clientset.AppsV1().Deployments(req.Namespace).Get(r.Context(), req.Name, metav1.GetOptions{})
		if err != nil {
			writeError(w, http.StatusNotFound, fmt.Sprintf("deployment %s/%s not found: %v", req.Namespace, req.Name, err))
			return
		}
		oldReplicas := int32(0)
		if dep.Spec.Replicas != nil {
			oldReplicas = *dep.Spec.Replicas
		}
		dep.Spec.Replicas = &req.Replicas
		updated, err := rc.clientset.AppsV1().Deployments(req.Namespace).Update(r.Context(), dep, metav1.UpdateOptions{})
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to scale deployment: "+err.Error())
			return
		}
		newReplicas = int32(0)
		if updated.Spec.Replicas != nil {
			newReplicas = *updated.Spec.Replicas
		}
		s.log.Info("scaled deployment", "namespace", req.Namespace, "name", req.Name, "old", oldReplicas, "new", newReplicas)

	case "statefulset":
		sts, err := rc.clientset.AppsV1().StatefulSets(req.Namespace).Get(r.Context(), req.Name, metav1.GetOptions{})
		if err != nil {
			writeError(w, http.StatusNotFound, fmt.Sprintf("statefulset %s/%s not found: %v", req.Namespace, req.Name, err))
			return
		}
		oldReplicas := int32(0)
		if sts.Spec.Replicas != nil {
			oldReplicas = *sts.Spec.Replicas
		}
		sts.Spec.Replicas = &req.Replicas
		updated, err := rc.clientset.AppsV1().StatefulSets(req.Namespace).Update(r.Context(), sts, metav1.UpdateOptions{})
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to scale statefulset: "+err.Error())
			return
		}
		newReplicas = int32(0)
		if updated.Spec.Replicas != nil {
			newReplicas = *updated.Spec.Replicas
		}
		s.log.Info("scaled statefulset", "namespace", req.Namespace, "name", req.Name, "old", oldReplicas, "new", newReplicas)
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success":  true,
		"kind":     req.Kind,
		"namespace": req.Namespace,
		"name":      req.Name,
		"replicas":  newReplicas,
	})
}
