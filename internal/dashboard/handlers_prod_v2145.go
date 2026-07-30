package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v21.45 — Product Dimension (Round 44)
// 1. Pod Overhead Resource Audit
// 2. Container Stdin Once Audit
// 3. Service Allocate LoadBalancer NodePorts
// ============================================================

type OverheadResult2145 struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	HealthScore     int                 `json:"healthScore"`
	Grade           string              `json:"grade"`
	Summary         OverheadSummary2145 `json:"summary"`
	Recommendations []string            `json:"recommendations"`
}

type OverheadSummary2145 struct {
	TotalPods    int `json:"totalPods"`
	WithOverhead int `json:"withOverhead"`
}

func (s *Server) handleOverhead2145(w http.ResponseWriter, r *http.Request) {
	result := OverheadResult2145{ScannedAt: time.Now()}
	score := 100
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		if pod.Spec.Overhead != nil && len(pod.Spec.Overhead) > 0 {
			result.Summary.WithOverhead++
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 2. Stdin Once Audit
type StdinOnceResult2145 struct {
	ScannedAt       time.Time            `json:"scannedAt"`
	HealthScore     int                  `json:"healthScore"`
	Grade           string               `json:"grade"`
	Summary         StdinOnceSummary2145 `json:"summary"`
	AtRisk          []StdinOnceEntry2145 `json:"atRiskContainers"`
	Recommendations []string             `json:"recommendations"`
}

type StdinOnceSummary2145 struct {
	TotalContainers int `json:"totalContainers"`
	StdinOnce       int `json:"stdinOnce"`
}

type StdinOnceEntry2145 struct {
	Pod       string `json:"pod"`
	Namespace string `json:"namespace"`
}

func (s *Server) handleStdinOnce2145(w http.ResponseWriter, r *http.Request) {
	result := StdinOnceResult2145{ScannedAt: time.Now()}
	score := 100
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++
			if c.StdinOnce {
				result.Summary.StdinOnce++
				result.AtRisk = append(result.AtRisk, StdinOnceEntry2145{Pod: pod.Name, Namespace: pod.Namespace})
			}
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	sort.Slice(result.AtRisk, func(i, j int) bool { return result.AtRisk[i].Namespace < result.AtRisk[j].Namespace })
	writeJSON(w, result)
}

// 3. Allocate LB NodePorts
type AllocLBResult2145 struct {
	ScannedAt       time.Time          `json:"scannedAt"`
	HealthScore     int                `json:"healthScore"`
	Grade           string             `json:"grade"`
	Summary         AllocLBSummary2145 `json:"summary"`
	Recommendations []string           `json:"recommendations"`
}

type AllocLBSummary2145 struct {
	TotalLB      int `json:"totalLoadBalancers"`
	WithNodePort int `json:"allocateNodePorts"`
}

func (s *Server) handleAllocLB2145(w http.ResponseWriter, r *http.Request) {
	result := AllocLBResult2145{ScannedAt: time.Now()}
	score := 100
	svcList, _ := s.clientset.CoreV1().Services("").List(r.Context(), metav1.ListOptions{})

	for _, svc := range svcList.Items {
		if svc.Spec.Type != corev1.ServiceTypeLoadBalancer {
			continue
		}
		result.Summary.TotalLB++
		if svc.Spec.AllocateLoadBalancerNodePorts == nil || *svc.Spec.AllocateLoadBalancerNodePorts {
			result.Summary.WithNodePort++
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)

	if result.Summary.WithNodePort > 5 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d LB services allocate node ports — disable if not needed", result.Summary.WithNodePort))
	}
	writeJSON(w, result)
}
