package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v25.57 Security: Pod Privileged Container, Secret Type Detail Summary, ClusterRole APIGroups Summary
type PrivilegedContainerResult2557 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalContainers int `json:"totalContainers"`
		Privileged      int `json:"privilegedContainers"`
	}
}

func (s *Server) handlePrivilegedContainer2557(w http.ResponseWriter, r *http.Request) {
	result := PrivilegedContainerResult2557{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++
			if c.SecurityContext != nil && c.SecurityContext.Privileged != nil && *c.SecurityContext.Privileged {
				result.Summary.Privileged++
			}
		}
	}
	score := 100
	if result.Summary.Privileged > 0 {
		score = 100 - result.Summary.Privileged*20
		if score < 0 {
			score = 0
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

type SecretTypeDetailResult2557 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalSecrets int            `json:"totalSecrets"`
		ByType       map[string]int `json:"byType"`
	}
}

func (s *Server) handleSecretTypeDetail2557(w http.ResponseWriter, r *http.Request) {
	result := SecretTypeDetailResult2557{ScannedAt: time.Now()}
	result.Summary.ByType = make(map[string]int)
	secretList, _ := s.clientset.CoreV1().Secrets("").List(r.Context(), metav1.ListOptions{})
	for _, secret := range secretList.Items {
		result.Summary.TotalSecrets++
		result.Summary.ByType[string(secret.Type)]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type CRAPIGroupsResult2557 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalCRs   int            `json:"totalClusterRoles"`
		ByAPIGroup map[string]int `json:"byAPIGroup"`
	}
}

func (s *Server) handleCRAPIGroups2557(w http.ResponseWriter, r *http.Request) {
	result := CRAPIGroupsResult2557{ScannedAt: time.Now()}
	result.Summary.ByAPIGroup = make(map[string]int)
	crList, _ := s.clientset.RbacV1().ClusterRoles().List(r.Context(), metav1.ListOptions{})
	for _, cr := range crList.Items {
		result.Summary.TotalCRs++
		for _, rule := range cr.Rules {
			for _, g := range rule.APIGroups {
				result.Summary.ByAPIGroup[g]++
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
