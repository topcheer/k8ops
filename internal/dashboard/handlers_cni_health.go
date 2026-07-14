package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// CNIHealthResult is the CNI plugin health & network stack configuration audit.
type CNIHealthResult struct {
	ScannedAt       time.Time       `json:"scannedAt"`
	Summary         CNISummary      `json:"summary"`
	Nodes           []CNINodeStatus `json:"nodes"`
	Gaps            []CNIHealthGap  `json:"gaps"`
	Recommendations []string        `json:"recommendations"`
	HealthScore     int             `json:"healthScore"`
}

// CNISummary aggregates CNI statistics.
type CNISummary struct {
	TotalNodes      int    `json:"totalNodes"`
	HealthyNodes    int    `json:"healthyNodes"`
	CNIType         string `json:"cniType"` // calico, flannel, cilium, weave, canal
	CNINamespace    string `json:"cniNamespace"`
	CNIPodsReady    int    `json:"cniPodsReady"`
	CNIPodsTotal    int    `json:"cniPodsTotal"`
	NodesWithCNI    int    `json:"nodesWithCNI"`    // nodes running CNI agent
	NodesWithoutCNI int    `json:"nodesWithoutCNI"` // nodes missing CNI
	IPAMType        string `json:"ipamType"`        // host-local, calico-ipam, etc.
	NetworkReady    int    `json:"networkReady"`
	NetworkNotReady int    `json:"networkNotReady"`
}

// CNINodeStatus describes CNI status per node.
type CNINodeStatus struct {
	NodeName     string `json:"nodeName"`
	HasCNI       bool   `json:"hasCNI"`
	CNIPlugin    string `json:"cniPlugin,omitempty"`
	NetworkReady bool   `json:"networkReady"`
	PodCIDR      string `json:"podCIDR,omitempty"`
	Status       string `json:"status"`
}

// CNIHealthGap describes a CNI health gap.
type CNIHealthGap struct {
	NodeName string `json:"nodeName,omitempty"`
	Issue    string `json:"issue"`
	Severity string `json:"severity"`
}

// handleCNIHealth audits CNI plugin health & network stack configuration.
// GET /api/operations/cni-health
func (s *Server) handleCNIHealth(w http.ResponseWriter, r *http.Request) {
	result := CNIHealthResult{
		ScannedAt: time.Now(),
	}

	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	// Known CNI plugins and their detection patterns
	cniPlugins := map[string]string{
		"calico":      "calico",
		"flannel":     "flannel",
		"cilium":      "cilium",
		"weave":       "weave",
		"canal":       "canal",
		"antrea":      "antrea",
		"multus":      "multus",
		"kube-router": "kube-router",
	}

	cniNamespaces := map[string]bool{
		"kube-system":    true,
		"calico-system":  true,
		"cilium-system":  true,
		"tigera-system":  true,
		"antrea-system":  true,
		"metallb-system": true,
	}

	// 1. Detect CNI type from pods in kube-system or CNI namespaces
	pods, err := rc.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	if err == nil {
		for _, pod := range pods.Items {
			if !cniNamespaces[pod.Namespace] {
				continue
			}
			for _, c := range pod.Spec.Containers {
				imageLower := strings.ToLower(c.Image)
				for keyword, plugin := range cniPlugins {
					if strings.Contains(imageLower, keyword) || strings.Contains(strings.ToLower(pod.Name), keyword) {
						if result.Summary.CNIType == "" {
							result.Summary.CNIType = plugin
							result.Summary.CNINamespace = pod.Namespace
						}
						break
					}
				}
			}
		}
	}

	// 2. Check nodes for CNI status
	nodes, err := rc.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	if err == nil {
		for _, node := range nodes.Items {
			result.Summary.TotalNodes++
			nodeStatus := CNINodeStatus{
				NodeName: node.Name,
				Status:   "healthy",
			}

			// Check network ready condition
			networkReady := false
			for _, cond := range node.Status.Conditions {
				if cond.Type == corev1.NodeReady && cond.Status == corev1.ConditionTrue {
					networkReady = true
				}
				if cond.Type == corev1.NodeNetworkUnavailable && cond.Status == corev1.ConditionTrue {
					networkReady = false
					result.Summary.NetworkNotReady++
					result.Gaps = append(result.Gaps, CNIHealthGap{
						NodeName: node.Name,
						Issue:    "Node network unavailable — CNI may not be running",
						Severity: "critical",
					})
				}
			}

			// Check PodCIDR
			if node.Spec.PodCIDR != "" {
				nodeStatus.PodCIDR = node.Spec.PodCIDR
				nodeStatus.HasCNI = true
				result.Summary.NodesWithCNI++
			} else {
				nodeStatus.HasCNI = false
				result.Summary.NodesWithoutCNI++
				nodeStatus.Status = "degraded"
				result.Gaps = append(result.Gaps, CNIHealthGap{
					NodeName: node.Name,
					Issue:    "No PodCIDR assigned — CNI may not have configured this node",
					Severity: "medium",
				})
			}

			nodeStatus.NetworkReady = networkReady
			nodeStatus.CNIPlugin = result.Summary.CNIType
			if networkReady && nodeStatus.HasCNI {
				result.Summary.HealthyNodes++
				result.Summary.NetworkReady++
			} else {
				nodeStatus.Status = "degraded"
			}

			result.Nodes = append(result.Nodes, nodeStatus)
		}
	}

	// 3. Count CNI agent pods
	if pods != nil {
		for _, pod := range pods.Items {
			if result.Summary.CNINamespace != "" && pod.Namespace != result.Summary.CNINamespace && !cniNamespaces[pod.Namespace] {
				continue
			}
			if result.Summary.CNIType == "" {
				continue
			}
			// Check if pod name or image matches CNI type
			for _, c := range pod.Spec.Containers {
				if strings.Contains(strings.ToLower(c.Image), result.Summary.CNIType) || strings.Contains(strings.ToLower(pod.Name), result.Summary.CNIType) {
					result.Summary.CNIPodsTotal++
					if pod.Status.Phase == corev1.PodRunning {
						result.Summary.CNIPodsReady++
					}
					break
				}
			}
		}
	}

	// Sort results
	sort.Slice(result.Nodes, func(i, j int) bool {
		return result.Nodes[i].Status > result.Nodes[j].Status
	})
	sort.Slice(result.Gaps, func(i, j int) bool {
		return result.Gaps[i].Severity > result.Gaps[j].Severity
	})

	// Recommendations
	if result.Summary.CNIType == "" {
		result.Recommendations = append(result.Recommendations,
			"CNI plugin not detected — ensure a CNI plugin (Calico, Cilium, Flannel) is installed")
	}
	if result.Summary.NodesWithoutCNI > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d nodes have no PodCIDR — verify CNI is running on all nodes", result.Summary.NodesWithoutCNI))
	}
	if result.Summary.NetworkNotReady > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d nodes have network unavailable — check CNI agent health", result.Summary.NetworkNotReady))
	}
	if result.Summary.CNIPodsTotal > 0 && result.Summary.CNIPodsReady < result.Summary.CNIPodsTotal {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d/%d CNI agent pods not ready — check CNI daemonset", result.Summary.CNIPodsTotal-result.Summary.CNIPodsReady, result.Summary.CNIPodsTotal))
	}

	// Health score
	score := 100
	if result.Summary.CNIType == "" {
		score -= 30
	}
	score -= result.Summary.NodesWithoutCNI * 5
	score -= result.Summary.NetworkNotReady * 15
	if result.Summary.CNIPodsTotal > 0 && result.Summary.CNIPodsReady < result.Summary.CNIPodsTotal {
		score -= 10
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score

	writeJSON(w, result)
}
