package dashboard

import (
	"net/http"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// handleNodePods returns pods running on a specific node.
// GET /api/nodes/{node}/pods
func (s *Server) handleNodePods(w http.ResponseWriter, r *http.Request) {
	rc := s.clientsFromReq(r)
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/nodes/"), "/")
	if len(parts) < 2 || parts[1] != "pods" {
		writeError(w, 400, "invalid path, expected /api/nodes/{node}/pods")
		return
	}
	nodeName := parts[0]

	pods, err := rc.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{
		FieldSelector: "spec.nodeName=" + nodeName, Limit: 500,
	})
	if err != nil {
		writeK8sError(w, err)
		return
	}

	type podItem struct {
		Name       string   `json:"name"`
		Namespace  string   `json:"namespace"`
		Status     string   `json:"status"`
		Restarts   int32    `json:"restarts"`
		Node       string   `json:"node"`
		IP         string   `json:"ip"`
		Age        string   `json:"age"`
		Containers []string `json:"containers"`
	}
	items := make([]podItem, 0, len(pods.Items))
	for _, p := range pods.Items {
		restarts := int32(0)
		var containers []string
		for _, c := range p.Spec.Containers {
			containers = append(containers, c.Name)
		}
		for _, c := range p.Status.ContainerStatuses {
			restarts += c.RestartCount
		}
		items = append(items, podItem{
			Name: p.Name, Namespace: p.Namespace,
			Status: string(p.Status.Phase), Restarts: restarts,
			Node: p.Spec.NodeName, IP: p.Status.PodIP,
			Age:        ageTime(p.CreationTimestamp.Time),
			Containers: containers,
		})
	}
	writeJSON(w, map[string]any{"node": nodeName, "count": len(items), "pods": items})
}

// countReadyContainers counts how many containers are ready.
func countReadyContainers(statuses []corev1.ContainerStatus) int {
	ready := 0
	for _, cs := range statuses {
		if cs.Ready {
			ready++
		}
	}
	return ready
}

