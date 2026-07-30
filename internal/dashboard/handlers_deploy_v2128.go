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
// v21.28 — Deployment Dimension (Round 41)
// 1. Container Resource Delta Analysis
// 2. Pod Eviction Budget Multiplier
// 3. Deployment Container Restart Budget
// ============================================================

type ResDeltaResult2128 struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	HealthScore     int                 `json:"healthScore"`
	Grade           string              `json:"grade"`
	Summary         ResDeltaSummary2128 `json:"summary"`
	LargeDelta      []ResDeltaEntry2128 `json:"largeDelta"`
	Recommendations []string            `json:"recommendations"`
}

type ResDeltaSummary2128 struct {
	TotalContainers int `json:"totalContainers"`
	LargeDelta      int `json:"largeDelta"`
}

type ResDeltaEntry2128 struct {
	Pod       string `json:"pod"`
	Namespace string `json:"namespace"`
	DeltaPct  int    `json:"deltaPct"`
}

func (s *Server) handleResDelta2128(w http.ResponseWriter, r *http.Request) {
	result := ResDeltaResult2128{ScannedAt: time.Now()}
	score := 100
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++
			req := c.Resources.Requests.Cpu()
			lim := c.Resources.Limits.Cpu()
			if req.IsZero() || lim.IsZero() {
				continue
			}
			delta := int((1 - req.AsApproximateFloat64()/lim.AsApproximateFloat64()) * 100)
			if delta > 90 {
				result.Summary.LargeDelta++
				result.LargeDelta = append(result.LargeDelta, ResDeltaEntry2128{Pod: pod.Name, Namespace: pod.Namespace, DeltaPct: delta})
				score -= 1
			}
		}
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	sort.Slice(result.LargeDelta, func(i, j int) bool { return result.LargeDelta[i].DeltaPct > result.LargeDelta[j].DeltaPct })
	writeJSON(w, result)
}

// 2. Eviction Budget Multiplier
type EvictMultResult2128 struct {
	ScannedAt       time.Time            `json:"scannedAt"`
	HealthScore     int                  `json:"healthScore"`
	Grade           string               `json:"grade"`
	Summary         EvictMultSummary2128 `json:"summary"`
	Recommendations []string             `json:"recommendations"`
}

type EvictMultSummary2128 struct {
	TotalMultiReplica int `json:"totalMultiReplica"`
	WithPDB           int `json:"withPDB"`
	EvictProtected    int `json:"evictionProtected"`
}

func (s *Server) handleEvictMult2128(w http.ResponseWriter, r *http.Request) {
	result := EvictMultResult2128{ScannedAt: time.Now()}
	score := 100
	deployList, _ := s.clientset.AppsV1().Deployments("").List(r.Context(), metav1.ListOptions{})
	pdbList, _ := s.clientset.PolicyV1().PodDisruptionBudgets("").List(r.Context(), metav1.ListOptions{})

	nsWithPDB := make(map[string]bool)
	for _, pdb := range pdbList.Items {
		nsWithPDB[pdb.Namespace] = true
	}

	for _, dep := range deployList.Items {
		replicas := int32(1)
		if dep.Spec.Replicas != nil {
			replicas = *dep.Spec.Replicas
		}
		if replicas <= 1 {
			continue
		}
		result.Summary.TotalMultiReplica++
		if nsWithPDB[dep.Namespace] {
			result.Summary.WithPDB++
			result.Summary.EvictProtected++
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 3. Container Restart Budget
type CtnrRestartResult2128 struct {
	ScannedAt       time.Time              `json:"scannedAt"`
	HealthScore     int                    `json:"healthScore"`
	Grade           string                 `json:"grade"`
	Summary         CtnrRestartSummary2128 `json:"summary"`
	HighRestart     []CtnrRestartEntry2128 `json:"highRestartContainers"`
	Recommendations []string               `json:"recommendations"`
}

type CtnrRestartSummary2128 struct {
	TotalPods     int   `json:"totalPods"`
	TotalRestarts int32 `json:"totalRestarts"`
	HighRestart   int   `json:"highRestartPods"`
}

type CtnrRestartEntry2128 struct {
	Pod       string `json:"pod"`
	Namespace string `json:"namespace"`
	Restarts  int32  `json:"restarts"`
}

func (s *Server) handleCtnrRestart2128(w http.ResponseWriter, r *http.Request) {
	result := CtnrRestartResult2128{ScannedAt: time.Now()}
	score := 100
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		var maxR int32
		for _, cs := range pod.Status.ContainerStatuses {
			result.Summary.TotalRestarts += cs.RestartCount
			if cs.RestartCount > maxR {
				maxR = cs.RestartCount
			}
		}
		if maxR > 3 {
			result.Summary.HighRestart++
			result.HighRestart = append(result.HighRestart, CtnrRestartEntry2128{Pod: pod.Name, Namespace: pod.Namespace, Restarts: maxR})
			score -= 2
		}
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	sort.Slice(result.HighRestart, func(i, j int) bool { return result.HighRestart[i].Restarts > result.HighRestart[j].Restarts })

	if result.Summary.HighRestart > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d pods with >3 restarts", result.Summary.HighRestart))
	}
	writeJSON(w, result)
}
