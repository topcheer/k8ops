package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v25.86 Operations: Node MachineID Dist, Pod Spec PodCIDR, Container Resource CPU Limit Detail
type NodeMachineIDResult2586 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes int `json:"totalNodes"`
		UniqueIDs  int `json:"uniqueMachineIDs"`
	}
}

func (s *Server) handleNodeMachineID2586(w http.ResponseWriter, r *http.Request) {
	result := NodeMachineIDResult2586{ScannedAt: time.Now()}
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	seen := make(map[string]bool)
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		mid := node.Status.NodeInfo.MachineID
		if mid != "" && !seen[mid] {
			seen[mid] = true
			result.Summary.UniqueIDs++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type PodCIDRResult2586 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes int            `json:"totalNodes"`
		ByCIDR     map[string]int `json:"byPodCIDR"`
	}
}

func (s *Server) handlePodCIDR2586(w http.ResponseWriter, r *http.Request) {
	result := PodCIDRResult2586{ScannedAt: time.Now()}
	result.Summary.ByCIDR = make(map[string]int)
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		cidr := node.Spec.PodCIDR
		if cidr == "" {
			cidr = "<none>"
		}
		result.Summary.ByCIDR[cidr]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type CPULimitDetailResult2586 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalContainers int     `json:"totalContainers"`
		TotalCPULimit   float64 `json:"totalCPULimitCores"`
	}
}

func (s *Server) handleCPULimitDetail2586(w http.ResponseWriter, r *http.Request) {
	result := CPULimitDetailResult2586{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++
			result.Summary.TotalCPULimit += c.Resources.Limits.Cpu().AsApproximateFloat64()
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
