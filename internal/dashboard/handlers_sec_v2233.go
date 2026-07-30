package dashboard

import (
	"net/http"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v22.33 — Security Dimension (Round 58)
// 1. Pod Seccomp Default Profile Coverage
// 2. ServiceAccount Automount Token Disabled Audit
// 3. ClusterRole Empty Resources Risk
// ============================================================

type SeccompDefaultCovResult2233 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods          int `json:"totalPods"`
		WithRuntimeDefault int `json:"withRuntimeDefault"`
		WithoutSeccomp     int `json:"withoutSeccomp"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleSeccompDefaultCov2233(w http.ResponseWriter, r *http.Request) {
	result := SeccompDefaultCovResult2233{ScannedAt: time.Now()}
	score := 100
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		if pod.Spec.SecurityContext != nil && pod.Spec.SecurityContext.SeccompProfile != nil {
			if pod.Spec.SecurityContext.SeccompProfile.Type == corev1.SeccompProfileTypeRuntimeDefault {
				result.Summary.WithRuntimeDefault++
			}
		} else {
			result.Summary.WithoutSeccomp++
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 2. SA Automount Token Disabled
type SAAutomountDisabledResult2233 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalSAs int `json:"totalServiceAccounts"`
		Disabled int `json:"automountDisabled"`
		Enabled  int `json:"automountEnabled"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleSAAutomountDisabled2233(w http.ResponseWriter, r *http.Request) {
	result := SAAutomountDisabledResult2233{ScannedAt: time.Now()}
	score := 100
	saList, _ := s.clientset.CoreV1().ServiceAccounts("").List(r.Context(), metav1.ListOptions{})
	for _, sa := range saList.Items {
		result.Summary.TotalSAs++
		if sa.AutomountServiceAccountToken != nil && !*sa.AutomountServiceAccountToken {
			result.Summary.Disabled++
		} else {
			result.Summary.Enabled++
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 3. ClusterRole Empty Resources Risk
type CREmptyResResult2233 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalCR      int `json:"totalClusterRoles"`
		WithEmptyRes int `json:"withEmptyResources"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleCREmptyRes2233(w http.ResponseWriter, r *http.Request) {
	result := CREmptyResResult2233{ScannedAt: time.Now()}
	score := 100
	crList, _ := s.clientset.RbacV1().ClusterRoles().List(r.Context(), metav1.ListOptions{})
	for _, cr := range crList.Items {
		result.Summary.TotalCR++
		for _, rule := range cr.Rules {
			if len(rule.Resources) == 0 && len(rule.Verbs) > 0 {
				result.Summary.WithEmptyRes++
				break
			}
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}
