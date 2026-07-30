package dashboard

import (
	"net/http"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v22.21 — Security Dimension (Round 56)
// 1. Pod HostIPC Audit
// 2. Namespace PSA Enforce Level Catalog
// 3. RoleBinding Aggregation Rule Count
// ============================================================

type HostIPCResult2221 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods   int `json:"totalPods"`
		WithHostIPC int `json:"withHostIPC"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleHostIPC2221(w http.ResponseWriter, r *http.Request) {
	result := HostIPCResult2221{ScannedAt: time.Now()}
	score := 100
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		if pod.Spec.HostIPC {
			result.Summary.WithHostIPC++
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 2. PSA Enforce Level
type PSAEnforceResult2221 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNS int            `json:"totalNamespaces"`
		ByLevel map[string]int `json:"byEnforceLevel"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handlePSAEnforce2221(w http.ResponseWriter, r *http.Request) {
	result := PSAEnforceResult2221{ScannedAt: time.Now()}
	score := 100
	nsList, _ := s.clientset.CoreV1().Namespaces().List(r.Context(), metav1.ListOptions{})
	result.Summary.ByLevel = make(map[string]int)
	for _, ns := range nsList.Items {
		result.Summary.TotalNS++
		level := ns.Labels["pod-security.kubernetes.io/enforce"]
		if level == "" {
			level = "none"
		}
		result.Summary.ByLevel[level]++
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 3. RB Aggregation Count
type RBAggCountResult2221 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalRB    int `json:"totalRoleBindings"`
		Aggregated int `json:"aggregated"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleRBAggCount2221(w http.ResponseWriter, r *http.Request) {
	result := RBAggCountResult2221{ScannedAt: time.Now()}
	score := 100
	rbList, _ := s.clientset.RbacV1().RoleBindings("").List(r.Context(), metav1.ListOptions{})
	for range rbList.Items {
		result.Summary.TotalRB++
	}
	crList, _ := s.clientset.RbacV1().ClusterRoles().List(r.Context(), metav1.ListOptions{})
	for _, cr := range crList.Items {
		if cr.AggregationRule != nil {
			result.Summary.Aggregated++
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}
