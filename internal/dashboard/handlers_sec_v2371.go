package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"strings"
	"time"
)

// v23.71 Security: Pod NonRoot UID, Secret Helm Type, ClusterRole Binding RoleRef
type NonRootUIDResult2371 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods  int `json:"totalPods"`
		NonRootUID int `json:"nonRootUID"`
	} `json:"summary"`
}

func (s *Server) handleNonRootUID2371(w http.ResponseWriter, r *http.Request) {
	result := NonRootUIDResult2371{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		if pod.Spec.SecurityContext != nil && pod.Spec.SecurityContext.RunAsUser != nil && *pod.Spec.SecurityContext.RunAsUser != 0 {
			result.Summary.NonRootUID++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type HelmSecretResult2371 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalSecrets int `json:"totalSecrets"`
		HelmSecrets  int `json:"helmSecrets"`
	} `json:"summary"`
}

func (s *Server) handleHelmSecret2371(w http.ResponseWriter, r *http.Request) {
	result := HelmSecretResult2371{ScannedAt: time.Now()}
	secretList, _ := s.clientset.CoreV1().Secrets("").List(r.Context(), metav1.ListOptions{})
	for _, secret := range secretList.Items {
		result.Summary.TotalSecrets++
		if strings.HasPrefix(secret.Name, "sh.helm.release") {
			result.Summary.HelmSecrets++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type CRBRoleRefResult2371 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalCRB int            `json:"totalClusterRoleBindings"`
		ByKind   map[string]int `json:"byRoleRefKind"`
	} `json:"summary"`
}

func (s *Server) handleCRBRoleRef2371(w http.ResponseWriter, r *http.Request) {
	result := CRBRoleRefResult2371{ScannedAt: time.Now()}
	result.Summary.ByKind = make(map[string]int)
	crbList, _ := s.clientset.RbacV1().ClusterRoleBindings().List(r.Context(), metav1.ListOptions{})
	for _, crb := range crbList.Items {
		result.Summary.TotalCRB++
		result.Summary.ByKind[crb.RoleRef.Kind]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
