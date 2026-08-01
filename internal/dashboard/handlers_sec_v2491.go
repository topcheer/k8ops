package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v24.91 Security: Pod RunAsGroup, Secret AuthToken Count, RoleBinding API Groups Summary
type RunAsGroupResult2491 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalContainers int `json:"totalContainers"`
		WithGroup       int `json:"withRunAsGroup"`
	} `json:"summary"`
}

func (s *Server) handleRunAsGroup2491(w http.ResponseWriter, r *http.Request) {
	result := RunAsGroupResult2491{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++
			if c.SecurityContext != nil && c.SecurityContext.RunAsGroup != nil {
				result.Summary.WithGroup++
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type SecretAuthTokenResult2491 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalSecrets int `json:"totalSecrets"`
		TotalKeys    int `json:"totalStringDataKeys"`
	} `json:"summary"`
}

func (s *Server) handleSecretAuthToken2491(w http.ResponseWriter, r *http.Request) {
	result := SecretAuthTokenResult2491{ScannedAt: time.Now()}
	secretList, _ := s.clientset.CoreV1().Secrets("").List(r.Context(), metav1.ListOptions{})
	for _, secret := range secretList.Items {
		result.Summary.TotalSecrets++
		result.Summary.TotalKeys += len(secret.StringData)
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type RBAPIGroupsResult2491 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalRB    int            `json:"totalRoleBindings"`
		ByAPIGroup map[string]int `json:"byAPIGroup"`
	} `json:"summary"`
}

func (s *Server) handleRBAPIGroups2491(w http.ResponseWriter, r *http.Request) {
	result := RBAPIGroupsResult2491{ScannedAt: time.Now()}
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
