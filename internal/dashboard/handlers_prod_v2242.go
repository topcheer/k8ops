package dashboard

import (
	"net/http"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v22.42 — Product Dimension (Round 60)
// 1. Pod Overhead Resource Distribution
// 2. Container Probe Timeout Audit
// 3. Service LoadBalancer Class Tracker
// ============================================================

type OverheadDistResult2242 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods    int `json:"totalPods"`
		WithOverhead int `json:"withOverhead"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleOverheadDist2242(w http.ResponseWriter, r *http.Request) {
	result := OverheadDistResult2242{ScannedAt: time.Now()}
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

// 2. Container Probe Timeout
type ProbeTimeoutResult2242 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalProbes   int `json:"totalProbes"`
		WithTimeout   int `json:"withTimeoutSeconds"`
		AvgTimeoutSec int `json:"avgTimeoutSeconds"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleProbeTimeout2242(w http.ResponseWriter, r *http.Request) {
	result := ProbeTimeoutResult2242{ScannedAt: time.Now()}
	score := 100
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	totalTimeout := 0
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			if c.LivenessProbe != nil {
				result.Summary.TotalProbes++
				if c.LivenessProbe.TimeoutSeconds > 0 {
					result.Summary.WithTimeout++
					totalTimeout += int(c.LivenessProbe.TimeoutSeconds)
				}
			}
		}
	}
	if result.Summary.WithTimeout > 0 {
		result.Summary.AvgTimeoutSec = totalTimeout / result.Summary.WithTimeout
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 3. LB Class Tracker
type LBClassResult2242 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalLB int            `json:"totalLoadBalancers"`
		ByClass map[string]int `json:"byLoadBalancerClass"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleLBClass2242(w http.ResponseWriter, r *http.Request) {
	result := LBClassResult2242{ScannedAt: time.Now()}
	score := 100
	svcList, _ := s.clientset.CoreV1().Services("").List(r.Context(), metav1.ListOptions{})
	result.Summary.ByClass = make(map[string]int)
	for _, svc := range svcList.Items {
		if svc.Spec.Type != corev1.ServiceTypeLoadBalancer {
			continue
		}
		result.Summary.TotalLB++
		if svc.Spec.LoadBalancerClass != nil {
			result.Summary.ByClass[*svc.Spec.LoadBalancerClass]++
		} else {
			result.Summary.ByClass["default"]++
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}
