package dashboard

import (
	"net/http"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v21.73 — Security Dimension (Round 48)
// 1. Pod Capabilities Add Audit
// 2. Secret Age Tracker
// 3. RoleBinding ServiceAccount Validator
// ============================================================

type CapAddResult2173 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalContainers int `json:"totalContainers"`
		WithCapAdd      int `json:"withCapabilityAdd"`
		WithNetRaw      int `json:"withNETRAW"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleCapAdd2173(w http.ResponseWriter, r *http.Request) {
	result := CapAddResult2173{ScannedAt: time.Now()}
	score := 100
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++
			if c.SecurityContext != nil && len(c.SecurityContext.Capabilities.Add) > 0 {
				result.Summary.WithCapAdd++
				for _, cap := range c.SecurityContext.Capabilities.Add {
					if cap == "NET_RAW" {
						result.Summary.WithNetRaw++
						score -= 3
					}
				}
			}
		}
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 2. Secret Age Tracker
type SecretAgeResult2173 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalSecrets int `json:"totalSecrets"`
		OldSecrets   int `json:"oldSecrets90d"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleSecretAge2173(w http.ResponseWriter, r *http.Request) {
	result := SecretAgeResult2173{ScannedAt: time.Now()}
	score := 100
	secretList, _ := s.clientset.CoreV1().Secrets("").List(r.Context(), metav1.ListOptions{})
	now := time.Now()
	for _, secret := range secretList.Items {
		result.Summary.TotalSecrets++
		if now.Sub(secret.CreationTimestamp.Time).Hours() > 24*90 {
			result.Summary.OldSecrets++
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 3. RB ServiceAccount Validator
type RBSAResult2173 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalRB  int `json:"totalRoleBindings"`
		WithSA   int `json:"withServiceAccount"`
		WithUser int `json:"withUser"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleRBSA2173(w http.ResponseWriter, r *http.Request) {
	result := RBSAResult2173{ScannedAt: time.Now()}
	score := 100
	rbList, _ := s.clientset.RbacV1().RoleBindings("").List(r.Context(), metav1.ListOptions{})
	for _, rb := range rbList.Items {
		result.Summary.TotalRB++
		for _, subj := range rb.Subjects {
			if subj.Kind == "ServiceAccount" {
				result.Summary.WithSA++
				break
			}
			if subj.Kind == "User" {
				result.Summary.WithUser++
				break
			}
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}
