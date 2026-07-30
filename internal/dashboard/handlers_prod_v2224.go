package dashboard

import (
	"net/http"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v22.24 — Product Dimension (Round 57)
// 1. Pod Container Command Audit
// 2. Service IPFamilyPolicy Catalog
// 3. Pod OSDomain Join Status
// ============================================================

type CtnrCmdResult2224 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalContainers int `json:"totalContainers"`
		WithCommand     int `json:"withCommand"`
		AvgCmdLen       int `json:"avgCommandLength"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleCtnrCmd2224(w http.ResponseWriter, r *http.Request) {
	result := CtnrCmdResult2224{ScannedAt: time.Now()}
	score := 100
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	totalLen := 0
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++
			if len(c.Command) > 0 {
				result.Summary.WithCommand++
			}
			totalLen += len(c.Command)
		}
	}
	if result.Summary.WithCommand > 0 {
		result.Summary.AvgCmdLen = totalLen / result.Summary.WithCommand
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 2. Service IPFamilyPolicy Catalog
type SvcIPFamResult2224 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalServices int            `json:"totalServices"`
		ByIPFamily    map[string]int `json:"byIPFamilyPolicy"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleSvcIPFam2224(w http.ResponseWriter, r *http.Request) {
	result := SvcIPFamResult2224{ScannedAt: time.Now()}
	score := 100
	svcList, _ := s.clientset.CoreV1().Services("").List(r.Context(), metav1.ListOptions{})
	result.Summary.ByIPFamily = make(map[string]int)
	for _, svc := range svcList.Items {
		result.Summary.TotalServices++
		if svc.Spec.IPFamilyPolicy != nil {
			result.Summary.ByIPFamily[string(*svc.Spec.IPFamilyPolicy)]++
		} else {
			result.Summary.ByIPFamily["default"]++
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 3. OSDomain Join Status
type OSDomainResult2224 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods  int `json:"totalPods"`
		WithDomain int `json:"withOSDomain"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleOSDomain2224(w http.ResponseWriter, r *http.Request) {
	result := OSDomainResult2224{ScannedAt: time.Now()}
	score := 100
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		if pod.Spec.Subdomain != "" {
			result.Summary.WithDomain++
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}
