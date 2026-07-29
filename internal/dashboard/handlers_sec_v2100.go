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
// v21.00 — Security Dimension (Round 36)
// 1. NetworkPolicy Empty Selector — NP matching all pods
// 2. Pod Host Alias Audit — /etc/hosts modifications
// 3. Secret Mount Path Tracker — where secrets are mounted
// ============================================================

type NPEmptyResult2100 struct {
	ScannedAt       time.Time          `json:"scannedAt"`
	HealthScore     int                `json:"healthScore"`
	Grade           string             `json:"grade"`
	Summary         NPEmptySummary2100 `json:"summary"`
	BroadNP         []NPEmptyEntry2100 `json:"broadNetworkPolicies"`
	Recommendations []string           `json:"recommendations"`
}

type NPEmptySummary2100 struct {
	TotalNP int `json:"totalNetworkPolicies"`
	BroadNP int `json:"broadNetworkPolicies"`
}

type NPEmptyEntry2100 struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
}

func (s *Server) handleNPEmpty2100(w http.ResponseWriter, r *http.Request) {
	result := NPEmptyResult2100{ScannedAt: time.Now()}
	score := 100
	npList, _ := s.clientset.NetworkingV1().NetworkPolicies("").List(r.Context(), metav1.ListOptions{})

	for _, np := range npList.Items {
		result.Summary.TotalNP++
		// Empty pod selector matches ALL pods in namespace
		if len(np.Spec.PodSelector.MatchLabels) == 0 && len(np.Spec.PodSelector.MatchExpressions) == 0 {
			result.Summary.BroadNP++
			result.BroadNP = append(result.BroadNP, NPEmptyEntry2100{Name: np.Name, Namespace: np.Namespace})
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	sort.Slice(result.BroadNP, func(i, j int) bool { return result.BroadNP[i].Namespace < result.BroadNP[j].Namespace })
	writeJSON(w, result)
}

// 2. Pod Host Alias Audit
type HostAliasResult2100 struct {
	ScannedAt       time.Time            `json:"scannedAt"`
	HealthScore     int                  `json:"healthScore"`
	Grade           string               `json:"grade"`
	Summary         HostAliasSummary2100 `json:"summary"`
	WithAlias       []HostAliasEntry2100 `json:"withHostAlias"`
	Recommendations []string             `json:"recommendations"`
}

type HostAliasSummary2100 struct {
	TotalPods int `json:"totalPods"`
	WithAlias int `json:"withHostAlias"`
}

type HostAliasEntry2100 struct {
	Pod       string `json:"pod"`
	Namespace string `json:"namespace"`
}

func (s *Server) handleHostAlias2100(w http.ResponseWriter, r *http.Request) {
	result := HostAliasResult2100{ScannedAt: time.Now()}
	score := 100
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		if len(pod.Spec.HostAliases) > 0 {
			result.Summary.WithAlias++
			result.WithAlias = append(result.WithAlias, HostAliasEntry2100{Pod: pod.Name, Namespace: pod.Namespace})
			score -= 3
		}
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)

	if result.Summary.WithAlias > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d pods use hostAliases — use DNS instead", result.Summary.WithAlias))
	}
	writeJSON(w, result)
}

// 3. Secret Mount Path Tracker
type SecMountResult2100 struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	HealthScore     int                 `json:"healthScore"`
	Grade           string              `json:"grade"`
	Summary         SecMountSummary2100 `json:"summary"`
	Recommendations []string            `json:"recommendations"`
}

type SecMountSummary2100 struct {
	TotalSecretMounts int `json:"totalSecretMounts"`
	EnvSecretMounts   int `json:"envSecretMounts"`
	VolSecretMounts   int `json:"volSecretMounts"`
}

func (s *Server) handleSecMount2100(w http.ResponseWriter, r *http.Request) {
	result := SecMountResult2100{ScannedAt: time.Now()}
	score := 100
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, vol := range pod.Spec.Volumes {
			if vol.Secret != nil {
				result.Summary.TotalSecretMounts++
				result.Summary.VolSecretMounts++
			}
		}
		for _, c := range pod.Spec.Containers {
			for _, env := range c.Env {
				if env.ValueFrom != nil && env.ValueFrom.SecretKeyRef != nil {
					result.Summary.TotalSecretMounts++
					result.Summary.EnvSecretMounts++
				}
			}
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}
