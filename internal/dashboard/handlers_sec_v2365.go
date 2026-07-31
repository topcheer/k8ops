package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v23.65 Security: Pod fsGroup Override, Secret SvcAcct Token, Role NonResourceURL
type FSGroupOverrideResult2365 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods   int `json:"totalPods"`
		WithFSGroup int `json:"withFSGroup"`
	} `json:"summary"`
}

func (s *Server) handleFSGroupOverride2365(w http.ResponseWriter, r *http.Request) {
	result := FSGroupOverrideResult2365{ScannedAt: time.Now()}
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

type SvcAcctTokenResult2365 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalSecrets   int `json:"totalSecrets"`
		SATokenSecrets int `json:"saTokenSecrets"`
	} `json:"summary"`
}

func (s *Server) handleSvcAcctToken2365(w http.ResponseWriter, r *http.Request) {
	result := SvcAcctTokenResult2365{ScannedAt: time.Now()}
	secretList, _ := s.clientset.CoreV1().Secrets("").List(r.Context(), metav1.ListOptions{})
	for _, secret := range secretList.Items {
		result.Summary.TotalSecrets++
		if secret.Type == corev1.SecretTypeServiceAccountToken {
			result.Summary.SATokenSecrets++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type RoleNonResURLResult2365 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalRoles    int `json:"totalRoles"`
		WithNonResURL int `json:"withNonResourceURLs"`
	} `json:"summary"`
}

func (s *Server) handleRoleNonResURL2365(w http.ResponseWriter, r *http.Request) {
	result := RoleNonResURLResult2365{ScannedAt: time.Now()}
	crList, _ := s.clientset.RbacV1().ClusterRoles().List(r.Context(), metav1.ListOptions{})
	for _, cr := range crList.Items {
		result.Summary.TotalRoles++
		for _, rule := range cr.Rules {
			if len(rule.NonResourceURLs) > 0 {
				result.Summary.WithNonResURL++
				break
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
