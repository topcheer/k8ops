package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v26.16 Operations: Node ProviderID Dist, Pod Spec ShareProcessNamespace, Container Resource Limit Memory Detail
type NodeProviderID2616Result struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes int            `json:"totalNodes"`
		ByProvider map[string]int `json:"byProviderID"`
	} `json:"summary"`
}

func (s *Server) handleNodeProviderID2616(w http.ResponseWriter, r *http.Request) {
	result := NodeProviderID2616Result{ScannedAt: time.Now()}
	result.Summary.ByProvider = make(map[string]int)
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		pid := node.Spec.ProviderID
		if pid == "" {
			pid = "<none>"
		}
		// Group by provider type prefix
		colonIdx := -1
		for i, ch := range pid {
			if ch == ':' {
				colonIdx = i
				break
			}
		}
		if colonIdx > 0 {
			pid = pid[:colonIdx]
		}
		result.Summary.ByProvider[pid]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type PodShareProcNS2616Result struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods int `json:"totalPods"`
		WithShare int `json:"withShareProcessNamespace"`
	} `json:"summary"`
}

func (s *Server) handlePodShareProcNS2616(w http.ResponseWriter, r *http.Request) {
	result := PodShareProcNS2616Result{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		if pod.Spec.ShareProcessNamespace != nil && *pod.Spec.ShareProcessNamespace {
			result.Summary.WithShare++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type MemLimitDetail2616Result struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalContainers int     `json:"totalContainers"`
		TotalMemLim     float64 `json:"totalMemLimitMB"`
	} `json:"summary"`
}

func (s *Server) handleMemLimitDetail2616(w http.ResponseWriter, r *http.Request) {
	result := MemLimitDetail2616Result{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++
			result.Summary.TotalMemLim += c.Resources.Limits.Memory().AsApproximateFloat64() / (1024 * 1024)
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
