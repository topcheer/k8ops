package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v23.30 Documentation: Endpoint Ready vs NotReady, Node Allocatable CPU Core, Pod DNS Config Catalog
type EPReadyResult2330 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalEndpoints int `json:"totalEndpoints"`
		ReadyAddrs     int `json:"readyAddresses"`
		NotReadyAddrs  int `json:"notReadyAddresses"`
	} `json:"summary"`
}

func (s *Server) handleEPReady2330(w http.ResponseWriter, r *http.Request) {
	result := EPReadyResult2330{ScannedAt: time.Now()}
	epList, _ := s.clientset.CoreV1().Endpoints("").List(r.Context(), metav1.ListOptions{})
	for _, ep := range epList.Items {
		result.Summary.TotalEndpoints++
		for _, sub := range ep.Subsets {
			result.Summary.ReadyAddrs += len(sub.Addresses)
			result.Summary.NotReadyAddrs += len(sub.NotReadyAddresses)
		}
	}
	score := 100
	total := result.Summary.ReadyAddrs + result.Summary.NotReadyAddrs
	if total > 0 {
		score = result.Summary.ReadyAddrs * 100 / total
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

type NodeAllocCPUResult2330 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes int `json:"totalNodes"`
		TotalCPUs  int `json:"totalAllocatableCPUCores"`
	} `json:"summary"`
}

func (s *Server) handleNodeAllocCPU2330(w http.ResponseWriter, r *http.Request) {
	result := NodeAllocCPUResult2330{ScannedAt: time.Now()}
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		result.Summary.TotalCPUs += int(node.Status.Allocatable.Cpu().Value())
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type PodDNSConfigResult2330 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods     int `json:"totalPods"`
		WithDNSConfig int `json:"withDNSConfig"`
		WithDNSPolicy int `json:"withCustomDNSPolicy"`
	} `json:"summary"`
}

func (s *Server) handlePodDNSConfig2330(w http.ResponseWriter, r *http.Request) {
	result := PodDNSConfigResult2330{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		if pod.Spec.DNSConfig != nil {
			result.Summary.WithDNSConfig++
		}
		if pod.Spec.DNSPolicy != "" && pod.Spec.DNSPolicy != corev1.DNSClusterFirst {
			result.Summary.WithDNSPolicy++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
