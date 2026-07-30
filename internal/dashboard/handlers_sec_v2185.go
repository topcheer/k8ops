package dashboard

import (
	"net/http"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v21.85 — Security Dimension (Round 50)
// 1. Pod FSGroup Change Policy Audit
// 2. Namespace PSA Audit Warning Level
// 3. NetworkPolicy Peer Namespace Selector
// ============================================================

type FSGroupPolicyResult2185 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods   int `json:"totalPods"`
		WithFSGroup int `json:"withFSGroup"`
		WithPolicy  int `json:"withChangePolicy"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleFSGroupPolicy2185(w http.ResponseWriter, r *http.Request) {
	result := FSGroupPolicyResult2185{ScannedAt: time.Now()}
	score := 100
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		if pod.Spec.SecurityContext != nil {
			if pod.Spec.SecurityContext.FSGroup != nil {
				result.Summary.WithFSGroup++
			}
			if pod.Spec.SecurityContext.FSGroupChangePolicy != nil {
				result.Summary.WithPolicy++
			}
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 2. PSA Audit Warning Level
type PSAAuditWarnResult2185 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNS   int `json:"totalNamespaces"`
		WithAudit int `json:"withAuditLevel"`
		WithWarn  int `json:"withWarnLevel"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handlePSAAuditWarn2185(w http.ResponseWriter, r *http.Request) {
	result := PSAAuditWarnResult2185{ScannedAt: time.Now()}
	score := 100
	nsList, _ := s.clientset.CoreV1().Namespaces().List(r.Context(), metav1.ListOptions{})
	systemNS := map[string]bool{"kube-system": true, "kube-public": true, "kube-node-lease": true, "k8ops-system": true}
	for _, ns := range nsList.Items {
		if systemNS[ns.Name] {
			continue
		}
		result.Summary.TotalNS++
		if ns.Labels["pod-security.kubernetes.io/audit"] != "" {
			result.Summary.WithAudit++
		}
		if ns.Labels["pod-security.kubernetes.io/warn"] != "" {
			result.Summary.WithWarn++
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 3. NP Peer NS Selector
type NPPeerNSSelectorResult2185 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNP        int `json:"totalNetworkPolicies"`
		WithNSSelector int `json:"withNamespaceSelector"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleNPPeerNSSelector2185(w http.ResponseWriter, r *http.Request) {
	result := NPPeerNSSelectorResult2185{ScannedAt: time.Now()}
	score := 100
	npList, _ := s.clientset.NetworkingV1().NetworkPolicies("").List(r.Context(), metav1.ListOptions{})
	for _, np := range npList.Items {
		result.Summary.TotalNP++
		checkRule := func(peers []interface{}) bool { return false }
		_ = checkRule
		// Check ingress peers
		for _, rule := range np.Spec.Ingress {
			for _, peer := range rule.From {
				if peer.NamespaceSelector != nil {
					result.Summary.WithNSSelector++
					goto next
				}
			}
		}
		// Check egress peers
		for _, rule := range np.Spec.Egress {
			for _, peer := range rule.To {
				if peer.NamespaceSelector != nil {
					result.Summary.WithNSSelector++
					goto next
				}
			}
		}
	next:
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}
