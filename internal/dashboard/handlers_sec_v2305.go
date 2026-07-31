package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v23.05 Security: ServiceAccount Age Audit, Pod fsgroupChangePolicy, ClusterRole Aggregation Rule
type SAAgeResult2305 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalSAs    int            `json:"totalServiceAccounts"`
		ByAgeBucket map[string]int `json:"byAgeBucket"`
	} `json:"summary"`
}

func (s *Server) handleSAAge2305(w http.ResponseWriter, r *http.Request) {
	result := SAAgeResult2305{ScannedAt: time.Now()}
	result.Summary.ByAgeBucket = make(map[string]int)
	saList, _ := s.clientset.CoreV1().ServiceAccounts("").List(r.Context(), metav1.ListOptions{})
	now := time.Now()
	for _, sa := range saList.Items {
		result.Summary.TotalSAs++
		age := now.Sub(sa.CreationTimestamp.Time)
		var bucket string
		switch {
		case age < 24*time.Hour:
			bucket = "<1d"
		case age < 7*24*time.Hour:
			bucket = "1-7d"
		case age < 30*24*time.Hour:
			bucket = "7-30d"
		case age < 90*24*time.Hour:
			bucket = "30-90d"
		default:
			bucket = "90d+"
		}
		result.Summary.ByAgeBucket[bucket]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type FSGroupChangeResult2305 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods int            `json:"totalPods"`
		ByPolicy  map[string]int `json:"byFSGroupChangePolicy"`
	} `json:"summary"`
}

func (s *Server) handleFSGroupChange2305(w http.ResponseWriter, r *http.Request) {
	result := FSGroupChangeResult2305{ScannedAt: time.Now()}
	result.Summary.ByPolicy = make(map[string]int)
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		if pod.Spec.SecurityContext != nil && pod.Spec.SecurityContext.FSGroupChangePolicy != nil {
			result.Summary.ByPolicy[string(*pod.Spec.SecurityContext.FSGroupChangePolicy)]++
		} else {
			result.Summary.ByPolicy["<default>"]++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type CRAggRuleResult2305 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalClusterRoles int `json:"totalClusterRoles"`
		WithAggregation   int `json:"withAggregationRule"`
	} `json:"summary"`
}

func (s *Server) handleCRAggRule2305(w http.ResponseWriter, r *http.Request) {
	result := CRAggRuleResult2305{ScannedAt: time.Now()}
	crList, _ := s.clientset.RbacV1().ClusterRoles().List(r.Context(), metav1.ListOptions{})
	for _, cr := range crList.Items {
		result.Summary.TotalClusterRoles++
		if cr.AggregationRule != nil && len(cr.AggregationRule.ClusterRoleSelectors) > 0 {
			result.Summary.WithAggregation++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
