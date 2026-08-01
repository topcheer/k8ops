package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v25.99 Security: Pod HostIPC Detail, Secret Creation Timestamp Distribution, ClusterRole Rule APIGroup Summary
type HostIPCDetailResult2599 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods int `json:"totalPods"`
		HostIPC   int `json:"hostIPC"`
	}
}

func (s *Server) handleHostIPCDetail2599(w http.ResponseWriter, r *http.Request) {
	result := HostIPCDetailResult2599{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		if pod.Spec.HostIPC {
			result.Summary.HostIPC++
		}
	}
	score := 100
	if result.Summary.HostIPC > 0 {
		score = 100 - result.Summary.HostIPC*10
		if score < 0 {
			score = 0
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

type SecretCreationDistResult2599 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalSecrets int `json:"totalSecrets"`
		Last7d       int `json:"createdLast7d"`
		Last30d      int `json:"createdLast30d"`
	}
}

func (s *Server) handleSecretCreationDist2599(w http.ResponseWriter, r *http.Request) {
	result := SecretCreationDistResult2599{ScannedAt: time.Now()}
	now := time.Now()
	cutoff7d := now.Add(-7 * 24 * time.Hour)
	cutoff30d := now.Add(-30 * 24 * time.Hour)
	secretList, _ := s.clientset.CoreV1().Secrets("").List(r.Context(), metav1.ListOptions{})
	for _, secret := range secretList.Items {
		result.Summary.TotalSecrets++
		ct := secret.CreationTimestamp.Time
		if ct.After(cutoff7d) {
			result.Summary.Last7d++
		}
		if ct.After(cutoff30d) {
			result.Summary.Last30d++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type CRRuleAPIGroupResult2599 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalCRs   int            `json:"totalClusterRoles"`
		ByAPIGroup map[string]int `json:"byAPIGroup"`
	}
}

func (s *Server) handleCRRuleAPIGroup2599(w http.ResponseWriter, r *http.Request) {
	result := CRRuleAPIGroupResult2599{ScannedAt: time.Now()}
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
