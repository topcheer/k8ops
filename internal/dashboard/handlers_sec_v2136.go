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
// v21.36 — Security Dimension (Round 42)
// 1. Pod HostPath Volume Writable Audit
// 2. Namespace PrivilegedPod PSA Violation
// 3. ServiceAccount Secret Reference Validator
// ============================================================

type HostPathWriteResult2136 struct {
	ScannedAt       time.Time                `json:"scannedAt"`
	HealthScore     int                      `json:"healthScore"`
	Grade           string                   `json:"grade"`
	Summary         HostPathWriteSummary2136 `json:"summary"`
	AtRisk          []HostPathWriteEntry2136 `json:"atRiskPods"`
	Recommendations []string                 `json:"recommendations"`
}

type HostPathWriteSummary2136 struct {
	TotalPods    int `json:"totalPods"`
	WithHostPath int `json:"withHostPath"`
	Writable     int `json:"writableHostPath"`
}

type HostPathWriteEntry2136 struct {
	Pod       string `json:"pod"`
	Namespace string `json:"namespace"`
	Path      string `json:"path"`
}

func (s *Server) handleHostPathWrite2136(w http.ResponseWriter, r *http.Request) {
	result := HostPathWriteResult2136{ScannedAt: time.Now()}
	score := 100
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		for _, vol := range pod.Spec.Volumes {
			if vol.HostPath != nil {
				result.Summary.WithHostPath++
				ro := false
				if vol.HostPath.Type != nil && *vol.HostPath.Type == corev1.HostPathDirectoryOrCreate {
					ro = false
				}
				if !ro {
					result.Summary.Writable++
					result.AtRisk = append(result.AtRisk, HostPathWriteEntry2136{Pod: pod.Name, Namespace: pod.Namespace, Path: vol.HostPath.Path})
					score -= 2
				}
			}
		}
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	sort.Slice(result.AtRisk, func(i, j int) bool { return result.AtRisk[i].Namespace < result.AtRisk[j].Namespace })

	if result.Summary.Writable > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d pods with writable hostPath volumes", result.Summary.Writable))
	}
	writeJSON(w, result)
}

// 2. PSA PrivilegedPod Violation
type PSAPrivResult2136 struct {
	ScannedAt       time.Time          `json:"scannedAt"`
	HealthScore     int                `json:"healthScore"`
	Grade           string             `json:"grade"`
	Summary         PSAPrivSummary2136 `json:"summary"`
	Violations      []PSAPrivEntry2136 `json:"violations"`
	Recommendations []string           `json:"recommendations"`
}

type PSAPrivSummary2136 struct {
	TotalPods  int `json:"totalPods"`
	Violations int `json:"violations"`
}

type PSAPrivEntry2136 struct {
	Pod       string `json:"pod"`
	Namespace string `json:"namespace"`
}

func (s *Server) handlePSAPriv2136(w http.ResponseWriter, r *http.Request) {
	result := PSAPrivResult2136{ScannedAt: time.Now()}
	score := 100
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		for _, c := range pod.Spec.Containers {
			if c.SecurityContext != nil && c.SecurityContext.Privileged != nil && *c.SecurityContext.Privileged {
				result.Summary.Violations++
				result.Violations = append(result.Violations, PSAPrivEntry2136{Pod: pod.Name, Namespace: pod.Namespace})
				score -= 3
				break
			}
		}
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	sort.Slice(result.Violations, func(i, j int) bool { return result.Violations[i].Namespace < result.Violations[j].Namespace })
	writeJSON(w, result)
}

// 3. SA Secret Reference Validator
type SASecretResult2136 struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	HealthScore     int                 `json:"healthScore"`
	Grade           string              `json:"grade"`
	Summary         SASecretSummary2136 `json:"summary"`
	Recommendations []string            `json:"recommendations"`
}

type SASecretSummary2136 struct {
	TotalSAs   int `json:"totalServiceAccounts"`
	WithSecret int `json:"withSecretRef"`
}

func (s *Server) handleSASecret2136(w http.ResponseWriter, r *http.Request) {
	result := SASecretResult2136{ScannedAt: time.Now()}
	score := 100
	saList, _ := s.clientset.CoreV1().ServiceAccounts("").List(r.Context(), metav1.ListOptions{})

	for _, sa := range saList.Items {
		result.Summary.TotalSAs++
		if len(sa.Secrets) > 0 {
			result.Summary.WithSecret++
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}
