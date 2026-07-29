package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v20.88 — Security Dimension (Round 34)
// 1. SA Token Age — service account token secret age
// 2. Pod Privilege Escalation — allowPrivilegeEscalation flag
// 3. NetworkPolicy Direction Coverage — ingress/egress policy spread
// ============================================================

type SATokenAgeResult2088 struct {
	ScannedAt       time.Time             `json:"scannedAt"`
	HealthScore     int                   `json:"healthScore"`
	Grade           string                `json:"grade"`
	Summary         SATokenAgeSummary2088 `json:"summary"`
	OldTokens       []SATokenAgeEntry2088 `json:"oldTokens"`
	Recommendations []string              `json:"recommendations"`
}

type SATokenAgeSummary2088 struct {
	TotalTokens int `json:"totalTokens"`
	OldTokens   int `json:"oldTokens"`
}

type SATokenAgeEntry2088 struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	AgeDays   int    `json:"ageDays"`
}

func (s *Server) handleSATokenAge2088(w http.ResponseWriter, r *http.Request) {
	result := SATokenAgeResult2088{ScannedAt: time.Now()}
	score := 100
	secretList, _ := s.clientset.CoreV1().Secrets("").List(r.Context(), metav1.ListOptions{})

	now := time.Now()
	for _, secret := range secretList.Items {
		if secret.Type != corev1.SecretTypeServiceAccountToken {
			continue
		}
		result.Summary.TotalTokens++
		ageDays := int(now.Sub(secret.CreationTimestamp.Time).Hours() / 24)
		if ageDays > 180 {
			result.Summary.OldTokens++
			result.OldTokens = append(result.OldTokens, SATokenAgeEntry2088{
				Name: secret.Name, Namespace: secret.Namespace, AgeDays: ageDays,
			})
			score -= 1
		}
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	sort.Slice(result.OldTokens, func(i, j int) bool { return result.OldTokens[i].AgeDays > result.OldTokens[j].AgeDays })
	writeJSON(w, result)
}

// 2. Pod Privilege Escalation
type PrivEscResult2088 struct {
	ScannedAt       time.Time          `json:"scannedAt"`
	HealthScore     int                `json:"healthScore"`
	Grade           string             `json:"grade"`
	Summary         PrivEscSummary2088 `json:"summary"`
	AtRisk          []PrivEscEntry2088 `json:"atRiskContainers"`
	Recommendations []string           `json:"recommendations"`
}

type PrivEscSummary2088 struct {
	TotalContainers int `json:"totalContainers"`
	AtRisk          int `json:"atRisk"`
}

type PrivEscEntry2088 struct {
	Pod       string `json:"pod"`
	Namespace string `json:"namespace"`
}

func (s *Server) handlePrivEsc2088(w http.ResponseWriter, r *http.Request) {
	result := PrivEscResult2088{ScannedAt: time.Now()}
	score := 100
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++
			if c.SecurityContext != nil && c.SecurityContext.AllowPrivilegeEscalation != nil && *c.SecurityContext.AllowPrivilegeEscalation {
				result.Summary.AtRisk++
				result.AtRisk = append(result.AtRisk, PrivEscEntry2088{Pod: pod.Name, Namespace: pod.Namespace})
				score -= 3
			}
		}
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)

	if result.Summary.AtRisk > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d containers allow privilege escalation", result.Summary.AtRisk))
	}
	writeJSON(w, result)
}

// 3. NetworkPolicy Direction Coverage
type NPDirResult2088 struct {
	ScannedAt       time.Time        `json:"scannedAt"`
	HealthScore     int              `json:"healthScore"`
	Grade           string           `json:"grade"`
	Summary         NPDirSummary2088 `json:"summary"`
	Recommendations []string         `json:"recommendations"`
}

type NPDirSummary2088 struct {
	TotalNS     int `json:"totalNamespaces"`
	WithIngress int `json:"withIngressPolicy"`
	WithEgress  int `json:"withEgressPolicy"`
}

func (s *Server) handleNPDir2088(w http.ResponseWriter, r *http.Request) {
	result := NPDirResult2088{ScannedAt: time.Now()}
	score := 100
	nsList, _ := s.clientset.CoreV1().Namespaces().List(r.Context(), metav1.ListOptions{})
	npList, _ := s.clientset.NetworkingV1().NetworkPolicies("").List(r.Context(), metav1.ListOptions{})

	nsIngress := make(map[string]bool)
	nsEgress := make(map[string]bool)
	for _, np := range npList.Items {
		for _, pt := range np.Spec.PolicyTypes {
			if pt == "Ingress" {
				nsIngress[np.Namespace] = true
			}
			if pt == "Egress" {
				nsEgress[np.Namespace] = true
			}
		}
	}

	systemNS := map[string]bool{"kube-system": true, "kube-public": true, "kube-node-lease": true, "k8ops-system": true}
	for _, ns := range nsList.Items {
		if systemNS[ns.Name] {
			continue
		}
		result.Summary.TotalNS++
		if nsIngress[ns.Name] {
			result.Summary.WithIngress++
		}
		if nsEgress[ns.Name] {
			result.Summary.WithEgress++
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}
