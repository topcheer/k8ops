package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v20.66 — Scalability & HA Dimension (Round 30)
// 1. HPA Scale Stability — HPA thrashing detection
// 2. Node Eviction Readiness — PDB-aware eviction safety
// 3. Cluster Scaling Headroom — remaining node slots for pods
// ============================================================

type HPAThrashResult2066 struct {
	ScannedAt       time.Time            `json:"scannedAt"`
	HealthScore     int                  `json:"healthScore"`
	Grade           string               `json:"grade"`
	Summary         HPAThrashSummary2066 `json:"summary"`
	AtRiskHPAs      []HPAThrashEntry2066 `json:"atRiskHPAs"`
	Recommendations []string             `json:"recommendations"`
}

type HPAThrashSummary2066 struct {
	TotalHPAs  int `json:"totalHPAs"`
	AtRisk     int `json:"atRiskHPAs"`
	NoBehavior int `json:"noBehavior"`
}

type HPAThrashEntry2066 struct {
	Name        string `json:"name"`
	Namespace   string `json:"namespace"`
	CurrentReps int32  `json:"currentReplicas"`
}

func (s *Server) handleHPAThrashDetect(w http.ResponseWriter, r *http.Request) {
	result := HPAThrashResult2066{ScannedAt: time.Now()}
	score := 100

	hpaList, _ := s.clientset.AutoscalingV2().HorizontalPodAutoscalers("").List(r.Context(), metav1.ListOptions{})

	for _, hpa := range hpaList.Items {
		result.Summary.TotalHPAs++
		current := hpa.Status.CurrentReplicas
		desired := hpa.Status.DesiredReplicas

		// No behavior = potential thrashing
		if hpa.Spec.Behavior == nil {
			result.Summary.NoBehavior++
			score -= 2
		}

		// Frequent scale changes
		if current != desired && desired > 0 {
			result.AtRiskHPAs = append(result.AtRiskHPAs, HPAThrashEntry2066{
				Name: hpa.Name, Namespace: hpa.Namespace, CurrentReps: current,
			})
			result.Summary.AtRisk++
			score -= 1
		}
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)

	if result.Summary.NoBehavior > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d HPAs without behavior config — add stabilizationWindowSeconds", result.Summary.NoBehavior))
	}
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 2. Node Eviction Readiness
// ---------------------------------------------------------------

type EvictReadyResult2066 struct {
	ScannedAt       time.Time             `json:"scannedAt"`
	HealthScore     int                   `json:"healthScore"`
	Grade           string                `json:"grade"`
	Summary         EvictReadySummary2066 `json:"summary"`
	UnsafeWorkloads []EvictReadyEntry2066 `json:"unsafeWorkloads"`
	Recommendations []string              `json:"recommendations"`
}

type EvictReadySummary2066 struct {
	TotalMultiReplica int `json:"totalMultiReplica"`
	WithPDB           int `json:"withPDB"`
	UnsafeEviction    int `json:"unsafeEviction"`
}

type EvictReadyEntry2066 struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
}

func (s *Server) handleEvictReadiness(w http.ResponseWriter, r *http.Request) {
	result := EvictReadyResult2066{ScannedAt: time.Now()}
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

		if nsPDB[dep.Namespace] {
			result.Summary.WithPDB++
		} else {
			result.Summary.UnsafeEviction++
			result.UnsafeWorkloads = append(result.UnsafeWorkloads, EvictReadyEntry2066{
				Name: dep.Name, Namespace: dep.Namespace,
			})
			score -= 3
		}
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	sort.Slice(result.UnsafeWorkloads, func(i, j int) bool { return result.UnsafeWorkloads[i].Namespace < result.UnsafeWorkloads[j].Namespace })

	if result.Summary.UnsafeEviction > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d workloads unsafe for eviction — add PDB", result.Summary.UnsafeEviction))
	}
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 3. Cluster Scaling Headroom
// ---------------------------------------------------------------

type ClusterScaleHRResult2066 struct {
	ScannedAt       time.Time                 `json:"scannedAt"`
	HealthScore     int                       `json:"healthScore"`
	Grade           string                    `json:"grade"`
	Summary         ClusterScaleHRSummary2066 `json:"summary"`
	Recommendations []string                  `json:"recommendations"`
}

type ClusterScaleHRSummary2066 struct {
	TotalPodCapacity int `json:"totalPodCapacity"`
	RunningPods      int `json:"runningPods"`
	HeadroomPods     int `json:"headroomPods"`
	HeadroomPct      int `json:"headroomPct"`
}

func (s *Server) handleClusterScaleHR(w http.ResponseWriter, r *http.Request) {
	result := ClusterScaleHRResult2066{ScannedAt: time.Now()}
	score := 100

	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	totalCap := 0
	for _, node := range nodeList.Items {
		pods := node.Status.Allocatable.Pods()
		if pods != nil {
			totalCap += int(pods.AsApproximateFloat64())
		}
	}

	runningPods := 0
	for _, pod := range podList.Items {
		if pod.Status.Phase == corev1.PodRunning {
			runningPods++
		}
	}

	result.Summary.TotalPodCapacity = totalCap
	result.Summary.RunningPods = runningPods
	result.Summary.HeadroomPods = totalCap - runningPods
	if totalCap > 0 {
		result.Summary.HeadroomPct = result.Summary.HeadroomPods * 100 / totalCap
	}

	if result.Summary.HeadroomPct < 20 {
		score -= 20
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)

	if result.Summary.HeadroomPct < 20 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("Pod capacity headroom only %d%% — add nodes", result.Summary.HeadroomPct))
	}
	writeJSON(w, result)
}

// keep imports
var _ = autoscalingv2.HorizontalPodAutoscaler{}
