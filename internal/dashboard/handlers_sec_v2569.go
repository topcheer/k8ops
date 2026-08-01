package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v25.69 Security: Pod PrivilegeEscalation, Secret Label Count, RoleBinding Subject Kind Detail
type PrivEscResult2569 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalContainers int `json:"totalContainers"`
		WithPrivEsc     int `json:"allowPrivilegeEscalation"`
	}
}

func (s *Server) handlePrivEsc2569(w http.ResponseWriter, r *http.Request) {
	result := PrivEscResult2569{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++
			if c.SecurityContext != nil && c.SecurityContext.AllowPrivilegeEscalation != nil && *c.SecurityContext.AllowPrivilegeEscalation {
				result.Summary.WithPrivEsc++
			}
		}
	}
	score := 100
	if result.Summary.WithPrivEsc > 0 {
		score = 100 - result.Summary.WithPrivEsc*20
		if score < 0 {
			score = 0
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

type SecretLabelCountResult2569 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalSecrets int `json:"totalSecrets"`
		TotalLabels  int `json:"totalLabels"`
	}
}

func (s *Server) handleSecretLabelCount2569(w http.ResponseWriter, r *http.Request) {
	result := SecretLabelCountResult2569{ScannedAt: time.Now()}
	secretList, _ := s.clientset.CoreV1().Secrets("").List(r.Context(), metav1.ListOptions{})
	for _, secret := range secretList.Items {
		result.Summary.TotalSecrets++
		result.Summary.TotalLabels += len(secret.Labels)
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type RBSubjectKindResult2569 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalRB int            `json:"totalRoleBindings"`
		ByKind  map[string]int `json:"bySubjectKind"`
	}
}

func (s *Server) handleRBSubjectKind2569(w http.ResponseWriter, r *http.Request) {
	result := RBSubjectKindResult2569{ScannedAt: time.Now()}
	result.Summary.ByKind = make(map[string]int)
	rbList, _ := s.clientset.RbacV1().RoleBindings("").List(r.Context(), metav1.ListOptions{})
	for _, rb := range rbList.Items {
		result.Summary.TotalRB++
		for _, subj := range rb.Subjects {
			result.Summary.ByKind[subj.Kind]++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
