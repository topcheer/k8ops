package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v24.37 Security: Pod CapAdd Specific, Secret Rotation Stale, RoleBinding RoleRef Kind
type CapAddSpecificResult2437 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalContainers int `json:"totalContainers"`
		WithCapAdd      int `json:"withCapAdd"`
	} `json:"summary"`
}

func (s *Server) handleCapAddSpecific2437(w http.ResponseWriter, r *http.Request) {
	result := CapAddSpecificResult2437{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++
			if c.SecurityContext != nil && c.SecurityContext.Capabilities != nil && len(c.SecurityContext.Capabilities.Add) > 0 {
				result.Summary.WithCapAdd++
			}
		}
	}
	score := 100
	if result.Summary.TotalContainers > 0 && result.Summary.WithCapAdd > 0 {
		score = 100 - (result.Summary.WithCapAdd*50)/result.Summary.TotalContainers
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

type SecretRotationResult2437 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalSecrets int `json:"totalSecrets"`
		Stale90d     int `json:"stale90d"`
	} `json:"summary"`
}

func (s *Server) handleSecretRotation2437(w http.ResponseWriter, r *http.Request) {
	result := SecretRotationResult2437{ScannedAt: time.Now()}
	secretList, _ := s.clientset.CoreV1().Secrets("").List(r.Context(), metav1.ListOptions{})
	now := time.Now()
	for _, secret := range secretList.Items {
		result.Summary.TotalSecrets++
		if secret.CreationTimestamp.Time.Before(now.AddDate(0, -3, 0)) {
			result.Summary.Stale90d++
		}
	}
	score := 100
	if result.Summary.TotalSecrets > 0 {
		score = 100 - (result.Summary.Stale90d*30)/result.Summary.TotalSecrets
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

type RBRoleRefKindResult2437 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalRB int            `json:"totalRoleBindings"`
		ByKind  map[string]int `json:"byRoleRefKind"`
	} `json:"summary"`
}

func (s *Server) handleRBRoleRefKind2437(w http.ResponseWriter, r *http.Request) {
	result := RBRoleRefKindResult2437{ScannedAt: time.Now()}
	result.Summary.ByKind = make(map[string]int)
	rbList, _ := s.clientset.RbacV1().RoleBindings("").List(r.Context(), metav1.ListOptions{})
	for _, rb := range rbList.Items {
		result.Summary.TotalRB++
		result.Summary.ByKind[rb.RoleRef.Kind]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
