package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v26.28 Operations: Node KubeProxyVersion Dist, Pod Spec DNSSearchDomains, Container CPU Request vs Limit Ratio
type NodeKubeProxy2628Result struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes int            `json:"totalNodes"`
		ByVersion  map[string]int `json:"byKubeProxyVersion"`
	} `json:"summary"`
}

func (s *Server) handleNodeKubeProxy2628(w http.ResponseWriter, r *http.Request) {
	result := NodeKubeProxy2628Result{ScannedAt: time.Now()}
	result.Summary.ByVersion = make(map[string]int)
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		kv := node.Status.NodeInfo.KubeProxyVersion
		if kv == "" {
			kv = "<unknown>"
		}
		result.Summary.ByVersion[kv]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type PodDNSSearch2628Result struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods int `json:"totalPods"`
		WithDNS   int `json:"withDNSSearchDomains"`
	} `json:"summary"`
}

func (s *Server) handlePodDNSSearch2628(w http.ResponseWriter, r *http.Request) {
	result := PodDNSSearch2628Result{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		if pod.Spec.DNSConfig != nil && len(pod.Spec.DNSConfig.Searches) > 0 {
			result.Summary.WithDNS++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type CPUReqVsLimRatio2628Result struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalContainers int `json:"totalContainers"`
		WithBothReqLim  int `json:"withBothReqAndLim"`
	} `json:"summary"`
}

func (s *Server) handleCPUReqVsLimRatio2628(w http.ResponseWriter, r *http.Request) {
	result := CPUReqVsLimRatio2628Result{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++
			req := c.Resources.Requests.Cpu().AsApproximateFloat64()
			lim := c.Resources.Limits.Cpu().AsApproximateFloat64()
			if req > 0 && lim > 0 {
				result.Summary.WithBothReqLim++
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
