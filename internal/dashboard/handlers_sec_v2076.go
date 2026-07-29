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
// v20.76 — Security Dimension (Round 32)
// 1. Privileged Container Inventory — all privileged containers
// 2. Volume Mount Permission — secret/projected volume exposure
// 3. ClusterRole Aggregation — aggregated ClusterRole coverage
// ============================================================

type PrivInvResult2076 struct {
	ScannedAt       time.Time          `json:"scannedAt"`
	HealthScore     int                `json:"healthScore"`
	Grade           string             `json:"grade"`
	Summary         PrivInvSummary2076 `json:"summary"`
	Privileged      []PrivInvEntry2076 `json:"privilegedContainers"`
	Recommendations []string           `json:"recommendations"`
}

type PrivInvSummary2076 struct {
	TotalContainers int `json:"totalContainers"`
	Privileged      int `json:"privileged"`
}

type PrivInvEntry2076 struct {
	Pod       string `json:"pod"`
	Namespace string `json:"namespace"`
	Container string `json:"container"`
}

func (s *Server) handlePrivInv2076(w http.ResponseWriter, r *http.Request) {
	result := PrivInvResult2076{ScannedAt: time.Now()}
	score := 100
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++
			if c.SecurityContext != nil && c.SecurityContext.Privileged != nil && *c.SecurityContext.Privileged {
				result.Summary.Privileged++
				result.Privileged = append(result.Privileged, PrivInvEntry2076{
					Pod: pod.Name, Namespace: pod.Namespace, Container: c.Name,
				})
				score -= 5
			}
		}
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	sort.Slice(result.Privileged, func(i, j int) bool { return result.Privileged[i].Namespace < result.Privileged[j].Namespace })

	if result.Summary.Privileged > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d privileged containers — remove privileged flag", result.Summary.Privileged))
	}
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 2. Volume Mount Permission
// ---------------------------------------------------------------

type VolPermResult2076 struct {
	ScannedAt       time.Time          `json:"scannedAt"`
	HealthScore     int                `json:"healthScore"`
	Grade           string             `json:"grade"`
	Summary         VolPermSummary2076 `json:"summary"`
	ReadOnlyMounts  []VolPermEntry2076 `json:"readOnlyMounts"`
	Recommendations []string           `json:"recommendations"`
}

type VolPermSummary2076 struct {
	TotalVolumes   int `json:"totalVolumes"`
	ReadOnly       int `json:"readOnlyVolumes"`
	WritableSecret int `json:"writableSecretVolumes"`
}

type VolPermEntry2076 struct {
	Pod    string `json:"pod"`
	Volume string `json:"volume"`
}

func (s *Server) handleVolPerm2076(w http.ResponseWriter, r *http.Request) {
	result := VolPermResult2076{ScannedAt: time.Now()}
	score := 100
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, vol := range pod.Spec.Volumes {
			result.Summary.TotalVolumes++

			isSecret := vol.Secret != nil || vol.Projected != nil
			isReadOnly := false
			if vol.Secret != nil && vol.Secret.Optional != nil && !*vol.Secret.Optional {
				isReadOnly = true
			}

			if isReadOnly {
				result.Summary.ReadOnly++
			} else if isSecret {
				result.Summary.WritableSecret++
				score -= 2
			}
		}
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)

	if result.Summary.WritableSecret > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d writable secret volumes — mount read-only", result.Summary.WritableSecret))
	}
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 3. ClusterRole Aggregation
// ---------------------------------------------------------------

type CRAggResult2076 struct {
	ScannedAt       time.Time        `json:"scannedAt"`
	HealthScore     int              `json:"healthScore"`
	Grade           string           `json:"grade"`
	Summary         CRAggSummary2076 `json:"summary"`
	Recommendations []string         `json:"recommendations"`
}

type CRAggSummary2076 struct {
	TotalClusterRoles int `json:"totalClusterRoles"`
	AggregatedRoles   int `json:"aggregatedRoles"`
	SystemRoles       int `json:"systemRoles"`
}

func (s *Server) handleCRAgg2076(w http.ResponseWriter, r *http.Request) {
	result := CRAggResult2076{ScannedAt: time.Now()}
	score := 100

	crList, _ := s.clientset.RbacV1().ClusterRoles().List(r.Context(), metav1.ListOptions{})

	for _, cr := range crList.Items {
		result.Summary.TotalClusterRoles++
		if cr.AggregationRule != nil && len(cr.AggregationRule.ClusterRoleSelectors) > 0 {
			result.Summary.AggregatedRoles++
		}
		if len(cr.Name) > 7 && cr.Name[:7] == "system:" {
			result.Summary.SystemRoles++
		}
	}

	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}
