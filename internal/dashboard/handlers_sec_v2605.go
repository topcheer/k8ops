package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v26.05 Security: Pod Seccomp LocalhostProfile, Secret StringData Key Count, ClusterRoleBinding RoleRef Kind
type SeccompLocalhost2605Result struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalContainers int `json:"totalContainers"`
		WithLocalhost   int `json:"withLocalhostProfile"`
	} `json:"summary"`
}

func (s *Server) handleSeccompLocalhost2605(w http.ResponseWriter, r *http.Request) {
	result := SeccompLocalhost2605Result{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++
			if c.SecurityContext != nil && c.SecurityContext.SeccompProfile != nil && c.SecurityContext.SeccompProfile.LocalhostProfile != nil {
				result.Summary.WithLocalhost++
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type SecretStringDataResult2605 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalSecrets   int `json:"totalSecrets"`
		WithStringData int `json:"withStringData"`
	} `json:"summary"`
}

func (s *Server) handleSecretStringData2605(w http.ResponseWriter, r *http.Request) {
	result := SecretStringDataResult2605{ScannedAt: time.Now()}
	secretList, _ := s.clientset.CoreV1().Secrets("").List(r.Context(), metav1.ListOptions{})
	for _, secret := range secretList.Items {
		result.Summary.TotalSecrets++
		if len(secret.StringData) > 0 {
			result.Summary.WithStringData++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type CRBRoleRefKindResult2605 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalCRBs int            `json:"totalClusterRoleBindings"`
		ByKind    map[string]int `json:"byRoleRefKind"`
	} `json:"summary"`
}

func (s *Server) handleCRBRoleRefKind2605(w http.ResponseWriter, r *http.Request) {
	result := CRBRoleRefKindResult2605{ScannedAt: time.Now()}
	result.Summary.ByKind = make(map[string]int)
	crbList, _ := s.clientset.RbacV1().ClusterRoleBindings().List(r.Context(), metav1.ListOptions{})
	for _, crb := range crbList.Items {
		result.Summary.TotalCRBs++
		result.Summary.ByKind[crb.RoleRef.Kind]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
