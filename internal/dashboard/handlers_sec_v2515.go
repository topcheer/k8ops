package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v25.15 Security: Pod FSGroupChangePolicy, Secret Annotation Count, RoleBinding RoleRef Name
type FSGroupChangeResult2515 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods int            `json:"totalPods"`
		ByPolicy  map[string]int `json:"byFSGroupChangePolicy"`
	} `json:"summary"`
}

func (s *Server) handleFSGroupChange2515(w http.ResponseWriter, r *http.Request) {
	result := FSGroupChangeResult2515{ScannedAt: time.Now()}
	result.Summary.ByPolicy = make(map[string]int)
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		policy := "<default>"
		if pod.Spec.SecurityContext != nil && pod.Spec.SecurityContext.FSGroupChangePolicy != nil {
			policy = string(*pod.Spec.SecurityContext.FSGroupChangePolicy)
		}
		result.Summary.ByPolicy[policy]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type SecretAnnotationResult2515 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalSecrets int `json:"totalSecrets"`
		TotalAnnots  int `json:"totalAnnotations"`
	} `json:"summary"`
}

func (s *Server) handleSecretAnnotation2515(w http.ResponseWriter, r *http.Request) {
	result := SecretAnnotationResult2515{ScannedAt: time.Now()}
	secretList, _ := s.clientset.CoreV1().Secrets("").List(r.Context(), metav1.ListOptions{})
	for _, secret := range secretList.Items {
		result.Summary.TotalSecrets++
		result.Summary.TotalAnnots += len(secret.Annotations)
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type RBRoleRefNameResult2515 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalRB   int            `json:"totalRoleBindings"`
		ByRefName map[string]int `json:"byRoleRefName"`
	} `json:"summary"`
}

func (s *Server) handleRBRoleRefName2515(w http.ResponseWriter, r *http.Request) {
	result := RBRoleRefNameResult2515{ScannedAt: time.Now()}
	result.Summary.ByRefName = make(map[string]int)
	rbList, _ := s.clientset.RbacV1().RoleBindings("").List(r.Context(), metav1.ListOptions{})
	for _, rb := range rbList.Items {
		result.Summary.TotalRB++
		result.Summary.ByRefName[rb.RoleRef.Name]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
