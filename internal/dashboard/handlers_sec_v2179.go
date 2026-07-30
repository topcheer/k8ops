package dashboard

import (
	"net/http"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v21.79 — Security Dimension (Round 49)
// 1. Pod SecurityContext RunAsGroup Audit
// 2. Secret Data Size Audit
// 3. ClusterRole Rule Resource Star Audit
// ============================================================

type RunAsGroupResult2179 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods      int `json:"totalPods"`
		WithRunAsGroup int `json:"withRunAsGroup"`
		WithRootGroup  int `json:"withRootGroup"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleRunAsGroup2179(w http.ResponseWriter, r *http.Request) {
	result := RunAsGroupResult2179{ScannedAt: time.Now()}
	score := 100
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		if pod.Spec.SecurityContext != nil && pod.Spec.SecurityContext.RunAsGroup != nil {
			result.Summary.WithRunAsGroup++
			if *pod.Spec.SecurityContext.RunAsGroup == 0 {
				result.Summary.WithRootGroup++
			}
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 2. Secret Data Size Audit
type SecretSizeResult2179 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalSecrets int `json:"totalSecrets"`
		TotalSizeKB  int `json:"totalSizeKB"`
		MaxSizeKB    int `json:"maxSizeKB"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleSecretSize2179(w http.ResponseWriter, r *http.Request) {
	result := SecretSizeResult2179{ScannedAt: time.Now()}
	score := 100
	secretList, _ := s.clientset.CoreV1().Secrets("").List(r.Context(), metav1.ListOptions{})
	for _, secret := range secretList.Items {
		result.Summary.TotalSecrets++
		sizeKB := 0
		for _, v := range secret.Data {
			sizeKB += len(v) / 1024
		}
		result.Summary.TotalSizeKB += sizeKB
		if sizeKB > result.Summary.MaxSizeKB {
			result.Summary.MaxSizeKB = sizeKB
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 3. CR Resource Star Audit
type CRResourceStarResult2179 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalCR     int `json:"totalClusterRoles"`
		WithStarRes int `json:"withStarResource"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleCRResourceStar2179(w http.ResponseWriter, r *http.Request) {
	result := CRResourceStarResult2179{ScannedAt: time.Now()}
	score := 100
	crList, _ := s.clientset.RbacV1().ClusterRoles().List(r.Context(), metav1.ListOptions{})
	for _, cr := range crList.Items {
		result.Summary.TotalCR++
		for _, rule := range cr.Rules {
			for _, res := range rule.Resources {
				if res == "*" {
					result.Summary.WithStarRes++
					break
				}
			}
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}
