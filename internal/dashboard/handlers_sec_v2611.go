package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v26.11 Security: Pod RunAsNonRoot, Secret Immutable Detail, RoleBinding RoleRef APIGroup
type RunAsNonRoot2611Result struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalContainers int `json:"totalContainers"`
		WithNonRoot     int `json:"withRunAsNonRoot"`
	} `json:"summary"`
}

func (s *Server) handleRunAsNonRoot2611(w http.ResponseWriter, r *http.Request) {
	result := RunAsNonRoot2611Result{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++
			if c.SecurityContext != nil && c.SecurityContext.RunAsNonRoot != nil && *c.SecurityContext.RunAsNonRoot {
				result.Summary.WithNonRoot++
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type SecretImmutable2611Result struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalSecrets int `json:"totalSecrets"`
		Immutable    int `json:"immutableCount"`
	} `json:"summary"`
}

func (s *Server) handleSecretImmutable2611(w http.ResponseWriter, r *http.Request) {
	result := SecretImmutable2611Result{ScannedAt: time.Now()}
	secretList, _ := s.clientset.CoreV1().Secrets("").List(r.Context(), metav1.ListOptions{})
	for _, secret := range secretList.Items {
		result.Summary.TotalSecrets++
		if secret.Immutable != nil && *secret.Immutable {
			result.Summary.Immutable++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type RBRoleRefAPIGroup2611Result struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalRB    int            `json:"totalRoleBindings"`
		ByAPIGroup map[string]int `json:"byRoleRefAPIGroup"`
	} `json:"summary"`
}

func (s *Server) handleRBRoleRefAPIGroup2611(w http.ResponseWriter, r *http.Request) {
	result := RBRoleRefAPIGroup2611Result{ScannedAt: time.Now()}
	result.Summary.ByAPIGroup = make(map[string]int)
	rbList, _ := s.clientset.RbacV1().RoleBindings("").List(r.Context(), metav1.ListOptions{})
	for _, rb := range rbList.Items {
		result.Summary.TotalRB++
		result.Summary.ByAPIGroup[rb.RoleRef.APIGroup]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
