package dashboard

import (
	"net/http"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v22.39 — Security Dimension (Round 59)
// 1. Pod Windows GMSA Credential Spec Audit
// 2. Secret Type ServiceAccount Token Tracker
// 3. NetworkPolicy CIDR Exception Count
// ============================================================

type GMSACredSpecResult2239 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods int `json:"totalPods"`
		WithGMSA  int `json:"withGMSACredentialSpec"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleGMSACredSpec2239(w http.ResponseWriter, r *http.Request) {
	result := GMSACredSpecResult2239{ScannedAt: time.Now()}
	score := 100
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		if pod.Spec.SecurityContext != nil && pod.Spec.SecurityContext.WindowsOptions != nil &&
			pod.Spec.SecurityContext.WindowsOptions.GMSACredentialSpecName != nil &&
			*pod.Spec.SecurityContext.WindowsOptions.GMSACredentialSpecName != "" {
			result.Summary.WithGMSA++
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 2. Secret Type SA Token
type SecSATokenResult2239 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalSecrets int            `json:"totalSecrets"`
		SATokens     int            `json:"serviceAccountTokens"`
		ByType       map[string]int `json:"byType"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleSecSAToken2239(w http.ResponseWriter, r *http.Request) {
	result := SecSATokenResult2239{ScannedAt: time.Now()}
	score := 100
	secretList, _ := s.clientset.CoreV1().Secrets("").List(r.Context(), metav1.ListOptions{})
	result.Summary.ByType = make(map[string]int)
	for _, secret := range secretList.Items {
		result.Summary.TotalSecrets++
		t := string(secret.Type)
		result.Summary.ByType[t]++
		if t == "kubernetes.io/service-account-token" {
			result.Summary.SATokens++
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 3. NP CIDR Exception
type NPCIDRExcResult2239 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNP        int `json:"totalNetworkPolicies"`
		WithCIDRExcept int `json:"withCIDRException"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleNPCIDRExc2239(w http.ResponseWriter, r *http.Request) {
	result := NPCIDRExcResult2239{ScannedAt: time.Now()}
	score := 100
	npList, _ := s.clientset.NetworkingV1().NetworkPolicies("").List(r.Context(), metav1.ListOptions{})
	for _, np := range npList.Items {
		result.Summary.TotalNP++
		checkPeer := func(peers []interface{}) bool { return false }
		_ = checkPeer
		for _, rule := range np.Spec.Ingress {
			for _, peer := range rule.From {
				if peer.IPBlock != nil && len(peer.IPBlock.Except) > 0 {
					result.Summary.WithCIDRExcept++
					goto next3
				}
			}
		}
	next3:
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}
