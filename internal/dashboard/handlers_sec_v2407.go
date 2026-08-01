package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v24.07 Security: Pod Seccomp RuntimeDefault, Secret Annotation Helm, ClusterRoleBinding Subject SA
type SeccompRDResult2407 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods      int `json:"totalPods"`
		RuntimeDefault int `json:"seccompRuntimeDefault"`
	} `json:"summary"`
}

func (s *Server) handleSeccompRD2407(w http.ResponseWriter, r *http.Request) {
	result := SeccompRDResult2407{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		if pod.Spec.SecurityContext != nil && pod.Spec.SecurityContext.SeccompProfile != nil && pod.Spec.SecurityContext.SeccompProfile.Type == corev1.SeccompProfileTypeRuntimeDefault {
			result.Summary.RuntimeDefault++
		}
	}
	score := 100
	if result.Summary.TotalPods > 0 {
		score = result.Summary.RuntimeDefault * 100 / result.Summary.TotalPods
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

type SecretHelmAnnotResult2407 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalSecrets  int `json:"totalSecrets"`
		HelmAnnotated int `json:"helmAnnotated"`
	} `json:"summary"`
}

func (s *Server) handleSecretHelmAnnot2407(w http.ResponseWriter, r *http.Request) {
	result := SecretHelmAnnotResult2407{ScannedAt: time.Now()}
	secretList, _ := s.clientset.CoreV1().Secrets("").List(r.Context(), metav1.ListOptions{})
	for _, secret := range secretList.Items {
		result.Summary.TotalSecrets++
		if _, ok := secret.Annotations["meta.helm.sh/release-name"]; ok {
			result.Summary.HelmAnnotated++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type CRBSubjectSAResult2407 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalCRB   int `json:"totalClusterRoleBindings"`
		SASubjects int `json:"saSubjects"`
	} `json:"summary"`
}

func (s *Server) handleCRBSubjectSA2407(w http.ResponseWriter, r *http.Request) {
	result := CRBSubjectSAResult2407{ScannedAt: time.Now()}
	crbList, _ := s.clientset.RbacV1().ClusterRoleBindings().List(r.Context(), metav1.ListOptions{})
	for _, crb := range crbList.Items {
		result.Summary.TotalCRB++
		for _, sub := range crb.Subjects {
			if sub.Kind == "ServiceAccount" {
				result.Summary.SASubjects++
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
