package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v24.13 Security: Pod DropAll Capabilities, Secret Creation Stale, Role ResourceNames Count
type DropAllCapsResult2413 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalContainers int `json:"totalContainers"`
		WithDropAll     int `json:"withDropAll"`
	} `json:"summary"`
}

func (s *Server) handleDropAllCaps2413(w http.ResponseWriter, r *http.Request) {
	result := DropAllCapsResult2413{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++
			if c.SecurityContext != nil && c.SecurityContext.Capabilities != nil {
				for _, d := range c.SecurityContext.Capabilities.Drop {
					if d == "ALL" {
						result.Summary.WithDropAll++
						break
					}
				}
			}
		}
	}
	score := 100
	if result.Summary.TotalContainers > 0 {
		score = result.Summary.WithDropAll * 100 / result.Summary.TotalContainers
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

type SecretStaleResult2413 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalSecrets int `json:"totalSecrets"`
		Stale        int `json:"stale365d"`
	} `json:"summary"`
}

func (s *Server) handleSecretStale2413(w http.ResponseWriter, r *http.Request) {
	result := SecretStaleResult2413{ScannedAt: time.Now()}
	secretList, _ := s.clientset.CoreV1().Secrets("").List(r.Context(), metav1.ListOptions{})
	now := time.Now()
	for _, secret := range secretList.Items {
		result.Summary.TotalSecrets++
		if secret.CreationTimestamp.Time.Before(now.AddDate(-1, 0, 0)) {
			result.Summary.Stale++
		}
	}
	score := 100
	if result.Summary.TotalSecrets > 0 {
		score = 100 - (result.Summary.Stale*30)/result.Summary.TotalSecrets
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

type RoleResNamesResult2413 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalRoles    int `json:"totalRoles"`
		TotalResNames int `json:"totalResourceNames"`
	} `json:"summary"`
}

func (s *Server) handleRoleResNames2413(w http.ResponseWriter, r *http.Request) {
	result := RoleResNamesResult2413{ScannedAt: time.Now()}
	crList, _ := s.clientset.RbacV1().ClusterRoles().List(r.Context(), metav1.ListOptions{})
	for _, cr := range crList.Items {
		result.Summary.TotalRoles++
		for _, rule := range cr.Rules {
			result.Summary.TotalResNames += len(rule.ResourceNames)
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
