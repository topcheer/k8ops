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
// v20.84 — Scalability & HA Dimension (Round 33)
// 1. Node Capacity Trend — resource capacity summary
// 2. Pod Density Forecast — pod density growth estimate
// 3. HA Workload Coverage — multi-replica + PDB + spread score
// ============================================================

type CapTrendResult2084 struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	HealthScore     int                 `json:"healthScore"`
	Grade           string              `json:"grade"`
	Summary         CapTrendSummary2084 `json:"summary"`
	Recommendations []string            `json:"recommendations"`
}

type CapTrendSummary2084 struct {
	TotalNodes       int     `json:"totalNodes"`
	TotalCapacityCPU float64 `json:"totalCapacityCPU"`
	TotalCapacityMem float64 `json:"totalCapacityMemGB"`
}

func (s *Server) handleCapTrend2084(w http.ResponseWriter, r *http.Request) {
	result := CapTrendResult2084{ScannedAt: time.Now()}
	score := 100
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})

	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		result.Summary.TotalCapacityCPU += node.Status.Capacity.Cpu().AsApproximateFloat64()
		result.Summary.TotalCapacityMem += node.Status.Capacity.Memory().AsApproximateFloat64() / 1e9
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 2. Pod Density Forecast
// ---------------------------------------------------------------

type PodForecastResult2084 struct {
	ScannedAt       time.Time              `json:"scannedAt"`
	HealthScore     int                    `json:"healthScore"`
	Grade           string                 `json:"grade"`
	Summary         PodForecastSummary2084 `json:"summary"`
	Recommendations []string               `json:"recommendations"`
}

type PodForecastSummary2084 struct {
	CurrentPods    int `json:"currentPods"`
	MaxCapacity    int `json:"maxPodCapacity"`
	GrowthHeadroom int `json:"growthHeadroom"`
}

func (s *Server) handlePodForecast2084(w http.ResponseWriter, r *http.Request) {
	result := PodForecastResult2084{ScannedAt: time.Now()}
	score := 100
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	maxCap := 0
	for _, node := range nodeList.Items {
		pods := node.Status.Allocatable.Pods()
		if pods != nil && !pods.IsZero() {
			maxCap += int(pods.AsApproximateFloat64())
		}
	}

	runningPods := 0
	for _, pod := range podList.Items {
		if pod.Status.Phase == corev1.PodRunning {
			runningPods++
		}
	}

	result.Summary.CurrentPods = runningPods
	result.Summary.MaxCapacity = maxCap
	result.Summary.GrowthHeadroom = maxCap - runningPods

	if result.Summary.GrowthHeadroom < 10 && maxCap > 0 {
		score -= 20
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)

	if result.Summary.GrowthHeadroom < 10 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("Only %d pods of growth headroom — add nodes", result.Summary.GrowthHeadroom))
	}
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 3. HA Workload Coverage
// ---------------------------------------------------------------

type HACoverResult2084 struct {
	ScannedAt       time.Time          `json:"scannedAt"`
	HealthScore     int                `json:"healthScore"`
	Grade           string             `json:"grade"`
	Summary         HACoverSummary2084 `json:"summary"`
	NotHA           []HACoverEntry2084 `json:"notHAWorkloads"`
	Recommendations []string           `json:"recommendations"`
}

type HACoverSummary2084 struct {
	TotalMultiReplica int `json:"totalMultiReplica"`
	HACovered         int `json:"haCovered"`
	NotHA             int `json:"notHA"`
}

type HACoverEntry2084 struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
}

func (s *Server) handleHACover2084(w http.ResponseWriter, r *http.Request) {
	result := HACoverResult2084{ScannedAt: time.Now()}
	score := 100
	deployList, _ := s.clientset.AppsV1().Deployments("").List(r.Context(), metav1.ListOptions{})
	pdbList, _ := s.clientset.PolicyV1().PodDisruptionBudgets("").List(r.Context(), metav1.ListOptions{})

	nsPDB := make(map[string]bool)
	for _, pdb := range pdbList.Items {
		nsPDB[pdb.Namespace] = true
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

		hasPDB := nsPDB[dep.Namespace]
		hasSpread := len(dep.Spec.Template.Spec.TopologySpreadConstraints) > 0
		hasAA := dep.Spec.Template.Spec.Affinity != nil && dep.Spec.Template.Spec.Affinity.PodAntiAffinity != nil

		if hasPDB && (hasSpread || hasAA) {
			result.Summary.HACovered++
		} else {
			result.Summary.NotHA++
			result.NotHA = append(result.NotHA, HACoverEntry2084{Name: dep.Name, Namespace: dep.Namespace})
			score -= 3
		}
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	sort.Slice(result.NotHA, func(i, j int) bool { return result.NotHA[i].Namespace < result.NotHA[j].Namespace })

	if result.Summary.NotHA > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d multi-replica workloads lack HA coverage", result.Summary.NotHA))
	}
	writeJSON(w, result)
}
