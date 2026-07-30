package dashboard

import (
	"net/http"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v22.30 — Product Dimension (Round 58)
// 1. Pod Resource Claim Tracker
// 2. Container Working Dir Audit
// 3. Service Publish NotReady Addresses Audit
// ============================================================

type ResClaimResult2230 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods  int `json:"totalPods"`
		WithClaims int `json:"withResourceClaims"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleResClaim2230(w http.ResponseWriter, r *http.Request) {
	result := ResClaimResult2230{ScannedAt: time.Now()}
	score := 100
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		if len(pod.Spec.ResourceClaims) > 0 {
			result.Summary.WithClaims++
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 2. Container Working Dir
type WorkingDirResult2230 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalContainers int            `json:"totalContainers"`
		WithCustomDir   int            `json:"withCustomWorkingDir"`
		ByDir           map[string]int `json:"byWorkingDir"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleWorkingDir2230(w http.ResponseWriter, r *http.Request) {
	result := WorkingDirResult2230{ScannedAt: time.Now()}
	score := 100
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	result.Summary.ByDir = make(map[string]int)
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++
			if c.WorkingDir != "" {
				result.Summary.WithCustomDir++
				result.Summary.ByDir[c.WorkingDir]++
			} else {
				result.Summary.ByDir["/ (default)"]++
			}
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 3. Publish NotReady Addresses
type PublishNotReadyResult2230 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalServices   int `json:"totalServices"`
		PublishNotReady int `json:"publishNotReadyAddresses"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handlePublishNotReady2230(w http.ResponseWriter, r *http.Request) {
	result := PublishNotReadyResult2230{ScannedAt: time.Now()}
	score := 100
	svcList, _ := s.clientset.CoreV1().Services("").List(r.Context(), metav1.ListOptions{})
	for _, svc := range svcList.Items {
		result.Summary.TotalServices++
		if svc.Spec.PublishNotReadyAddresses {
			result.Summary.PublishNotReady++
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}
