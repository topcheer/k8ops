package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v25.82 Documentation: Node Allocatable Storage vs Capacity, Pod Spec Container Port Summary, Namespace Age Distribution
type NodeStorVsCapResult2582 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes int     `json:"totalNodes"`
		TotalCap   float64 `json:"totalCapStorGB"`
		TotalAlloc float64 `json:"totalAllocStorGB"`
	}
}

func (s *Server) handleNodeStorVsCap2582(w http.ResponseWriter, r *http.Request) {
	result := NodeStorVsCapResult2582{ScannedAt: time.Now()}
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		result.Summary.TotalCap += node.Status.Capacity.StorageEphemeral().AsApproximateFloat64() / (1024 * 1024 * 1024)
		result.Summary.TotalAlloc += node.Status.Allocatable.StorageEphemeral().AsApproximateFloat64() / (1024 * 1024 * 1024)
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type CtnrPortSummaryResult2582 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalContainers int            `json:"totalContainers"`
		ByProtocol      map[string]int `json:"byPortProtocol"`
	}
}

func (s *Server) handleCtnrPortSummary2582(w http.ResponseWriter, r *http.Request) {
	result := CtnrPortSummaryResult2582{ScannedAt: time.Now()}
	result.Summary.ByProtocol = make(map[string]int)
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++
			for _, p := range c.Ports {
				result.Summary.ByProtocol[string(p.Protocol)]++
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type NSAgedistResult2582 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNS    int   `json:"totalNamespaces"`
		MaxAgeDays int64 `json:"maxAgeDays"`
	}
}

func (s *Server) handleNSAgeDist2582(w http.ResponseWriter, r *http.Request) {
	result := NSAgedistResult2582{ScannedAt: time.Now()}
	nsList, _ := s.clientset.CoreV1().Namespaces().List(r.Context(), metav1.ListOptions{})
	now := time.Now()
	var maxAge time.Duration
	for _, ns := range nsList.Items {
		result.Summary.TotalNS++
		age := now.Sub(ns.CreationTimestamp.Time)
		if age > maxAge {
			maxAge = age
		}
	}
	result.Summary.MaxAgeDays = int64(maxAge.Hours() / 24)
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
