package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v24.43 Security: Pod RunAsNonRoot Ratio, ClusterRole AggregatedRules, SA AutoMount Disabled
type RunAsNonRootResult2443 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalContainers int `json:"totalContainers"`
		NonRoot         int `json:"runAsNonRoot"`
	} `json:"summary"`
}

func (s *Server) handleRunAsNonRoot2443(w http.ResponseWriter, r *http.Request) {
	result := RunAsNonRootResult2443{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++
			if c.SecurityContext != nil && c.SecurityContext.RunAsNonRoot != nil && *c.SecurityContext.RunAsNonRoot {
				result.Summary.NonRoot++
			}
		}
	}
	score := 100
	if result.Summary.TotalContainers > 0 {
		ratio := result.Summary.NonRoot * 100 / result.Summary.TotalContainers
		score = ratio
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

type CRAggregatedRulesResult2443 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalCRs   int `json:"totalClusterRoles"`
		Aggregated int `json:"withAggregationRule"`
	} `json:"summary"`
}

func (s *Server) handleCRAggregatedRules2443(w http.ResponseWriter, r *http.Request) {
	result := CRAggregatedRulesResult2443{ScannedAt: time.Now()}
	crList, _ := s.clientset.RbacV1().ClusterRoles().List(r.Context(), metav1.ListOptions{})
	for _, cr := range crList.Items {
		result.Summary.TotalCRs++
		if cr.AggregationRule != nil && len(cr.AggregationRule.ClusterRoleSelectors) > 0 {
			result.Summary.Aggregated++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type SAAutoMountDisabledResult2443 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalSAs     int `json:"totalServiceAccounts"`
		AutoDisabled int `json:"autoMountDisabled"`
	} `json:"summary"`
}

func (s *Server) handleSAAutoMountDisabled2443(w http.ResponseWriter, r *http.Request) {
	result := SAAutoMountDisabledResult2443{ScannedAt: time.Now()}
	saList, _ := s.clientset.CoreV1().ServiceAccounts("").List(r.Context(), metav1.ListOptions{})
	for _, sa := range saList.Items {
		result.Summary.TotalSAs++
		if sa.AutomountServiceAccountToken != nil && !*sa.AutomountServiceAccountToken {
			result.Summary.AutoDisabled++
		}
	}
	score := 100
	if result.Summary.TotalSAs > 0 {
		score = result.Summary.AutoDisabled * 100 / result.Summary.TotalSAs
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}
