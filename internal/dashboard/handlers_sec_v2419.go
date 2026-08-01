package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v24.19 Security: Pod Seccomp Unconfined, Secret Namespace Count, RoleBinding Subject User
type SeccompUnconfResult2419 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods  int `json:"totalPods"`
		Unconfined int `json:"seccompUnconfined"`
	} `json:"summary"`
}

func (s *Server) handleSeccompUnconf2419(w http.ResponseWriter, r *http.Request) {
	result := SeccompUnconfResult2419{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		if pod.Spec.SecurityContext != nil && pod.Spec.SecurityContext.SeccompProfile != nil && pod.Spec.SecurityContext.SeccompProfile.Type == corev1.SeccompProfileTypeUnconfined {
			result.Summary.Unconfined++
		}
	}
	score := 100
	if result.Summary.TotalPods > 0 {
		score = 100 - (result.Summary.Unconfined*50)/result.Summary.TotalPods
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

type SecretNSResult2419 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalSecrets int            `json:"totalSecrets"`
		ByNS         map[string]int `json:"byNamespace"`
	} `json:"summary"`
}

func (s *Server) handleSecretNS2419(w http.ResponseWriter, r *http.Request) {
	result := SecretNSResult2419{ScannedAt: time.Now()}
	result.Summary.ByNS = make(map[string]int)
	secretList, _ := s.clientset.CoreV1().Secrets("").List(r.Context(), metav1.ListOptions{})
	for _, secret := range secretList.Items {
		result.Summary.TotalSecrets++
		result.Summary.ByNS[secret.Namespace]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type RBSubjectUserResult2419 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalRB  int `json:"totalRoleBindings"`
		UserSubs int `json:"userSubjects"`
	} `json:"summary"`
}

func (s *Server) handleRBSubjectUser2419(w http.ResponseWriter, r *http.Request) {
	result := RBSubjectUserResult2419{ScannedAt: time.Now()}
	rbList, _ := s.clientset.RbacV1().RoleBindings("").List(r.Context(), metav1.ListOptions{})
	for _, rb := range rbList.Items {
		result.Summary.TotalRB++
		for _, sub := range rb.Subjects {
			if sub.Kind == "User" {
				result.Summary.UserSubs++
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
