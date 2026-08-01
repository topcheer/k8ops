package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v25.45 Security: Pod AppArmorProfile, Secret MaxAge, ClusterRoleBinding ResourceNames
type AppArmorResult2545 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalContainers int            `json:"totalContainers"`
		ByProfile       map[string]int `json:"byAppArmorProfile"`
	} `json:"summary"`
}

func (s *Server) handleAppArmor2545(w http.ResponseWriter, r *http.Request) {
	result := AppArmorResult2545{ScannedAt: time.Now()}
	result.Summary.ByProfile = make(map[string]int)
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++
			p := "<none>"
			if c.SecurityContext != nil && c.SecurityContext.AppArmorProfile != nil {
				p = string(c.SecurityContext.AppArmorProfile.Type)
			}
			result.Summary.ByProfile[p]++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type SecretMaxAgeResult2545 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalSecrets int   `json:"totalSecrets"`
		MaxAgeDays   int64 `json:"maxAgeDays"`
	} `json:"summary"`
}

func (s *Server) handleSecretMaxAge2545(w http.ResponseWriter, r *http.Request) {
	result := SecretMaxAgeResult2545{ScannedAt: time.Now()}
	secretList, _ := s.clientset.CoreV1().Secrets("").List(r.Context(), metav1.ListOptions{})
	var maxAge time.Duration
	now := time.Now()
	for _, secret := range secretList.Items {
		result.Summary.TotalSecrets++
		age := now.Sub(secret.CreationTimestamp.Time)
		if age > maxAge {
			maxAge = age
		}
	}
	result.Summary.MaxAgeDays = int64(maxAge.Hours() / 24)
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type CRBResourceNamesResult2545 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalCRBs  int            `json:"totalClusterRoles"`
		ByResource map[string]int `json:"byResourceName"`
	} `json:"summary"`
}

func (s *Server) handleCRBResourceNames2545(w http.ResponseWriter, r *http.Request) {
	result := CRBResourceNamesResult2545{ScannedAt: time.Now()}
	result.Summary.ByResource = make(map[string]int)
	crList, _ := s.clientset.RbacV1().ClusterRoles().List(r.Context(), metav1.ListOptions{})
	for _, cr := range crList.Items {
		result.Summary.TotalCRBs++
		for _, rule := range cr.Rules {
			for _, res := range rule.ResourceNames {
				result.Summary.ByResource[res]++
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
