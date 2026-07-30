package dashboard

import (
	"fmt"
	"net/http"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v21.58 — Product Dimension (Round 46)
// 1. Pod Topology Spread Constraints Audit
// 2. Service ExternalName Health
// 3. Container Capability Drop Audit
// ============================================================

type TopoSpreadAuditResult2158 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods  int `json:"totalPods"`
		WithSpread int `json:"withTopologySpreadConstraints"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleTopoSpreadAudit2158(w http.ResponseWriter, r *http.Request) {
	result := TopoSpreadAuditResult2158{ScannedAt: time.Now()}
	score := 100
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		if len(pod.Spec.TopologySpreadConstraints) > 0 {
			result.Summary.WithSpread++
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 2. ExternalName Health
type ExtNameResult2158 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalServices int `json:"totalServices"`
		ExternalName  int `json:"externalNameServices"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleExtName2158(w http.ResponseWriter, r *http.Request) {
	result := ExtNameResult2158{ScannedAt: time.Now()}
	score := 100
	svcList, _ := s.clientset.CoreV1().Services("").List(r.Context(), metav1.ListOptions{})
	for _, svc := range svcList.Items {
		result.Summary.TotalServices++
		if svc.Spec.Type == corev1.ServiceTypeExternalName {
			result.Summary.ExternalName++
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 3. Capability Drop Audit
type CapDropResult2158 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalContainers int `json:"totalContainers"`
		WithCapDrop     int `json:"withCapabilityDrop"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleCapDrop2158(w http.ResponseWriter, r *http.Request) {
	result := CapDropResult2158{ScannedAt: time.Now()}
	score := 100
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++
			if c.SecurityContext != nil && len(c.SecurityContext.Capabilities.Drop) > 0 {
				result.Summary.WithCapDrop++
			}
		}
	}
	if result.Summary.WithCapDrop < result.Summary.TotalContainers/2 && result.Summary.TotalContainers > 0 {
		score -= 10
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	if result.Summary.WithCapDrop < result.Summary.TotalContainers {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d/%d containers missing capability drops", result.Summary.TotalContainers-result.Summary.WithCapDrop, result.Summary.TotalContainers))
	}
	writeJSON(w, result)
}
