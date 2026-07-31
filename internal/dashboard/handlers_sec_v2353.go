package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v23.53 Security: Pod Seccomp Localhost, Secret BasicAuth, Role Resource Wildcard
type SeccompLocalResult2353 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods int `json:"totalPods"`
		Localhost int `json:"localhostProfile"`
	} `json:"summary"`
}

func (s *Server) handleSeccompLocal2353(w http.ResponseWriter, r *http.Request) {
	result := SeccompLocalResult2353{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		if pod.Spec.SecurityContext != nil && pod.Spec.SecurityContext.SeccompProfile != nil && pod.Spec.SecurityContext.SeccompProfile.Type == corev1.SeccompProfileTypeLocalhost {
			result.Summary.Localhost++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type BasicAuthResult2353 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalSecrets int `json:"totalSecrets"`
		BasicAuth    int `json:"basicAuthSecrets"`
	} `json:"summary"`
}

func (s *Server) handleBasicAuth2353(w http.ResponseWriter, r *http.Request) {
	result := BasicAuthResult2353{ScannedAt: time.Now()}
	secretList, _ := s.clientset.CoreV1().Secrets("").List(r.Context(), metav1.ListOptions{})
	for _, secret := range secretList.Items {
		result.Summary.TotalSecrets++
		if secret.Type == corev1.SecretTypeBasicAuth {
			result.Summary.BasicAuth++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type RoleResWildcardResult2353 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalRoles  int `json:"totalRoles"`
		WildcardRes int `json:"withWildcardResources"`
	} `json:"summary"`
}

func (s *Server) handleRoleResWildcard2353(w http.ResponseWriter, r *http.Request) {
	result := RoleResWildcardResult2353{ScannedAt: time.Now()}
	roleList, _ := s.clientset.RbacV1().Roles("").List(r.Context(), metav1.ListOptions{})
	for _, role := range roleList.Items {
		result.Summary.TotalRoles++
		for _, rule := range role.Rules {
			for _, res := range rule.Resources {
				if res == "*" {
					result.Summary.WildcardRes++
					break
				}
			}
		}
	}
	score := 100
	if result.Summary.TotalRoles > 0 && result.Summary.WildcardRes > 0 {
		score = 100 - (result.Summary.WildcardRes*30)/result.Summary.TotalRoles
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}
