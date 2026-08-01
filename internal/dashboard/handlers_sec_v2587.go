package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v25.87 Security: Pod SupplementalGroups, Secret Type Label Count, RoleBinding Subject API Group
type SupplementalGroupsResult2587 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods int `json:"totalPods"`
		WithSupp  int `json:"withSupplementalGroups"`
	}
}

func (s *Server) handleSupplementalGroups2587(w http.ResponseWriter, r *http.Request) {
	result := SupplementalGroupsResult2587{ScannedAt: time.Now()}
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

type SecretTypeLabelResult2587 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalSecrets int `json:"totalSecrets"`
		WithLabels   int `json:"withTypeLabel"`
	}
}

func (s *Server) handleSecretTypeLabel2587(w http.ResponseWriter, r *http.Request) {
	result := SecretTypeLabelResult2587{ScannedAt: time.Now()}
	secretList, _ := s.clientset.CoreV1().Secrets("").List(r.Context(), metav1.ListOptions{})
	for _, secret := range secretList.Items {
		result.Summary.TotalSecrets++
		if _, ok := secret.Labels["type"]; ok {
			result.Summary.WithLabels++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type RBSubjectAPIGroupResult2587 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalRB    int            `json:"totalRoleBindings"`
		ByAPIGroup map[string]int `json:"bySubjectAPIGroup"`
	}
}

func (s *Server) handleRBSubjectAPIGroup2587(w http.ResponseWriter, r *http.Request) {
	result := RBSubjectAPIGroupResult2587{ScannedAt: time.Now()}
	result.Summary.ByAPIGroup = make(map[string]int)
	rbList, _ := s.clientset.RbacV1().RoleBindings("").List(r.Context(), metav1.ListOptions{})
	for _, rb := range rbList.Items {
		result.Summary.TotalRB++
		for _, subj := range rb.Subjects {
			result.Summary.ByAPIGroup[subj.APIGroup]++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
