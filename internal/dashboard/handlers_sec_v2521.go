package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v25.21 Security: Pod Seccomp OnRootMissing, Secret Owner Kind Summary, ClusterRole NonResourceURLs
type SeccompOnRootResult2521 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalContainers int `json:"totalContainers"`
		OnRootMissing   int `json:"seccompOnRootMissing"`
	} `json:"summary"`
}

func (s *Server) handleSeccompOnRoot2521(w http.ResponseWriter, r *http.Request) {
	result := SeccompOnRootResult2521{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++
			if c.SecurityContext != nil && c.SecurityContext.SeccompProfile != nil && c.SecurityContext.SeccompProfile.Type == corev1.SeccompProfileTypeUnconfined {
				result.Summary.OnRootMissing++
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type SecretOwnerKindResult2521 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalSecrets int            `json:"totalSecrets"`
		ByOwnerKind  map[string]int `json:"byOwnerKind"`
	} `json:"summary"`
}

func (s *Server) handleSecretOwnerKind2521(w http.ResponseWriter, r *http.Request) {
	result := SecretOwnerKindResult2521{ScannedAt: time.Now()}
	result.Summary.ByOwnerKind = make(map[string]int)
	secretList, _ := s.clientset.CoreV1().Secrets("").List(r.Context(), metav1.ListOptions{})
	for _, secret := range secretList.Items {
		result.Summary.TotalSecrets++
		for _, ref := range secret.OwnerReferences {
			result.Summary.ByOwnerKind[ref.Kind]++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type CRNonResourceResult2521 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalCRs int            `json:"totalClusterRoles"`
		ByURL    map[string]int `json:"byNonResourceURL"`
	} `json:"summary"`
}

func (s *Server) handleCRNonResource2521(w http.ResponseWriter, r *http.Request) {
	result := CRNonResourceResult2521{ScannedAt: time.Now()}
	result.Summary.ByURL = make(map[string]int)
	crList, _ := s.clientset.RbacV1().ClusterRoles().List(r.Context(), metav1.ListOptions{})
	for _, cr := range crList.Items {
		result.Summary.TotalCRs++
		for _, rule := range cr.Rules {
			for _, url := range rule.NonResourceURLs {
				result.Summary.ByURL[url]++
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
