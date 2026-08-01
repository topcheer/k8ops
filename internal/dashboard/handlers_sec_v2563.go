package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v25.63 Security: Pod HostPID Detail, Secret Immutable Check, RoleBinding Subjects Count
type HostPIDDetailResult2563 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods int `json:"totalPods"`
		HostPID   int `json:"hostPIDPods"`
	}
}

func (s *Server) handleHostPIDDetail2563(w http.ResponseWriter, r *http.Request) {
	result := HostPIDDetailResult2563{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		if pod.Spec.HostPID {
			result.Summary.HostPID++
		}
	}
	score := 100
	if result.Summary.HostPID > 0 {
		score = 100 - result.Summary.HostPID*10
		if score < 0 {
			score = 0
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

type SecretImmutableResult2563 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalSecrets int `json:"totalSecrets"`
		Immutable    int `json:"immutableCount"`
	}
}

func (s *Server) handleSecretImmutable2563(w http.ResponseWriter, r *http.Request) {
	result := SecretImmutableResult2563{ScannedAt: time.Now()}
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

type RBSubjectsCountResult2563 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalRB    int `json:"totalRoleBindings"`
		TotalSubjs int `json:"totalSubjects"`
	}
}

func (s *Server) handleRBSubjectsCount2563(w http.ResponseWriter, r *http.Request) {
	result := RBSubjectsCountResult2563{ScannedAt: time.Now()}
	rbList, _ := s.clientset.RbacV1().RoleBindings("").List(r.Context(), metav1.ListOptions{})
	for _, rb := range rbList.Items {
		result.Summary.TotalRB++
		result.Summary.TotalSubjs += len(rb.Subjects)
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
