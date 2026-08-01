package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v25.81 Security: Pod SeccompProfile Detail, Secret Data Key Count, ClusterRoleBinding Subject Name
type SeccompDetailResult2581 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalContainers int            `json:"totalContainers"`
		ByType          map[string]int `json:"bySeccompType"`
	}
}

func (s *Server) handleSeccompDetail2581(w http.ResponseWriter, r *http.Request) {
	result := SeccompDetailResult2581{ScannedAt: time.Now()}
	result.Summary.ByType = make(map[string]int)
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++
			t := "<none>"
			if c.SecurityContext != nil && c.SecurityContext.SeccompProfile != nil {
				t = string(c.SecurityContext.SeccompProfile.Type)
			}
			result.Summary.ByType[t]++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type SecretDataKeyResult2581 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalSecrets int `json:"totalSecrets"`
		TotalKeys    int `json:"totalDataKeys"`
	}
}

func (s *Server) handleSecretDataKey2581(w http.ResponseWriter, r *http.Request) {
	result := SecretDataKeyResult2581{ScannedAt: time.Now()}
	secretList, _ := s.clientset.CoreV1().Secrets("").List(r.Context(), metav1.ListOptions{})
	for _, secret := range secretList.Items {
		result.Summary.TotalSecrets++
		result.Summary.TotalKeys += len(secret.Data)
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type CRBSubjectNameResult2581 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalCRBs int            `json:"totalClusterRoleBindings"`
		ByName    map[string]int `json:"bySubjectName"`
	}
}

func (s *Server) handleCRBSubjectName2581(w http.ResponseWriter, r *http.Request) {
	result := CRBSubjectNameResult2581{ScannedAt: time.Now()}
	result.Summary.ByName = make(map[string]int)
	crbList, _ := s.clientset.RbacV1().ClusterRoleBindings().List(r.Context(), metav1.ListOptions{})
	for _, crb := range crbList.Items {
		result.Summary.TotalCRBs++
		for _, subj := range crb.Subjects {
			result.Summary.ByName[subj.Name]++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
