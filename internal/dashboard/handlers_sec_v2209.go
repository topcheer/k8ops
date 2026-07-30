package dashboard

import (
	"net/http"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v22.09 — Security Dimension (Round 54)
// 1. Pod SupplementalGroups Audit
// 2. Namespace Default SA Usage Risk
// 3. ClusterRoleBinding Subject Namespace Validator
// ============================================================

type SuppGroupsResult2209 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods   int `json:"totalPods"`
		WithSuppGrp int `json:"withSupplementalGroups"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleSuppGroups2209(w http.ResponseWriter, r *http.Request) {
	result := SuppGroupsResult2209{ScannedAt: time.Now()}
	score := 100
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		if pod.Spec.SecurityContext != nil && len(pod.Spec.SecurityContext.SupplementalGroups) > 0 {
			result.Summary.WithSuppGrp++
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 2. Default SA Usage Risk
type DefaultSARiskResult2209 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods      int `json:"totalPods"`
		UsingDefaultSA int `json:"usingDefaultSA"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleDefaultSARisk2209(w http.ResponseWriter, r *http.Request) {
	result := DefaultSARiskResult2209{ScannedAt: time.Now()}
	score := 100
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		sa := pod.Spec.ServiceAccountName
		if sa == "" || sa == "default" {
			result.Summary.UsingDefaultSA++
			score -= 1
		}
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 3. CRB Subject NS Validator
type CRBSubjNSResult2209 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalCRB int `json:"totalClusterRoleBindings"`
		WithNS   int `json:"withNamespaceScopedSubject"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleCRBSubjNS2209(w http.ResponseWriter, r *http.Request) {
	result := CRBSubjNSResult2209{ScannedAt: time.Now()}
	score := 100
	crbList, _ := s.clientset.RbacV1().ClusterRoleBindings().List(r.Context(), metav1.ListOptions{})
	for _, crb := range crbList.Items {
		result.Summary.TotalCRB++
		for _, subj := range crb.Subjects {
			if subj.Namespace != "" {
				result.Summary.WithNS++
				break
			}
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}
