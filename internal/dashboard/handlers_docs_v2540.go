package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v25.40 Documentation: Node Allocatable vs Capacity CPU, Pod Spec DNSPolicy Detail, Namespace Spec Finalizer Summary
type NodeAllocVsCapCPUResult2540 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes int     `json:"totalNodes"`
		TotalAlloc float64 `json:"totalAllocatableCPU"`
		TotalCap   float64 `json:"totalCapacityCPU"`
	} `json:"summary"`
}

func (s *Server) handleNodeAllocVsCapCPU2540(w http.ResponseWriter, r *http.Request) {
	result := NodeAllocVsCapCPUResult2540{ScannedAt: time.Now()}
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		result.Summary.TotalAlloc += node.Status.Allocatable.Cpu().AsApproximateFloat64()
		result.Summary.TotalCap += node.Status.Capacity.Cpu().AsApproximateFloat64()
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type DNSPolicyDetailResult2540 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods int            `json:"totalPods"`
		ByPolicy  map[string]int `json:"byDNSPolicy"`
	} `json:"summary"`
}

func (s *Server) handleDNSPolicyDetail2540(w http.ResponseWriter, r *http.Request) {
	result := DNSPolicyDetailResult2540{ScannedAt: time.Now()}
	result.Summary.ByPolicy = make(map[string]int)
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		result.Summary.ByPolicy[string(pod.Spec.DNSPolicy)]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type NSFinalizerSummaryResult2540 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNS   int            `json:"totalNamespaces"`
		WithFinal map[string]int `json:"byFinalizer"`
	} `json:"summary"`
}

func (s *Server) handleNSFinalizerSummary2540(w http.ResponseWriter, r *http.Request) {
	result := NSFinalizerSummaryResult2540{ScannedAt: time.Now()}
	result.Summary.WithFinal = make(map[string]int)
	nsList, _ := s.clientset.CoreV1().Namespaces().List(r.Context(), metav1.ListOptions{})
	for _, ns := range nsList.Items {
		result.Summary.TotalNS++
		for _, f := range ns.Spec.Finalizers {
			result.Summary.WithFinal[string(f)]++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
