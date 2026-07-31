package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v22.63 Security: Pod ServiceAccount Audit, Secret Type Distribution, PSP Equiv Pod Security Violations
type SvcAccountResult2263 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods        int            `json:"totalPods"`
		WithDefaultSA    int            `json:"withDefaultSA"`
		ByServiceAccount map[string]int `json:"byServiceAccount"`
	} `json:"summary"`
}

func (s *Server) handleSvcAccount2263(w http.ResponseWriter, r *http.Request) {
	result := SvcAccountResult2263{ScannedAt: time.Now()}
	result.Summary.ByServiceAccount = make(map[string]int)
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		sa := pod.Spec.ServiceAccountName
		if sa == "" {
			sa = "default"
		}
		if sa == "default" {
			result.Summary.WithDefaultSA++
		}
		result.Summary.ByServiceAccount[sa]++
	}
	score := 100
	if result.Summary.TotalPods > 0 {
		// Lower score if too many pods use default SA
		defaultPct := result.Summary.WithDefaultSA * 100 / result.Summary.TotalPods
		score = 100 - defaultPct/3
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

type SecretTypeResult2263 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalSecrets int            `json:"totalSecrets"`
		ByType       map[string]int `json:"byType"`
	} `json:"summary"`
}

func (s *Server) handleSecretType2263(w http.ResponseWriter, r *http.Request) {
	result := SecretTypeResult2263{ScannedAt: time.Now()}
	result.Summary.ByType = make(map[string]int)
	secretList, _ := s.clientset.CoreV1().Secrets("").List(r.Context(), metav1.ListOptions{})
	for _, secret := range secretList.Items {
		result.Summary.TotalSecrets++
		result.Summary.ByType[string(secret.Type)]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type PodSecViolationResult2263 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods             int `json:"totalPods"`
		HostNetwork           int `json:"hostNetwork"`
		HostPID               int `json:"hostPID"`
		HostIPC               int `json:"hostIPC"`
		ShareProcessNamespace int `json:"shareProcessNamespace"`
	} `json:"summary"`
}

func (s *Server) handlePodSecViolation2263(w http.ResponseWriter, r *http.Request) {
	result := PodSecViolationResult2263{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		if pod.Spec.HostNetwork {
			result.Summary.HostNetwork++
		}
		if pod.Spec.HostPID {
			result.Summary.HostPID++
		}
		if pod.Spec.HostIPC {
			result.Summary.HostIPC++
		}
		if pod.Spec.ShareProcessNamespace != nil && *pod.Spec.ShareProcessNamespace {
			result.Summary.ShareProcessNamespace++
		}
	}
	score := 100
	violations := result.Summary.HostNetwork + result.Summary.HostPID + result.Summary.HostIPC
	if result.Summary.TotalPods > 0 {
		score = 100 - (violations*50)/result.Summary.TotalPods
		if score < 0 {
			score = 0
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}
