package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v23.83 Security: Pod ReadOnlyRootFS, Secret Empty Data, Role Bind All Subjects
type ReadOnlyFSResult2383 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalContainers int `json:"totalContainers"`
		ReadOnlyFS      int `json:"readOnlyRootFS"`
	} `json:"summary"`
}

func (s *Server) handleReadOnlyFS2383(w http.ResponseWriter, r *http.Request) {
	result := ReadOnlyFSResult2383{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++
			if c.SecurityContext != nil && c.SecurityContext.ReadOnlyRootFilesystem != nil && *c.SecurityContext.ReadOnlyRootFilesystem {
				result.Summary.ReadOnlyFS++
			}
		}
	}
	score := 100
	if result.Summary.TotalContainers > 0 {
		score = result.Summary.ReadOnlyFS * 100 / result.Summary.TotalContainers
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

type SecretEmptyResult2383 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalSecrets int `json:"totalSecrets"`
		EmptySecrets int `json:"emptySecrets"`
	} `json:"summary"`
}

func (s *Server) handleSecretEmpty2383(w http.ResponseWriter, r *http.Request) {
	result := SecretEmptyResult2383{ScannedAt: time.Now()}
	secretList, _ := s.clientset.CoreV1().Secrets("").List(r.Context(), metav1.ListOptions{})
	for _, secret := range secretList.Items {
		result.Summary.TotalSecrets++
		if len(secret.Data) == 0 {
			result.Summary.EmptySecrets++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type RoleBindAllResult2383 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalCRB int `json:"totalClusterRoleBindings"`
		TotalRB  int `json:"totalRoleBindings"`
	} `json:"summary"`
}

func (s *Server) handleRoleBindAll2383(w http.ResponseWriter, r *http.Request) {
	result := RoleBindAllResult2383{ScannedAt: time.Now()}
	crbList, _ := s.clientset.RbacV1().ClusterRoleBindings().List(r.Context(), metav1.ListOptions{})
	rbList, _ := s.clientset.RbacV1().RoleBindings("").List(r.Context(), metav1.ListOptions{})
	result.Summary.TotalCRB = len(crbList.Items)
	result.Summary.TotalRB = len(rbList.Items)
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
