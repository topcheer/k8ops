package dashboard

import (
	"fmt"
	"net/http"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v21.61 — Security Dimension (Round 46)
// 1. Pod RunAsUser UID Range Audit
// 2. Namespace Secret Count Per Type
// 3. NetworkPolicy Egress Rule Port Coverage
// ============================================================

type UIDRangeResult2161 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods     int `json:"totalPods"`
		WithRunAsUser int `json:"withRunAsUser"`
		RootUID       int `json:"rootUIDPods"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleUIDRange2161(w http.ResponseWriter, r *http.Request) {
	result := UIDRangeResult2161{ScannedAt: time.Now()}
	score := 100
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		uid := int64(-1)
		if pod.Spec.SecurityContext != nil && pod.Spec.SecurityContext.RunAsUser != nil {
			uid = *pod.Spec.SecurityContext.RunAsUser
		}
		for _, c := range pod.Spec.Containers {
			if c.SecurityContext != nil && c.SecurityContext.RunAsUser != nil {
				uid = *c.SecurityContext.RunAsUser
			}
		}
		if uid >= 0 {
			result.Summary.WithRunAsUser++
		}
		if uid == 0 {
			result.Summary.RootUID++
			score -= 3
		}
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	if result.Summary.RootUID > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d pods running as root (UID 0)", result.Summary.RootUID))
	}
	writeJSON(w, result)
}

// 2. Secret Count Per Type
type SecCountTypeResult2161 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalSecrets int            `json:"totalSecrets"`
		ByType       map[string]int `json:"byType"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleSecCountType2161(w http.ResponseWriter, r *http.Request) {
	result := SecCountTypeResult2161{ScannedAt: time.Now()}
	score := 100
	secretList, _ := s.clientset.CoreV1().Secrets("").List(r.Context(), metav1.ListOptions{})
	result.Summary.ByType = make(map[string]int)
	for _, secret := range secretList.Items {
		result.Summary.TotalSecrets++
		result.Summary.ByType[string(secret.Type)]++
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 3. NP Egress Port Coverage
type NPEgressPortResult2161 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNP         int `json:"totalNetworkPolicies"`
		WithEgressPorts int `json:"withEgressPortRules"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleNPEgressPort2161(w http.ResponseWriter, r *http.Request) {
	result := NPEgressPortResult2161{ScannedAt: time.Now()}
	score := 100
	npList, _ := s.clientset.NetworkingV1().NetworkPolicies("").List(r.Context(), metav1.ListOptions{})
	for _, np := range npList.Items {
		result.Summary.TotalNP++
		for _, rule := range np.Spec.Egress {
			if len(rule.Ports) > 0 {
				result.Summary.WithEgressPorts++
				break
			}
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}
