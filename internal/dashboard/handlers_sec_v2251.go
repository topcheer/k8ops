package dashboard

import (
	"net/http"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v22.51 — Security Dimension (Round 61)
// 1. Pod ReadOnlyRootFilesystem Audit
// 2. Secret Service Account Token Age
// 3. ClusterRole Wildcard Verb Audit
// ============================================================

type ReadOnlyFSResult2251 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalContainers int `json:"totalContainers"`
		ReadOnlyRoot    int `json:"withReadOnlyRootFilesystem"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleReadOnlyFS2251(w http.ResponseWriter, r *http.Request) {
	result := ReadOnlyFSResult2251{ScannedAt: time.Now()}
	score := 100
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++
			if c.SecurityContext != nil && c.SecurityContext.ReadOnlyRootFilesystem != nil && *c.SecurityContext.ReadOnlyRootFilesystem {
				result.Summary.ReadOnlyRoot++
			}
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 2. SA Token Age
type SATokenAgeResult2251 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalSAs  int `json:"totalServiceAccounts"`
		OldTokens int `json:"oldTokens90d"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleSATokenAge2251(w http.ResponseWriter, r *http.Request) {
	result := SATokenAgeResult2251{ScannedAt: time.Now()}
	score := 100
	saList, _ := s.clientset.CoreV1().ServiceAccounts("").List(r.Context(), metav1.ListOptions{})
	now := time.Now()
	for _, sa := range saList.Items {
		result.Summary.TotalSAs++
		if now.Sub(sa.CreationTimestamp.Time).Hours() > 24*90 {
			result.Summary.OldTokens++
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 3. ClusterRole Wildcard Verb
type CRWildcardVerbResult2251 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalCR      int `json:"totalClusterRoles"`
		WithWildcard int `json:"withWildcardVerb"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleCRWildcardVerb2251(w http.ResponseWriter, r *http.Request) {
	result := CRWildcardVerbResult2251{ScannedAt: time.Now()}
	score := 100
	crList, _ := s.clientset.RbacV1().ClusterRoles().List(r.Context(), metav1.ListOptions{})
	for _, cr := range crList.Items {
		result.Summary.TotalCR++
		for _, rule := range cr.Rules {
			for _, verb := range rule.Verbs {
				if verb == "*" {
					result.Summary.WithWildcard++
					goto nextCR
				}
			}
		}
	nextCR:
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}
