package dashboard

import (
	"net/http"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v21.94 — Product Dimension (Round 52)
// 1. Pod Readiness Gate Type Audit
// 2. Container Stdin Once Distribution
// 3. Service ExternalTrafficPolicy Local Risk
// ============================================================

type ReadyGateTypeResult2194 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods  int            `json:"totalPods"`
		WithGates  int            `json:"withReadinessGates"`
		ByGateType map[string]int `json:"byGateType"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleReadyGateType2194(w http.ResponseWriter, r *http.Request) {
	result := ReadyGateTypeResult2194{ScannedAt: time.Now()}
	score := 100
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	result.Summary.ByGateType = make(map[string]int)
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		if len(pod.Spec.ReadinessGates) > 0 {
			result.Summary.WithGates++
			for _, gate := range pod.Spec.ReadinessGates {
				result.Summary.ByGateType[string(gate.ConditionType)]++
			}
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 2. Stdin Once Distribution
type StdinOnceDistResult2194 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalContainers int `json:"totalContainers"`
		WithStdinOnce   int `json:"withStdinOnce"`
		WithTTY         int `json:"withTTY"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleStdinOnceDist2194(w http.ResponseWriter, r *http.Request) {
	result := StdinOnceDistResult2194{ScannedAt: time.Now()}
	score := 100
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++
			if c.StdinOnce {
				result.Summary.WithStdinOnce++
			}
			if c.TTY {
				result.Summary.WithTTY++
			}
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 3. ExtTrafficPolicy Local Risk
type ExtTrafficRiskResult2194 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalServices int `json:"totalServices"`
		LocalRisk     int `json:"localPolicyServices"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleExtTrafficRisk2194(w http.ResponseWriter, r *http.Request) {
	result := ExtTrafficRiskResult2194{ScannedAt: time.Now()}
	score := 100
	svcList, _ := s.clientset.CoreV1().Services("").List(r.Context(), metav1.ListOptions{})
	for _, svc := range svcList.Items {
		if svc.Spec.Type != corev1.ServiceTypeLoadBalancer && svc.Spec.Type != corev1.ServiceTypeNodePort {
			continue
		}
		result.Summary.TotalServices++
		if svc.Spec.ExternalTrafficPolicy == corev1.ServiceExternalTrafficPolicyLocal {
			result.Summary.LocalRisk++
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}
