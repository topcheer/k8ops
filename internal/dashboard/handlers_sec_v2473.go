package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v24.73 Security: Pod SupplementalGroups, Secret BasicAuth Count, ClusterRoleBinding RoleRef Name
type SupplementalGroupsResult2473 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods int `json:"totalPods"`
		WithSupp  int `json:"withSupplementalGroups"`
	} `json:"summary"`
}

func (s *Server) handleSupplementalGroups2473(w http.ResponseWriter, r *http.Request) {
	result := SupplementalGroupsResult2473{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		if pod.Spec.SecurityContext != nil && len(pod.Spec.SecurityContext.SupplementalGroups) > 0 {
			result.Summary.WithSupp++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type SecretBasicAuthResult2473 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalSecrets int `json:"totalSecrets"`
		BasicAuth    int `json:"basicAuthCount"`
	} `json:"summary"`
}

func (s *Server) handleSecretBasicAuth2473(w http.ResponseWriter, r *http.Request) {
	result := SecretBasicAuthResult2473{ScannedAt: time.Now()}
	secretList, _ := s.clientset.CoreV1().Secrets("").List(r.Context(), metav1.ListOptions{})
	for _, secret := range secretList.Items {
		result.Summary.TotalSecrets++
		if secret.Type == corev1.SecretTypeBasicAuth {
			result.Summary.BasicAuth++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type CRBRoleRefNameResult2473 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalCRBs int            `json:"totalClusterRoleBindings"`
		ByRoleRef map[string]int `json:"byRoleRefName"`
	} `json:"summary"`
}

func (s *Server) handleCRBRoleRefName2473(w http.ResponseWriter, r *http.Request) {
	result := CRBRoleRefNameResult2473{ScannedAt: time.Now()}
	result.Summary.ByRoleRef = make(map[string]int)
	crbList, _ := s.clientset.RbacV1().ClusterRoleBindings().List(r.Context(), metav1.ListOptions{})
	for _, crb := range crbList.Items {
		result.Summary.TotalCRBs++
		result.Summary.ByRoleRef[crb.RoleRef.Name]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
