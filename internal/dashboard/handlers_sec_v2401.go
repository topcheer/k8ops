package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v24.01 Security: Pod Privileged Container, Secret Type Opaque, RoleBinding Subjects Count
type PrivilegedResult2401 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalContainers int `json:"totalContainers"`
		Privileged      int `json:"privileged"`
	} `json:"summary"`
}

func (s *Server) handlePrivileged2401(w http.ResponseWriter, r *http.Request) {
	result := PrivilegedResult2401{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++
			if c.SecurityContext != nil && c.SecurityContext.Privileged != nil && *c.SecurityContext.Privileged {
				result.Summary.Privileged++
			}
		}
	}
	score := 100
	if result.Summary.TotalContainers > 0 && result.Summary.Privileged > 0 {
		score = 100 - (result.Summary.Privileged*80)/result.Summary.TotalContainers
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

type SecretOpaqueResult2401 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalSecrets  int `json:"totalSecrets"`
		OpaqueSecrets int `json:"opaqueSecrets"`
	} `json:"summary"`
}

func (s *Server) handleSecretOpaque2401(w http.ResponseWriter, r *http.Request) {
	result := SecretOpaqueResult2401{ScannedAt: time.Now()}
	secretList, _ := s.clientset.CoreV1().Secrets("").List(r.Context(), metav1.ListOptions{})
	for _, secret := range secretList.Items {
		result.Summary.TotalSecrets++
		if secret.Type == corev1.SecretTypeOpaque {
			result.Summary.OpaqueSecrets++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type RBSubjectsResult2401 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalRB       int `json:"totalRoleBindings"`
		TotalSubjects int `json:"totalSubjects"`
	} `json:"summary"`
}

func (s *Server) handleRBSubjects2401(w http.ResponseWriter, r *http.Request) {
	result := RBSubjectsResult2401{ScannedAt: time.Now()}
	rbList, _ := s.clientset.RbacV1().RoleBindings("").List(r.Context(), metav1.ListOptions{})
	for _, rb := range rbList.Items {
		result.Summary.TotalRB++
		result.Summary.TotalSubjects += len(rb.Subjects)
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
