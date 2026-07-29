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
// v20.86 — Deployment Dimension (Round 34)
// 1. Container Resource Gap Wide — request much less than limit
// 2. Pod QoS Class Distribution — Guaranteed/Burstable/BestEffort
// 3. Deployment Revision Staleness — old ReplicaSet count
// ============================================================

type ResGapWideResult2086 struct {
	ScannedAt       time.Time             `json:"scannedAt"`
	HealthScore     int                   `json:"healthScore"`
	Grade           string                `json:"grade"`
	Summary         ResGapWideSummary2086 `json:"summary"`
	WideGap         []ResGapWideEntry2086 `json:"wideGap"`
	Recommendations []string              `json:"recommendations"`
}

type ResGapWideSummary2086 struct {
	TotalContainers int `json:"totalContainers"`
	WideGap         int `json:"wideGap"`
}

type ResGapWideEntry2086 struct {
	Pod       string  `json:"pod"`
	Namespace string  `json:"namespace"`
	CPURatio  float64 `json:"cpuRatio"`
}

func (s *Server) handleResGapWide2086(w http.ResponseWriter, r *http.Request) {
	result := ResGapWideResult2086{ScannedAt: time.Now()}
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
			if lim.IsZero() || req.IsZero() {
				continue
			}
			ratio := req.AsApproximateFloat64() / lim.AsApproximateFloat64()
			if ratio < 0.05 {
				result.Summary.WideGap++
				result.WideGap = append(result.WideGap, ResGapWideEntry2086{
					Pod: pod.Name, Namespace: pod.Namespace, CPURatio: ratio,
				})
				score -= 1
			}
		}
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	sort.Slice(result.WideGap, func(i, j int) bool { return result.WideGap[i].CPURatio < result.WideGap[j].CPURatio })

	if result.Summary.WideGap > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d containers with request <5%% of limit", result.Summary.WideGap))
	}
	writeJSON(w, result)
}

// 2. Pod QoS Distribution
type QoSDistResult2086 struct {
	ScannedAt       time.Time          `json:"scannedAt"`
	HealthScore     int                `json:"healthScore"`
	Grade           string             `json:"grade"`
	Summary         QoSDistSummary2086 `json:"summary"`
	Recommendations []string           `json:"recommendations"`
}

type QoSDistSummary2086 struct {
	TotalPods  int `json:"totalPods"`
	Guaranteed int `json:"guaranteed"`
	Burstable  int `json:"burstable"`
	BestEffort int `json:"bestEffort"`
}

func (s *Server) handleQoSDist2086(w http.ResponseWriter, r *http.Request) {
	result := QoSDistResult2086{ScannedAt: time.Now()}
	score := 100
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++

		allEqual := true
		hasReq := false
		hasLim := true
		for _, c := range pod.Spec.Containers {
			if c.Resources.Requests.Cpu().IsZero() && c.Resources.Requests.Memory().IsZero() {
				hasReq = false
			}
			if c.Resources.Limits.Cpu().IsZero() || c.Resources.Limits.Memory().IsZero() {
				hasLim = false
			}
		}
		if !hasReq && !hasLim {
			result.Summary.BestEffort++
		} else if hasLim && allEqual {
			result.Summary.Guaranteed++
		} else {
			result.Summary.Burstable++
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 3. Deployment Revision Staleness
type RevStaleResult2086 struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	HealthScore     int                 `json:"healthScore"`
	Grade           string              `json:"grade"`
	Summary         RevStaleSummary2086 `json:"summary"`
	StaleDeploys    []RevStaleEntry2086 `json:"staleDeploys"`
	Recommendations []string            `json:"recommendations"`
}

type RevStaleSummary2086 struct {
	TotalDeploys int `json:"totalDeployments"`
	StaleDeploys int `json:"staleDeployments"`
}

type RevStaleEntry2086 struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Gen       int64  `json:"generation"`
}

func (s *Server) handleRevStale2086(w http.ResponseWriter, r *http.Request) {
	result := RevStaleResult2086{ScannedAt: time.Now()}
	score := 100
	deployList, _ := s.clientset.AppsV1().Deployments("").List(r.Context(), metav1.ListOptions{})

	for _, dep := range deployList.Items {
		result.Summary.TotalDeploys++
		if dep.Generation > 100 {
			result.Summary.StaleDeploys++
			result.StaleDeploys = append(result.StaleDeploys, RevStaleEntry2086{
				Name: dep.Name, Namespace: dep.Namespace, Gen: dep.Generation,
			})
			score -= 1
		}
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	sort.Slice(result.StaleDeploys, func(i, j int) bool { return result.StaleDeploys[i].Gen > result.StaleDeploys[j].Gen })
	writeJSON(w, result)
}
