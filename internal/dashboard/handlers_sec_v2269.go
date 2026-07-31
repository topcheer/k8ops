package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v22.69 Security: RunAsNonRoot Audit, Service Account Token Auto-Mount, Pod HostPath Mount Audit
type NonRootResult2269 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalContainers int `json:"totalContainers"`
		NonRootEnforced int `json:"nonRootEnforced"`
	} `json:"summary"`
}

func (s *Server) handleNonRoot2269(w http.ResponseWriter, r *http.Request) {
	result := NonRootResult2269{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++
			if c.SecurityContext != nil && c.SecurityContext.RunAsNonRoot != nil && *c.SecurityContext.RunAsNonRoot {
				result.Summary.NonRootEnforced++
			}
		}
	}
	score := 100
	if result.Summary.TotalContainers > 0 {
		score = result.Summary.NonRootEnforced * 100 / result.Summary.TotalContainers
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

type SATokenMountResult2269 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods        int `json:"totalPods"`
		TokenAutoMounted int `json:"tokenAutoMounted"`
	} `json:"summary"`
}

func (s *Server) handleSATokenMount2269(w http.ResponseWriter, r *http.Request) {
	result := SATokenMountResult2269{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		if pod.Spec.AutomountServiceAccountToken == nil || *pod.Spec.AutomountServiceAccountToken {
			result.Summary.TokenAutoMounted++
		}
	}
	score := 100
	if result.Summary.TotalPods > 0 {
		mountedPct := result.Summary.TokenAutoMounted * 100 / result.Summary.TotalPods
		score = 100 - mountedPct/4
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

type HostPathResult2269 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods    int `json:"totalPods"`
		WithHostPath int `json:"withHostPath"`
		TotalMounts  int `json:"totalMounts"`
	} `json:"summary"`
}

func (s *Server) handleHostPath2269(w http.ResponseWriter, r *http.Request) {
	result := HostPathResult2269{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		hasHostPath := false
		for _, vol := range pod.Spec.Volumes {
			if vol.HostPath != nil {
				hasHostPath = true
				result.Summary.TotalMounts++
			}
		}
		if hasHostPath {
			result.Summary.WithHostPath++
		}
	}
	score := 100
	if result.Summary.TotalPods > 0 && result.Summary.WithHostPath > 0 {
		score = 100 - (result.Summary.WithHostPath*50)/result.Summary.TotalPods
		if score < 0 {
			score = 0
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}
