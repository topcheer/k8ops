package dashboard

import (
	"net/http"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v21.97 — Security Dimension (Round 52)
// 1. Pod Seccomp Localhost Profile Audit
// 2. ServiceAccount Long-lived Token Risk
// 3. NetworkPolicy Egress Default Allow Risk
// ============================================================

type SeccompLocalhostResult2197 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods        int `json:"totalPods"`
		WithSeccomp      int `json:"withSeccomp"`
		LocalhostProfile int `json:"localhostProfile"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleSeccompLocalhost2197(w http.ResponseWriter, r *http.Request) {
	result := SeccompLocalhostResult2197{ScannedAt: time.Now()}
	score := 100
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		if pod.Spec.SecurityContext != nil && pod.Spec.SecurityContext.SeccompProfile != nil {
			sp := pod.Spec.SecurityContext.SeccompProfile
			result.Summary.WithSeccomp++
			if sp.Type == corev1.SeccompProfileTypeLocalhost {
				result.Summary.LocalhostProfile++
			}
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 2. SA Long-lived Token Risk
type SATokenRiskResult2197 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalSAs      int `json:"totalServiceAccounts"`
		WithToken     int `json:"withLongLivedToken"`
		WithAutoMount int `json:"autoMountEnabled"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleSATokenRisk2197(w http.ResponseWriter, r *http.Request) {
	result := SATokenRiskResult2197{ScannedAt: time.Now()}
	score := 100
	saList, _ := s.clientset.CoreV1().ServiceAccounts("").List(r.Context(), metav1.ListOptions{})
	for _, sa := range saList.Items {
		result.Summary.TotalSAs++
		if len(sa.Secrets) > 0 {
			result.Summary.WithToken++
			score -= 1
		}
		if sa.AutomountServiceAccountToken == nil || *sa.AutomountServiceAccountToken {
			result.Summary.WithAutoMount++
		}
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 3. NP Egress Default Allow
type NPEgressDefaultResult2197 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNS    int `json:"totalNamespaces"`
		EgressOpen int `json:"egressDefaultAllow"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleNPEgressDefault2197(w http.ResponseWriter, r *http.Request) {
	result := NPEgressDefaultResult2197{ScannedAt: time.Now()}
	score := 100
	nsList, _ := s.clientset.CoreV1().Namespaces().List(r.Context(), metav1.ListOptions{})
	npList, _ := s.clientset.NetworkingV1().NetworkPolicies("").List(r.Context(), metav1.ListOptions{})
	nsWithEgressNP := make(map[string]bool)
	for _, np := range npList.Items {
		for _, pt := range np.Spec.PolicyTypes {
			if pt == "Egress" {
				nsWithEgressNP[np.Namespace] = true
			}
		}
	}
	systemNS := map[string]bool{"kube-system": true, "kube-public": true, "kube-node-lease": true, "k8ops-system": true}
	for _, ns := range nsList.Items {
		if systemNS[ns.Name] {
			continue
		}
		result.Summary.TotalNS++
		if !nsWithEgressNP[ns.Name] {
			result.Summary.EgressOpen++
			score -= 2
		}
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}
