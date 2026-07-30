package dashboard

import (
	"fmt"
	"net/http"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v21.64 — Product Dimension (Round 47)
// 1. Pod Image Pull Policy Audit
// 2. Container Termination Message Path
// 3. Service HealthProbe Proxy
// ============================================================

type PullPolicyResult2164 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalContainers int            `json:"totalContainers"`
		ByPolicy        map[string]int `json:"byPullPolicy"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handlePullPolicy2164(w http.ResponseWriter, r *http.Request) {
	result := PullPolicyResult2164{ScannedAt: time.Now()}
	score := 100
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	result.Summary.ByPolicy = make(map[string]int)
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++
			policy := string(c.ImagePullPolicy)
			if policy == "" {
				policy = "IfNotPresent"
			}
			result.Summary.ByPolicy[policy]++
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 2. Termination Message Path
type TermMsgPathResult2164 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalContainers int `json:"totalContainers"`
		WithMsgPath     int `json:"withTerminationMessagePath"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleTermMsgPath2164(w http.ResponseWriter, r *http.Request) {
	result := TermMsgPathResult2164{ScannedAt: time.Now()}
	score := 100
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++
			if c.TerminationMessagePath != "" {
				result.Summary.WithMsgPath++
			}
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 3. HealthProbe Proxy
type HealthProxyResult2164 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalServices int `json:"totalServices"`
		WithProxy     int `json:"withHealthProbeProxy"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleHealthProxy2164(w http.ResponseWriter, r *http.Request) {
	result := HealthProxyResult2164{ScannedAt: time.Now()}
	score := 100
	svcList, _ := s.clientset.CoreV1().Services("").List(r.Context(), metav1.ListOptions{})
	for _, svc := range svcList.Items {
		result.Summary.TotalServices++
		if svc.Spec.HealthCheckNodePort > 0 {
			result.Summary.WithProxy++
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	if result.Summary.TotalServices > 0 && result.Summary.WithProxy == 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("No services with health probe proxy"))
	}
	writeJSON(w, result)
}
