package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v24.61 Security: Pod FSGroup Distribution, Secret Immutable Count, RoleBinding ClusterWide
type FSGroupResult2461 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods   int `json:"totalPods"`
		WithFSGroup int `json:"withFSGroup"`
	} `json:"summary"`
}

func (s *Server) handleFSGroup2461(w http.ResponseWriter, r *http.Request) {
	result := FSGroupResult2461{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		if pod.Spec.SecurityContext != nil && pod.Spec.SecurityContext.FSGroup != nil {
			result.Summary.WithFSGroup++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type SecretImmutableResult2461 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalSecrets int `json:"totalSecrets"`
		Immutable    int `json:"immutableCount"`
	} `json:"summary"`
}

func (s *Server) handleSecretImmutable2461(w http.ResponseWriter, r *http.Request) {
	result := SecretImmutableResult2461{ScannedAt: time.Now()}
	secretList, _ := s.clientset.CoreV1().Secrets("").List(r.Context(), metav1.ListOptions{})
	for _, secret := range secretList.Items {
		result.Summary.TotalSecrets++
		if secret.Immutable != nil && *secret.Immutable {
			result.Summary.Immutable++
		}
	}
	score := 100
	if result.Summary.TotalSecrets > 0 {
		score = result.Summary.Immutable * 100 / result.Summary.TotalSecrets
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

type RBClusterWideResult2461 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalRB     int `json:"totalRoleBindings"`
		ClusterWide int `json:"clusterWide"`
	} `json:"summary"`
}

func (s *Server) handleRBClusterWide2461(w http.ResponseWriter, r *http.Request) {
	result := RBClusterWideResult2461{ScannedAt: time.Now()}
	rbList, _ := s.clientset.RbacV1().RoleBindings("").List(r.Context(), metav1.ListOptions{})
	for _, rb := range rbList.Items {
		result.Summary.TotalRB++
		for _, subj := range rb.Subjects {
			if subj.Namespace == "" {
				result.Summary.ClusterWide++
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
